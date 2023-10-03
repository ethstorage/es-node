// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	prv "github.com/ethstorage/go-ethstorage/ethstorage/prover"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// StreamCtxFn provides a new context to use when handling stream requests
type StreamCtxFn func() context.Context

// Note: the mocknet in testing does not support read/write stream timeouts, the timeouts are only applied if available.
// Rate-limits always apply, and are making sure the request/response throughput is not too fast, instead of too slow.
const (
	maxGossipSize = 10 * (1 << 20)
	// timeout for writing the request as client. Can be as long as serverReadRequestTimeout
	clientWriteRequestTimeout = time.Second * 10
	// timeout for reading a response of a serving peer as client. Can be as long as serverWriteChunkTimeout
	clientReadResponsetimeout = time.Second * 10
	// after the rate-limit reservation hits the max throttle delay, give up on serving a request and just close the stream
	maxThrottleDelay = time.Second * 20

	maxMessageSize = 4 * 1024 * 1024

	defaultMaxPeerCount = 30

	defaultMinPeersPerShard = 5
)

const (
	maxConcurrency = 16

	minSubTaskSize = 16
)

const (
	RequestBlobsByRangeProtocolID = "/ethstorage/dev/requestblobsbyrange/%d/1.0.0"
	RequestBlobsByListProtocolID  = "/ethstorage/dev/requestblobsbylist/%d/1.0.0"
	RequestShardList              = "/ethstorage/dev/shardlist/1.0.0"
)

var (
	maxKvCountPerReq            = uint64(16)
	syncStatusKey               = []byte("SyncStatus")
	maxFillEmptyTaskTreads      int32
	requestTimeoutInMillisecond = 1000 * time.Millisecond // Millisecond
)

func GetProtocolID(format string, l2ChainID *big.Int) protocol.ID {
	return protocol.ID(fmt.Sprintf(format, l2ChainID))
}

type requestHandlerFn func(ctx context.Context, log log.Logger, stream network.Stream)

func MakeStreamHandler(resourcesCtx context.Context, log log.Logger, fn requestHandlerFn) network.StreamHandler {
	return func(stream network.Stream) {
		handleLog := log.New("peer", stream.Conn().ID(), "remote", stream.Conn().RemoteMultiaddr())
		defer func() {
			if err := recover(); err != nil {
				handleLog.Error("p2p server request handling panic", "err", err, "protocol", stream.Protocol())
			}
		}()
		defer stream.Close()
		fn(resourcesCtx, handleLog, stream)
	}
}

type newStreamFn func(ctx context.Context, peerId peer.ID, protocolId ...protocol.ID) (network.Stream, error)

type SyncClientMetrics interface {
	ClientGetBlobsByRangeEvent(peerID string, resultCode byte, duration time.Duration)
	ClientGetBlobsByListEvent(peerID string, resultCode byte, duration time.Duration)
	ClientFillEmptyBlobsEvent(count uint64, duration time.Duration)
	ClientOnBlobsByRange(peerID string, reqCount, retBlobCount, insertedCount uint64, duration time.Duration)
	ClientOnBlobsByList(peerID string, reqCount, retBlobCount, insertedCount uint64, duration time.Duration)
	ClientRecordTimeUsed(method string) func()
	IncDropPeerCount()
	IncPeerCount()
	DecPeerCount()
}

type ShardManagerInfo interface {
	KvEntries() uint64

	ContractAddress() common.Address

	Shards() []uint64

	MaxKvSize() uint64

	GetShardMiner(shardIdx uint64) (common.Address, bool)

	GetShardEncodeType(shardIdx uint64) (uint64, bool)
}

type StorageManagerReader interface {
	ShardManagerInfo

	TryReadEncoded(kvIdx uint64, readLen int) ([]byte, bool, error)

	TryReadMeta(kvIdx uint64) ([]byte, bool, error)
}

type StorageManagerWriter interface {
	CommitBlob(kvIndex uint64, blob []byte, commit common.Hash) error
}

type StorageManager interface {
	StorageManagerReader

	StorageManagerWriter

	LastKvIndex() (uint64, error)

	DecodeKV(kvIdx uint64, b []byte, hash common.Hash, providerAddr common.Address, encodeType uint64) ([]byte, bool, error)
}

type SyncClient struct {
	log         log.Logger
	mux         *event.Feed // Event multiplexer to announce sync operation events
	cfg         *rollup.EsConfig
	db          ethdb.Database
	metrics     SyncClientMetrics
	newStreamFn newStreamFn
	tasks       []*task

	maxPeers         int
	minPeersPerShard int

	// Don't allow anything to be added to the wait-group while, or after, we are shutting down.
	// This is protected by lock.
	closingPeers               bool
	syncDone                   bool // Flag to signal that eth storage sync is done
	peers                      map[peer.ID]*Peer
	idlerPeers                 map[peer.ID]struct{} // Peers that aren't serving requests
	runningFillEmptyTaskTreads int32                // Number of working threads for processing empty task

	peerJoin chan peer.ID
	update   chan struct{} // Notification channel for possible sync progression

	// resource context: all peers and mainLoop tasks inherit this, and origin shutting down once resCancel() is called.
	resCtx    context.Context
	resCancel context.CancelFunc

	// wait group: wait for the resources to close. Adding to this is only safe if the peersLock is held.
	wg sync.WaitGroup
	// lock Protects fields (peers, idlerPeers, runningFillEmptyTaskTreads, closingPeers, syncDone,
	// task.statelessPeers, healTask.Indexes, subTask.isRunning, subTask.done, subEmptyTask.isRunning, subEmptyTask.done)
	lock sync.Mutex

	prover         prv.IProver
	startTime      time.Time // Time instance when sstorage sync started
	logTime        time.Time // Time instance when status was last reported
	storageManager StorageManager

	blobsSynced      uint64
	syncedBytes      common.StorageSize
	emptyBlobsToFill uint64
	emptyBlobsFilled uint64
}

func NewSyncClient(log log.Logger, cfg *rollup.EsConfig, newStream newStreamFn, storageManager StorageManager,
	db ethdb.Database, metrics SyncClientMetrics, mux *event.Feed) *SyncClient {
	ctx, cancel := context.WithCancel(context.Background())
	maxFillEmptyTaskTreads = int32(runtime.NumCPU() - 2)
	if maxFillEmptyTaskTreads < 1 {
		maxFillEmptyTaskTreads = 1
	}
	maxKvCountPerReq = maxMessageSize / storageManager.MaxKvSize()
	shardCount := len(storageManager.Shards())
	if metrics == nil {
		metrics = NoopMetrics
	}

	c := &SyncClient{
		log:                        log,
		mux:                        mux,
		cfg:                        cfg,
		db:                         db,
		metrics:                    metrics,
		newStreamFn:                newStream,
		idlerPeers:                 make(map[peer.ID]struct{}),
		peers:                      make(map[peer.ID]*Peer),
		peerJoin:                   make(chan peer.ID, 1),
		update:                     make(chan struct{}, 1),
		runningFillEmptyTaskTreads: 0,
		resCtx:                     ctx,
		resCancel:                  cancel,
		storageManager:             storageManager,
		prover:                     prv.NewKZGProver(log),
		maxPeers:                   defaultMaxPeerCount,
		minPeersPerShard:           getMinPeersPerShard(defaultMaxPeerCount, shardCount),
	}
	return c
}

func (s *SyncClient) UpdateMaxPeers(maxPeers int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.maxPeers = maxPeers
	shardCount := len(s.storageManager.Shards())
	s.minPeersPerShard = getMinPeersPerShard(maxPeers, shardCount)
}

func getMinPeersPerShard(maxPeers, shardCount int) int {
	minPeersPerShard := (maxPeers + shardCount - 1) / shardCount
	if minPeersPerShard < defaultMinPeersPerShard {
		minPeersPerShard = defaultMinPeersPerShard
	}
	return minPeersPerShard
}

func (s *SyncClient) setSyncDone() {
	s.syncDone = true
	if s.mux != nil {
		s.mux.Send(EthStorageSyncDone{DoneType: AllShardDone})
	}
	log.Info("Sync done", "time used", time.Since(s.startTime))
}

func (s *SyncClient) loadSyncStatus() {
	// Start a fresh sync for retrieval.
	s.blobsSynced, s.syncedBytes = 0, 0
	s.emptyBlobsToFill, s.emptyBlobsFilled = 0, 0
	var progress SyncProgress

	if status, _ := s.db.Get(syncStatusKey); status != nil {
		if err := json.Unmarshal(status, &progress); err != nil {
			log.Error("Failed to decode storage sync status", "err", err)
		} else {
			for _, task := range progress.Tasks {
				log.Debug("Load sync subTask", "Contract", task.Contract.Hex(),
					"shard", task.ShardId, "count", len(task.SubTasks))
				task.healTask = &healTask{
					Indexes: make(map[uint64]int64),
					task:    task,
				}
				task.statelessPeers = make(map[peer.ID]struct{})
				task.peers = make(map[peer.ID]struct{})
				for _, sTask := range task.SubTasks {
					sTask.task = task
					sTask.next = sTask.First
				}
				for _, sEmptyTask := range task.SubEmptyTasks {
					sEmptyTask.task = task
					s.emptyBlobsToFill += (sEmptyTask.Last - sEmptyTask.First)
				}
			}
			s.blobsSynced, s.syncedBytes = progress.BlobsSynced, progress.SyncedBytes
			s.emptyBlobsFilled = progress.EmptyBlobsFilled
		}
	}

	// create tasks
	lastKvIndex, err := s.storageManager.LastKvIndex()
	if err != nil {
		// TODO: panic?
		log.Info("loadSyncStatus failed: get lastKvIdx")
		lastKvIndex = 0
	}
	for _, sid := range s.storageManager.Shards() {
		exist := false
		for _, task := range progress.Tasks {
			if task.Contract == s.storageManager.ContractAddress() && task.ShardId == sid {
				s.tasks = append(s.tasks, task)
				exist = true
				continue
			}
		}
		if exist {
			continue
		}

		task := s.createTask(sid, lastKvIndex)
		s.tasks = append(s.tasks, task)
	}
}

func (s *SyncClient) createTask(sid uint64, lastKvIndex uint64) *task {
	task := task{
		Contract:       s.storageManager.ContractAddress(),
		ShardId:        sid,
		statelessPeers: make(map[peer.ID]struct{}),
		peers:          make(map[peer.ID]struct{}),
	}

	healTask := healTask{
		task:    &task,
		Indexes: make(map[uint64]int64),
	}

	first, limit := s.storageManager.KvEntries()*sid, s.storageManager.KvEntries()*(sid+1)
	firstEmpty, limitForEmpty := uint64(0), uint64(0)
	if first >= lastKvIndex {
		firstEmpty, limitForEmpty = first, limit
		limit = first
	} else if limit >= lastKvIndex {
		firstEmpty, limitForEmpty = lastKvIndex, limit
		limit = lastKvIndex
	}

	subTasks := make([]*subTask, 0)
	// split subTask for a shard to 16 subtasks and if one batch is too small
	// set to minSubTaskSize
	maxTaskSize := (limit - first - 1 + maxConcurrency) / maxConcurrency
	if maxTaskSize < minSubTaskSize {
		maxTaskSize = minSubTaskSize
	}

	for first < limit {
		last := first + maxTaskSize
		if last > limit {
			last = limit
		}
		subTask := subTask{
			task:  &task,
			next:  first,
			First: first,
			Last:  last,
			done:  false,
		}

		subTasks = append(subTasks, &subTask)
		first = last
	}

	subEmptyTasks := make([]*subEmptyTask, 0)
	if limitForEmpty > 0 {
		s.emptyBlobsToFill += limitForEmpty - firstEmpty
		maxEmptyTaskSize := (limitForEmpty - firstEmpty + uint64(maxFillEmptyTaskTreads)) / uint64(maxFillEmptyTaskTreads)
		if maxEmptyTaskSize < minSubTaskSize {
			maxEmptyTaskSize = minSubTaskSize
		}

		for firstEmpty < limitForEmpty {
			last := firstEmpty + maxEmptyTaskSize
			if last > limitForEmpty {
				last = limitForEmpty
			}
			subTask := subEmptyTask{
				task:  &task,
				First: firstEmpty,
				Last:  last,
				done:  false,
			}

			subEmptyTasks = append(subEmptyTasks, &subTask)
			firstEmpty = last
		}
	}

	task.healTask, task.SubTasks, task.SubEmptyTasks = &healTask, subTasks, subEmptyTasks
	return &task
}

// saveSyncStatus marshals the remaining sync tasks into leveldb.
func (s *SyncClient) saveSyncStatus() {
	// Store the actual progress markers
	progress := &SyncProgress{
		Tasks:            s.tasks,
		BlobsSynced:      s.blobsSynced,
		SyncedBytes:      s.syncedBytes,
		EmptyBlobsToFill: s.emptyBlobsToFill,
		EmptyBlobsFilled: s.emptyBlobsFilled,
	}
	status, err := json.Marshal(progress)
	if err != nil {
		panic(err) // This can only fail during implementation
	}
	if err := s.db.Put(syncStatusKey, status); err != nil {
		log.Crit("Failed to store sync status", "err", err)
	}
}

// cleanTasks removes kv range retrieval tasks that have already been completed.
func (s *SyncClient) cleanTasks() {
	// Sync wasn't finished previously, check for any subTask that can be finalized
	s.lock.Lock()
	defer s.lock.Unlock()
	allDone := true
	for _, t := range s.tasks {
		for i := 0; i < len(t.SubTasks); i++ {
			exist, min := t.healTask.hasIndexInRange(t.SubTasks[i].First, t.SubTasks[i].next)
			// if exist, min will be the smallest index in range [subTask.First, subTask.next)
			// if no exist, min will be next, so subTask.First can directly set to subTask.next
			t.SubTasks[i].First = min
			if t.SubTasks[i].done && !exist {
				t.SubTasks = append(t.SubTasks[:i], t.SubTasks[i+1:]...)
				i--
			}
		}
		for i := 0; i < len(t.SubEmptyTasks); i++ {
			if t.SubEmptyTasks[i].done {
				t.SubEmptyTasks = append(t.SubEmptyTasks[:i], t.SubEmptyTasks[i+1:]...)
				i--
			}
		}
		if len(t.SubTasks) > 0 || len(t.SubEmptyTasks) > 0 {
			allDone = false
		} else if !t.done {
			t.done = true
			if s.mux != nil {
				s.mux.Send(EthStorageSyncDone{DoneType: SingleShardDone, ShardId: t.ShardId})
			}
		}
	}

	// If everything was just finalized, generate the account trie and origin heal
	if allDone {
		s.setSyncDone()
		log.Info("storage sync done", "subTask count", len(s.tasks))

		s.report(true)
	}
}

func (s *SyncClient) Start() {
	if s.startTime == (time.Time{}) {
		s.startTime = time.Now()
	}

	// Retrieve the previous sync status from LevelDB and abort if already synced
	s.loadSyncStatus()

	s.wg.Add(1)
	go s.mainLoop()
}

func (s *SyncClient) AddPeer(id peer.ID, shards map[common.Address][]uint64) bool {
	s.lock.Lock()
	if _, ok := s.peers[id]; ok {
		s.log.Warn("cannot register peer for sync duties, peer was already registered", "peer", id)
		s.lock.Unlock()
		return true
	}
	if s.closingPeers {
		s.lock.Unlock()
		return false
	}
	if !s.needThisPeer(shards) {
		s.metrics.IncDropPeerCount()
		s.lock.Unlock()
		return false
	}
	// add new peer routine
	peer := NewPeer(0, s.cfg.L2ChainID, id, s.newStreamFn, shards)
	s.peers[id] = peer

	s.idlerPeers[id] = struct{}{}
	s.addPeerToTask(id, shards)
	s.metrics.IncPeerCount()
	s.lock.Unlock()

	s.notifyPeerJoin(id)
	return true
}

func (s *SyncClient) RemovePeer(id peer.ID) {
	s.lock.Lock()
	defer s.lock.Unlock()
	peer, ok := s.peers[id]
	if !ok {
		s.log.Warn("cannot remove peer from sync duties, peer was not registered", "peer", id)
		return
	}
	peer.resCancel() // once loop exits
	delete(s.peers, id)
	s.removePeerFromTask(id, peer.shards)
	s.metrics.DecPeerCount()
	delete(s.idlerPeers, id)
	for _, t := range s.tasks {
		delete(t.statelessPeers, id)
	}
}

// Close will shut down the sync client and all attached work, and block until shutdown is complete.
// This will block if the Start() has not created the main background loop.
func (s *SyncClient) Close() error {
	s.lock.Lock()
	s.closingPeers = true
	s.lock.Unlock()
	s.resCancel()
	s.wg.Wait()
	s.cleanTasks()
	s.saveSyncStatus()
	s.report(true)
	return nil
}

func (s *SyncClient) RequestL2Range(ctx context.Context, start, end uint64) (uint64, error) {
	for _, peer := range s.peers {
		id := rand.Uint64()
		var packet BlobsByRangePacket
		_, err := peer.RequestBlobsByRange(id, s.storageManager.ContractAddress(), start/s.storageManager.KvEntries(), start, end, &packet)
		if err != nil {
			return 0, err
		}
		_, _, _, err = s.onResult(packet.Blobs)
		if err != nil {
			return 0, err
		}
		return id, nil
	}
	return 0, fmt.Errorf("no peer can be used to send requests")
}

func (s *SyncClient) RequestL2List(indexes []uint64) (uint64, error) {
	if len(indexes) == 0 {
		return 0, nil
	}
	for _, peer := range s.peers {
		id := rand.Uint64()
		var packet BlobsByListPacket
		_, err := peer.RequestBlobsByList(id, s.storageManager.ContractAddress(), indexes[0]/s.storageManager.KvEntries(), indexes, &packet)
		if err != nil {
			return 0, err
		}
		s.onResult(packet.Blobs)
		if err != nil {
			return 0, err
		}
		return id, nil
	}
	return 0, fmt.Errorf("no peer can be used to send requests")
}

func (s *SyncClient) mainLoop() {
	defer s.wg.Done()

	for {
		// Remove all completed tasks and terminate sync if everything's done
		s.cleanTasks()
		if s.syncDone {
			return
		}
		s.assignBlobRangeTasks()
		// Assign all the Data retrieval tasks to any free peers
		s.assignBlobHealTasks()

		s.assignFillEmptyBlobTasks()

		select {
		case <-time.After(requestTimeoutInMillisecond):

		case <-s.update:
			// Something happened (new peer, delivery, timeout), recheck tasks
		case <-s.peerJoin:
			// A new peer joined, try to schedule it new tasks
		case <-s.resCtx.Done():
			s.log.Info("stopped P2P req-resp L2 block sync client")
			return
		}
		// Report stats if something meaningful happened
		s.report(false)
	}
}

func (s *SyncClient) notifyPeerJoin(id peer.ID) {
	select {
	case s.peerJoin <- id:
	default:
	}
}

func (s *SyncClient) notifyUpdate() {
	select {
	case s.update <- struct{}{}:
	default:
	}
}

// assignBlobRangeTasks attempts to match idle peers to pending blob range retrievals.
func (s *SyncClient) assignBlobRangeTasks() {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(s.idlerPeers) == 0 {
		return
	}

	// Iterate over all the tasks and try to find a pending one
	for _, t := range s.tasks {
		maxRange := maxMessageSize / ethstorage.ContractToShardManager[t.Contract].MaxKvSize() * 2
		for _, stask := range t.SubTasks {
			st := stask
			if st.done {
				continue
			}
			// Skip any tasks already running
			if st.isRunning {
				continue
			}
			pr := s.getIdlePeerForTask(t)
			if pr == nil {
				continue
			}

			last := st.next + maxRange
			if last > st.Last {
				last = st.Last
			}
			req := &blobsByRangeRequest{
				peer:     pr.ID(),
				id:       rand.Uint64(),
				contract: t.Contract,
				shardId:  t.ShardId,
				origin:   st.next,
				limit:    last - 1,
				time:     time.Now(),
				subTask:  st,
			}
			delete(s.idlerPeers, pr.ID())
			st.isRunning = true

			s.wg.Add(1)
			go func(id peer.ID) {
				defer func() {
					s.lock.Lock()
					st.isRunning = false
					s.lock.Unlock()
					s.notifyUpdate()
					s.wg.Done()
				}()
				start := time.Now()
				var packet BlobsByRangePacket
				// Attempt to send the remote request and revert if it fails
				returnCode, err := pr.RequestBlobsByRange(req.id, req.contract, req.shardId, req.origin, req.limit, &packet)
				s.metrics.ClientGetBlobsByRangeEvent(req.peer.String(), returnCode, time.Since(start))

				s.lock.Lock()
				if _, ok := s.peers[id]; ok {
					s.idlerPeers[id] = struct{}{}
				}
				s.lock.Unlock()

				if err != nil {
					log.Warn("Failed to request blobs", "err", err)
					return
				}

				if req.id != packet.ID || req.contract != packet.Contract || req.shardId != packet.ShardId {
					log.Warn("req mismatch with res", "req id", req.id, "packet id", packet.ID,
						"req contract", req.contract.Hex(), "packet contract", packet.Contract.Hex(),
						"req shardId", req.shardId, "packet shardId", packet.ShardId)
					return
				}
				res := &blobsByRangeResponse{
					req:   req,
					Blobs: packet.Blobs,
					time:  time.Now(),
				}
				s.OnBlobsByRange(res)
			}(pr.id)
		}
	}
}

// assignBlobHealTasks attempts to match idle peers to heal blob requests to retrieval missing blob from the blob list request.
func (s *SyncClient) assignBlobHealTasks() {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(s.idlerPeers) == 0 {
		return
	}

	// Iterate over all the tasks and try to find a pending one
	for _, t := range s.tasks {
		// All the kvs are downloading, wait for request time or success
		batch := maxMessageSize / ethstorage.ContractToShardManager[t.Contract].MaxKvSize() * 2

		// kvHealTask pending retrieval, try to find an idle pr. If no such pr
		// exists, we probably assigned tasks for all (or they are stateless).
		// Abort the entire assignment mechanism.
		if len(s.idlerPeers) == 0 {
			return
		}
		indexes := t.healTask.getBlobIndexesForRequest(batch)
		if len(indexes) == 0 {
			continue
		}
		pr := s.getIdlePeerForTask(t)
		if pr == nil {
			log.Info("pr for request no found", "Contract", t.Contract.Hex(), "shard id",
				t.ShardId, "index count", t.healTask.count(), "peers", len(s.peers), "idlers", len(s.idlerPeers))
			continue
		}

		req := &blobsByListRequest{
			peer:     pr.ID(),
			id:       rand.Uint64(),
			contract: t.Contract,
			shardId:  t.ShardId,
			indexes:  indexes,
			time:     time.Now(),
			healTask: t.healTask,
		}
		delete(s.idlerPeers, pr.ID())
		req.healTask.refresh(indexes)

		s.wg.Add(1)
		go func(id peer.ID) {
			defer func() {
				s.notifyUpdate()
				s.wg.Done()
			}()
			start := time.Now()
			var packet BlobsByListPacket
			// Attempt to send the remote request and revert if it fails
			returnCode, err := pr.RequestBlobsByList(req.id, req.contract, req.shardId, req.indexes, &packet)
			s.metrics.ClientGetBlobsByListEvent(req.peer.String(), returnCode, time.Since(start))

			s.lock.Lock()
			if _, ok := s.peers[id]; ok {
				s.idlerPeers[id] = struct{}{}
			}
			s.lock.Unlock()

			if err != nil {
				log.Warn("Failed to request packet", "err", err)
				return
			}
			if req.id != packet.ID || req.contract != packet.Contract || req.shardId != packet.ShardId {
				log.Warn("req mismatch with res", "req id", req.id, "packet id", packet.ID,
					"req contract", req.contract.Hex(), "packet contract", packet.Contract.Hex(),
					"req shardId", req.shardId, "packet shardId", packet.ShardId)
				return
			}
			res := &blobsByListResponse{
				req:   req,
				Blobs: packet.Blobs,
				time:  time.Now(),
			}
			s.OnBlobsByList(res)
		}(pr.ID())
	}
}

// assignFillEmptyBlobTasks attempts to match idle peers to heal kv requests to retrieval missing kv from the kv range request.
func (s *SyncClient) assignFillEmptyBlobTasks() {
	s.lock.Lock()
	defer s.lock.Unlock()
	for _, task := range s.tasks {
		for _, emptyTask := range task.SubEmptyTasks {
			if s.closingPeers {
				return
			}
			if s.runningFillEmptyTaskTreads >= maxFillEmptyTaskTreads {
				return
			}
			if emptyTask.isRunning || emptyTask.done {
				continue
			}
			eTask := emptyTask
			start, last := eTask.First, eTask.Last
			if last > start+minSubTaskSize {
				last = start + minSubTaskSize
			}
			eTask.isRunning = true
			s.runningFillEmptyTaskTreads += 1
			s.wg.Add(1)
			go func(eTask *subEmptyTask, contract common.Address, start, limit uint64) {
				defer func() {
					s.notifyUpdate()
					s.wg.Done()
				}()
				t := time.Now()
				next, err := s.FillFileWithEmptyBlob(start, limit)
				if err != nil {
					log.Warn("fill in empty fail", "err", err.Error())
				} else {
					log.Debug("fill in empty done", "time", time.Now().Sub(t).Seconds())
				}
				filled := next - start
				s.emptyBlobsFilled += filled
				if s.emptyBlobsToFill >= filled {
					s.emptyBlobsToFill -= filled
				}

				s.lock.Lock()
				eTask.First = next
				if eTask.First >= eTask.Last {
					eTask.done = true
				}
				eTask.isRunning = false
				s.runningFillEmptyTaskTreads -= 1
				s.lock.Unlock()
			}(eTask, task.Contract, start, last-1)
		}
	}
}

func (s *SyncClient) getIdlePeerForTask(t *task) *Peer {
	for id, _ := range s.idlerPeers {
		if _, ok := t.statelessPeers[id]; ok {
			continue
		}
		p := s.peers[id]
		if p.IsShardExist(t.Contract, t.ShardId) {
			return p
		}
	}
	return nil
}

// OnBlobsByRange is a callback method to invoke when a batch of Contract
// bytes codes are received from a remote peer.
func (s *SyncClient) OnBlobsByRange(res *blobsByRangeResponse) {
	var (
		size     common.StorageSize
		req      = res.req
		start    = time.Now()
		reqCount = req.limit - req.origin + 1
	)

	if reqCount > maxKvCountPerReq {
		reqCount = maxKvCountPerReq
	}
	for _, blob := range res.Blobs {
		if blob != nil {
			size += common.StorageSize(len(blob.EncodedBlob))
		}
	}
	s.log.Debug("OnBlobsByRange: static", "reqid", req.id, "blob count", len(res.Blobs), "bytes", size)

	blobsInRange := make([]*BlobPayload, 0)
	for _, blob := range res.Blobs {
		if req.origin <= blob.BlobIndex && req.limit >= blob.BlobIndex {
			blobsInRange = append(blobsInRange, blob)
		}
	}
	if len(res.Blobs) > len(blobsInRange) {
		s.log.Trace("Drop unexpected kvs", "count", len(res.Blobs)-len(blobsInRange))
	}

	// Response is valid, but check if peer is signalling that it does not have
	// the requested Data. For blob range queries that means the peer is not
	// yet synced.
	if len(blobsInRange) == 0 {
		s.log.Warn("Peer rejected get blob by range request")
		s.lock.Lock()
		if _, ok := s.peers[req.peer]; ok {
			req.subTask.task.statelessPeers[req.peer] = struct{}{}
		}
		s.lock.Unlock()
		s.metrics.ClientOnBlobsByRange(req.peer.String(), reqCount, uint64(len(res.Blobs)), 0, time.Since(start))
		return
	}

	synced, syncedBytes, inserted, err := s.onResult(blobsInRange)
	if err != nil {
		log.Error("OnBlobsByRange fail", "err", err.Error())
		return
	}

	s.blobsSynced += synced
	s.syncedBytes += common.StorageSize(syncedBytes)
	s.metrics.ClientOnBlobsByRange(req.peer.String(), reqCount, uint64(len(res.Blobs)), synced, time.Since(start))
	log.Info("Persisted set of kvs", "count", synced, "bytes", syncedBytes)

	// set peer to stateless peer if fail too much
	if len(inserted) == 0 {
		s.lock.Lock()
		if _, ok := s.peers[req.peer]; ok {
			req.subTask.task.statelessPeers[req.peer] = struct{}{}
		}
		s.lock.Unlock()
		return
	}

	sort.Slice(inserted, func(i, j int) bool {
		return inserted[i] < inserted[j]
	})
	max := inserted[len(inserted)-1]
	missing := make([]uint64, 0)
	for i, n := 0, res.req.subTask.next; n <= max; n++ {
		if inserted[i] == n {
			i++
		} else if inserted[i] > n {
			missing = append(missing, n)
		}
	}
	s.lock.Lock()
	res.req.subTask.task.healTask.insert(missing)
	if max == res.req.subTask.Last-1 {
		res.req.subTask.done = true
	}
	res.req.subTask.next = max + 1
	s.lock.Unlock()
}

// OnBlobsByList is a callback method to invoke when a batch of Contract
// bytes codes are received from a remote peer.
func (s *SyncClient) OnBlobsByList(res *blobsByListResponse) {
	var (
		size  common.StorageSize
		req   = res.req
		start = time.Now()
	)
	for _, blob := range res.Blobs {
		if blob != nil {
			size += common.StorageSize(len(blob.EncodedBlob))
		}
	}
	s.log.Debug("OnBlobsByList: static", "reqid", req.id, "blob count", len(res.Blobs), "bytes", size)

	startIdx, endIdx := s.storageManager.KvEntries()*req.shardId, s.storageManager.KvEntries()*(req.shardId+1)-1
	blobsInRange := make([]*BlobPayload, 0)
	for _, blob := range res.Blobs {
		if startIdx <= blob.BlobIndex && endIdx >= blob.BlobIndex {
			blobsInRange = append(blobsInRange, blob)
		}
	}
	if len(res.Blobs) > len(blobsInRange) {
		s.log.Trace("Drop unexpected kvs", "count", len(res.Blobs)-len(blobsInRange))
	}

	// Response is valid, but check if peer is signalling that it does not have
	// the requested Data. For kv range queries that means the peer is not
	// yet synced.
	if len(blobsInRange) == 0 {
		s.log.Warn("Peer rejected get blobs by list request")
		s.lock.Lock()
		if _, ok := s.peers[req.peer]; ok {
			req.healTask.task.statelessPeers[req.peer] = struct{}{}
		}
		s.lock.Unlock()
		s.metrics.ClientOnBlobsByList(req.peer.String(), uint64(len(req.indexes)), uint64(len(res.Blobs)),
			0, time.Since(start))
		return
	}

	synced, syncedBytes, inserted, err := s.onResult(blobsInRange)
	if err != nil {
		log.Error("OnBlobsByList fail", "err", err.Error())
		return
	}

	s.blobsSynced += synced
	s.syncedBytes += common.StorageSize(syncedBytes)
	s.metrics.ClientOnBlobsByList(req.peer.String(), uint64(len(req.indexes)), uint64(len(res.Blobs)),
		synced, time.Since(start))
	log.Trace("Persisted set of kvs", "count", synced, "bytes", syncedBytes)

	s.lock.Lock()
	// set peer to stateless peer if fail too much
	if len(inserted) == 0 {
		if _, ok := s.peers[req.peer]; ok {
			req.healTask.task.statelessPeers[req.peer] = struct{}{}
		}
	}
	res.req.healTask.remove(inserted)
	s.lock.Unlock()
}

// FillFileWithEmptyBlob this func is used to fill empty blobs to storage file to make the whole file data encoded.
// file in the blobs between origin and limit (include limit). if the lastKvIdx larger than kv idx to fill, ignore it.
func (s *SyncClient) FillFileWithEmptyBlob(start, limit uint64) (uint64, error) {
	st := time.Now()
	inserted := uint64(0)
	defer s.metrics.ClientFillEmptyBlobsEvent(inserted, time.Since(st))
	empty := make([]byte, 0)
	lastBlobIdx, err := s.storageManager.LastKvIndex()
	if err != nil {
		return start, fmt.Errorf("get lastBlobIdx for FillEmptyKV fail, err: %s", err.Error())
	}
	if start < lastBlobIdx {
		start = lastBlobIdx
	}
	for idx := start; idx <= limit; idx++ {
		err = s.storageManager.CommitBlob(idx, empty, common.Hash{})
		if err != nil {
			err = fmt.Errorf("write empty to kv file fail, index: %d; error: %s", idx, err.Error())
			return idx, err
		}
		inserted++
	}

	return limit + 1, nil
}

// onResult is exclusively called by the main loop, and has thus direct access to the request bookkeeping state.
// This function verifies if the result is canonical, and either promotes the result or moves the result into quarantine.
func (s *SyncClient) onResult(blobs []*BlobPayload) (uint64, uint64, []uint64, error) {
	var (
		synced      uint64
		syncedBytes uint64
		inserted    = make([]uint64, 0)
	)
	for _, payload := range blobs {
		synced++
		syncedBytes += uint64(len(payload.EncodedBlob))

		decodedBlob, success := s.decodeKV(payload)
		if !success {
			continue
		}

		success = s.checkBlobCommit(decodedBlob, payload)
		if !success {
			continue
		}

		success = s.commitBlob(decodedBlob, payload)
		if success {
			inserted = append(inserted, payload.BlobIndex)
		}
	}

	return synced, syncedBytes, inserted, nil
}

func (s *SyncClient) decodeKV(payload *BlobPayload) ([]byte, bool) {
	recordDur := s.metrics.ClientRecordTimeUsed("decodeKv")
	defer recordDur()

	decodedBlob, found, err := s.storageManager.DecodeKV(payload.BlobIndex, payload.EncodedBlob, payload.BlobCommit,
		payload.MinerAddress, payload.EncodeType)
	if err != nil || !found {
		if err != nil {
			s.log.Error("failed to decode", "kv_idx", payload.BlobIndex, "error", err)
		} else {
			s.log.Error("failed to decode", "kv_idx", payload.BlobIndex, "error", "not found")
		}
		return []byte{}, false
	}
	return decodedBlob, true
}

func (s *SyncClient) checkBlobCommit(decodedBlob []byte, payload *BlobPayload) bool {
	recordDur := s.metrics.ClientRecordTimeUsed("getRoot")
	root, err := s.prover.GetRoot(decodedBlob, 0, 0)
	recordDur()

	if err != nil {
		s.log.Error("get proof fail", "idx", payload.BlobIndex, "err", err.Error())
		return false
	}
	if !bytes.Equal(root[:ethstorage.HashSizeInContract], payload.BlobCommit[:ethstorage.HashSizeInContract]) {
		s.log.Error("compare blob failed", "idx", payload.BlobIndex, "err",
			fmt.Sprintf("verify blob fail: root: %s; MetaHash hash (24): %s, providerAddr %s, data len %d",
				common.Bytes2Hex(root[:ethstorage.HashSizeInContract]), common.Bytes2Hex(payload.BlobCommit[:ethstorage.HashSizeInContract]),
				payload.MinerAddress.Hex(), len(payload.EncodedBlob)))
		return false
	}

	return true
}

func (s *SyncClient) commitBlob(decodedBlob []byte, payload *BlobPayload) bool {
	recordDur := s.metrics.ClientRecordTimeUsed("commitBlob")
	defer recordDur()

	err := s.storageManager.CommitBlob(payload.BlobIndex, decodedBlob, payload.BlobCommit)
	if err != nil {
		s.log.Error("commit blob failed", "err", err.Error())
		return false
	}

	return true
}

// report calculates various status reports and provides it to the user.
func (s *SyncClient) report(force bool) {
	// Don't report all the events, just occasionally
	if !force && time.Since(s.logTime) < 8*time.Second {
		return
	}
	s.logTime = time.Now()

	// Don't report anything until we have a meaningful progress
	synced := s.blobsSynced
	if synced == 0 {
		return
	}
	kvsToSync := uint64(0)
	subTaskRemain := 0
	for _, task := range s.tasks {
		for _, subTask := range task.SubTasks {
			kvsToSync = kvsToSync + (subTask.Last - subTask.next)
			subTaskRemain++
		}
		kvsToSync = kvsToSync + uint64(task.healTask.count())
	}

	elapsed := time.Since(s.startTime)
	estTime := elapsed / time.Duration(synced) * time.Duration(kvsToSync+synced)

	// Create a mega progress report
	var (
		progress = fmt.Sprintf("%.2f%%", float64(synced)*100/float64(kvsToSync+synced))
		kv       = fmt.Sprintf("%v@%v", log.FormatLogfmtUint64(s.blobsSynced), s.syncedBytes.TerminalString())
	)
	log.Info("Sstorage sync in progress", "synced", progress, "state", synced, "kvsToSync", kvsToSync,
		"sub subTask remain", subTaskRemain, "kv", kv, "eta", common.PrettyDuration(estTime-elapsed),
		"empty KV filled", s.emptyBlobsFilled, "empty KV to fill", s.emptyBlobsToFill)
}

func (s *SyncClient) needThisPeer(contractShards map[common.Address][]uint64) bool {
	for contract, shards := range contractShards {
		for _, shard := range shards {
			for _, t := range s.tasks {
				if t.Contract != contract || shard != t.ShardId {
					continue
				}

				// when the peer and local node has overlap, the peer will be added to the sync client when
				// - SyncClient peer count smaller than maxPeers; or
				// - task peer count smaller than minPeersPerShard
				// otherwise, the peer will be disconnected.
				if len(s.peers) < s.maxPeers || len(t.peers) < s.minPeersPerShard {
					return true
				}
			}
		}
	}

	return false
}

func (s *SyncClient) addPeerToTask(peerID peer.ID, contractShards map[common.Address][]uint64) {
	for contract, shards := range contractShards {
		for _, shard := range shards {
			for _, t := range s.tasks {
				if t.Contract == contract && shard == t.ShardId {
					t.peers[peerID] = struct{}{}
				}
			}
		}
	}
}

func (s *SyncClient) removePeerFromTask(peerID peer.ID, contractShards map[common.Address][]uint64) {
	for contract, shards := range contractShards {
		for _, shard := range shards {
			for _, t := range s.tasks {
				if t.Contract == contract && shard == t.ShardId {
					delete(t.peers, peerID)
				}
			}
		}
	}
}
