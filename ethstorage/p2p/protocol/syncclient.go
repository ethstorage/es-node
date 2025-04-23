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
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
	prv "github.com/ethstorage/go-ethstorage/ethstorage/prover"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-yamux/v4"
)

// StreamCtxFn provides a new context to use when handling stream requests
type StreamCtxFn func() context.Context

// Note: the mock net in testing does not support read/write stream timeouts, the timeouts are only applied if available.
// Rate-limits always apply, and are making sure the request/response throughput is not too fast, instead of too slow.
const (
	maxGossipSize = 10 * (1 << 20)

	// after the rate-limit reservation hits the max throttle delay, give up on serving a request and just close the stream
	maxThrottleDelay = time.Second * 20

	NewStreamTimeout = time.Second * 15

	defaultMinPeersPerShard = 5

	minSubTaskSize = 16
)

const (
	RequestBlobsByRangeProtocolID = "/ethstorage/dev/requestblobsbyrange/%d/1.0.0"
	RequestBlobsByListProtocolID  = "/ethstorage/dev/requestblobsbylist/%d/1.0.0"
	RequestShardList              = "/ethstorage/dev/shardlist/1.0.0"
)

var (
	maxKvCountPerReq            = uint64(16)
	SyncStatusKey               = []byte("SyncStatusKey")
	SyncTasksKey                = []byte("SyncStatus") // TODO this is the legacy value, change the value before next test net
	maxFillEmptyTaskTreads      = 1
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
				handleLog.Error("P2p server request handling panic", "err", err, "protocol", stream.Protocol())
			}
		}()
		defer stream.Close()
		fn(resourcesCtx, handleLog, stream)
	}
}

type newStreamFn func(ctx context.Context, peerId peer.ID, protocolId ...protocol.ID) (network.Stream, error)

type capacitySort struct {
	ids  []peer.ID
	caps []float64
}

func (s *capacitySort) Len() int {
	return len(s.ids)
}

func (s *capacitySort) Less(i, j int) bool {
	return s.caps[i] < s.caps[j]
}

func (s *capacitySort) Swap(i, j int) {
	s.ids[i], s.ids[j] = s.ids[j], s.ids[i]
	s.caps[i], s.caps[j] = s.caps[j], s.caps[i]
}

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

	CommitEmptyBlobs(start, limit uint64) (uint64, uint64, error)

	CommitBlobs(kvIndices []uint64, blobs [][]byte, commits []common.Hash) ([]uint64, error)
}

type StorageManager interface {
	StorageManagerReader

	StorageManagerWriter

	LastKvIndex() uint64

	DecodeKV(kvIdx uint64, b []byte, hash common.Hash, providerAddr common.Address, encodeType uint64) ([]byte, bool, error)

	DownloadAllMetas(ctx context.Context, batchSize uint64) error
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
	syncerParams     *SyncerParams

	// Don't allow anything to be added to the wait-group while, or after, we are shutting down.
	// This is protected by lock.
	closingPeers               bool
	syncDone                   bool // Flag to signal that eth storage sync is done
	peers                      map[peer.ID]*Peer
	idlerPeers                 map[peer.ID]struct{} // Peers that aren't serving requests
	runningFillEmptyTaskTreads int                  // Number of working threads for processing empty task
	peerJoin                   chan peer.ID
	update                     chan struct{} // Notification channel for possible sync progression

	// resource context: all peers and mainLoop tasks inherit this, and origin shutting down once resCancel() is called.
	resCtx    context.Context
	resCancel context.CancelFunc

	// wait group: wait for the resources to close. Adding to this is only safe if the peersLock is held.
	wg sync.WaitGroup
	// lock Protects fields (peers, idlerPeers, runningFillEmptyTaskTreads, closingPeers, syncDone,
	// task.statelessPeers, healTask.Indexes, subTask.isRunning, subTask.done, subEmptyTask.isRunning, subEmptyTask.done)
	lock sync.Mutex

	prover         prv.IProver
	logTime        time.Time // Time instance when status was last reported
	storageManager StorageManager
}

func NewSyncClient(log log.Logger, cfg *rollup.EsConfig, newStream newStreamFn, storageManager StorageManager, params *SyncerParams,
	db ethdb.Database, m SyncClientMetrics, mux *event.Feed) *SyncClient {
	ctx, cancel := context.WithCancel(context.Background())
	if params.FillEmptyConcurrency > 0 {
		maxFillEmptyTaskTreads = params.FillEmptyConcurrency
	} else if runtime.NumCPU() > 2 {
		maxFillEmptyTaskTreads = runtime.NumCPU() - 2
	}
	maxKvCountPerReq = params.InitRequestSize / storageManager.MaxKvSize()
	shardCount := len(storageManager.Shards())
	if m == nil {
		m = metrics.NoopMetrics
	}

	c := &SyncClient{
		log:                        log,
		mux:                        mux,
		cfg:                        cfg,
		db:                         db,
		metrics:                    m,
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
		maxPeers:                   params.MaxPeers,
		minPeersPerShard:           getMinPeersPerShard(params.MaxPeers, shardCount),
		syncerParams:               params,
	}
	return c
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
	log.Info("Sync done")
}

func (s *SyncClient) loadSyncStatus() {
	var progress SyncProgress

	if status, _ := s.db.Get(SyncTasksKey); status != nil {
		if err := json.Unmarshal(status, &progress); err != nil {
			log.Error("Failed to decode storage sync status", "err", err)
		} else {
			for _, t := range progress.Tasks {
				log.Debug("Load sync subTask", "contract", t.Contract.Hex(),
					"shard", t.ShardId, "count", len(t.SubTasks))
				t.healTask = &healTask{
					Indexes: make(map[uint64]int64),
					task:    t,
				}
				t.statelessPeers = make(map[peer.ID]struct{})
				for _, sTask := range t.SubTasks {
					sTask.task = t
					sTask.next = sTask.First
				}
				for _, sEmptyTask := range t.SubEmptyTasks {
					sEmptyTask.task = t
				}
			}
		}
	}

	var states map[uint64]*SyncState
	if status, _ := s.db.Get(SyncStatusKey); status != nil {
		if err := json.Unmarshal(status, &states); err != nil {
			log.Error("Failed to decode storage sync status", "err", err)
		}
	}

	// create tasks
	lastKvIndex := s.storageManager.LastKvIndex()
	for _, sid := range s.storageManager.Shards() {
		exist := false
		for _, t := range progress.Tasks {
			if t.Contract == s.storageManager.ContractAddress() && t.ShardId == sid {
				if states != nil {
					if state, ok := states[t.ShardId]; ok {
						state.PeerCount = 0
						t.state = state
					}
				}
				if t.state == nil {
					// TODO if t.state is nil, that mean the status is marshal by old state,
					// set process value to SyncState to make it compatible.
					// it can be removed after public test done.
					t.state = &SyncState{
						PeerCount:         0,
						BlobsToSync:       0,
						BlobsSynced:       progress.BlobsSynced,
						SyncProgress:      0,
						SyncedSeconds:     progress.TotalSecondsUsed,
						EmptyFilled:       progress.EmptyBlobsFilled,
						EmptyToFill:       0,
						FillEmptySeconds:  progress.TotalSecondsUsed,
						FillEmptyProgress: 0,
					}
				}
				s.tasks = append(s.tasks, t)
				exist = true
				continue
			}
		}
		if exist {
			continue
		}

		t := s.createTask(sid, lastKvIndex)
		s.tasks = append(s.tasks, t)
	}

	sort.Slice(s.tasks, func(i, j int) bool {
		return s.tasks[i].ShardId < s.tasks[j].ShardId
	})
}

func (s *SyncClient) createTask(sid uint64, lastKvIndex uint64) *task {
	task := task{
		Contract:       s.storageManager.ContractAddress(),
		ShardId:        sid,
		nextIdx:        0,
		statelessPeers: make(map[peer.ID]struct{}),
		state: &SyncState{
			PeerCount:         0,
			BlobsToSync:       0,
			BlobsSynced:       0,
			SyncProgress:      0,
			SyncedSeconds:     0,
			EmptyFilled:       0,
			EmptyToFill:       0,
			FillEmptySeconds:  0,
			FillEmptyProgress: 0,
		},
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
	maxTaskSize := (limit - first + s.syncerParams.SyncConcurrency - 1) / s.syncerParams.SyncConcurrency
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
		task.state.EmptyToFill = limitForEmpty - firstEmpty
		maxEmptyTaskSize := (limitForEmpty - firstEmpty + uint64(maxFillEmptyTaskTreads) - 1) / uint64(maxFillEmptyTaskTreads)
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
	s.lock.Lock()
	defer s.lock.Unlock()
	// Store the actual progress markers
	progress := &SyncProgress{
		Tasks: s.tasks,
		// TODO remote it before next test net
		BlobsSynced:      0,
		SyncedBytes:      0,
		EmptyBlobsToFill: 0,
		EmptyBlobsFilled: 0,
		TotalSecondsUsed: 0,
	}
	status, err := json.Marshal(progress)
	if err != nil {
		panic(err) // This can only fail during implementation
	}
	if err := s.db.Put(SyncTasksKey, status); err != nil {
		log.Error("Failed to store sync tasks", "err", err)
	}
	log.Debug("Save sync state to DB")

	// save sync states to DB for status reporting
	states := make(map[uint64]*SyncState)
	for _, t := range s.tasks {
		states[t.ShardId] = t.state
	}
	status, err = json.Marshal(states)
	if err != nil {
		panic(err) // This can only fail during implementation
	}
	if err := s.db.Put(SyncStatusKey, status); err != nil {
		log.Error("Failed to store sync states", "err", err)
	}
}

// saveStatusLoop marshals the remaining sync tasks into leveldb.
func (s *SyncClient) saveStatusLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.saveSyncStatus()
		case <-s.resCtx.Done():
			s.log.Info("Stopped P2P sync client save status")
			return
		}
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
			exist, first := t.healTask.hasIndexInRange(t.SubTasks[i].First, t.SubTasks[i].next)
			// if existed, min will be the smallest index in range [subTask.First, subTask.next)
			// if no exist, min will be next, so subTask.First can directly set to subTask.next
			t.SubTasks[i].First = first
			if t.SubTasks[i].done && !exist {
				t.SubTasks = append(t.SubTasks[:i], t.SubTasks[i+1:]...)
				if t.nextIdx > i {
					t.nextIdx--
				}
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
		log.Info("Storage sync done", "subTaskCount", len(s.tasks))
	}
}

func (s *SyncClient) Start() error {
	// Retrieve the previous sync status from LevelDB and abort if already synced
	s.loadSyncStatus()
	s.lock.Lock()
	s.closingPeers = false
	s.lock.Unlock()

	s.wg.Add(2)
	go s.mainLoop()
	go s.saveStatusLoop()

	return nil
}

func (s *SyncClient) AddPeer(id peer.ID, shards map[common.Address][]uint64, direction network.Direction) bool {
	s.lock.Lock()
	if _, ok := s.peers[id]; ok {
		s.log.Debug("Cannot register peer for sync duties, peer was already registered", "peer", id)
		s.lock.Unlock()
		return true
	}
	if s.closingPeers {
		s.lock.Unlock()
		return false
	}
	if !s.needThisPeer(shards) {
		s.log.Info("No need this peer, the connection would be closed later", "maxPeers", s.maxPeers,
			"Peer count", len(s.peers), "peer", id.String(), "shards", shards)
		s.metrics.IncDropPeerCount()
		s.lock.Unlock()
		return false
	}
	// add new peer routine
	pr := NewPeer(0, s.cfg.L2ChainID, id, s.newStreamFn, direction, s.syncerParams.InitRequestSize, s.storageManager.MaxKvSize(), shards)
	s.peers[id] = pr

	s.idlerPeers[id] = struct{}{}
	s.addPeerToTask(shards)
	s.metrics.IncPeerCount()
	s.lock.Unlock()

	s.notifyPeerJoin(id)
	return true
}

func (s *SyncClient) RemovePeer(id peer.ID) bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	pr, ok := s.peers[id]
	if !ok {
		s.log.Debug("Cannot remove peer from sync duties, peer was not registered", "peer", id)
		return false
	}
	pr.resCancel() // once loop exits
	delete(s.peers, id)
	s.removePeerFromTask(pr.shards)
	s.metrics.DecPeerCount()
	delete(s.idlerPeers, id)
	for _, t := range s.tasks {
		delete(t.statelessPeers, id)
	}
	return true
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
	s.report(true)
	s.saveSyncStatus()
	return nil
}

func (s *SyncClient) RequestL2Range(start, end uint64) (uint64, error) {
	for _, pr := range s.peers {
		id := rand.Uint64()
		var packet BlobsByRangePacket
		_, err := pr.RequestBlobsByRange(id, s.storageManager.ContractAddress(), start/s.storageManager.KvEntries(), start, end, &packet)
		if err != nil {
			return 0, err
		}
		_, _, inserted, err := s.onResult(packet.Blobs)
		if err != nil {
			return 0, err
		}

		return uint64(len(inserted)), nil
	}
	return 0, fmt.Errorf("no peer can be used to send requests")
}

func (s *SyncClient) FetchBlob(kvIndex uint64, commit common.Hash) ([]byte, error) {
	if len(s.peers) == 0 {
		return []byte{}, fmt.Errorf("no peer can be used to send requests")
	}
	for _, pr := range s.peers {
		var packet BlobsByListPacket
		var payload *BlobPayload = nil

		_, err := pr.RequestBlobsByList(rand.Uint64(), s.storageManager.ContractAddress(), kvIndex/s.storageManager.KvEntries(), []uint64{kvIndex}, &packet)
		if err != nil {
			log.Warn("FetchBlob failed", "error", err)
		}

		for _, val := range packet.Blobs {
			if val.BlobIndex != kvIndex {
				continue
			}
			if val.BlobCommit.Cmp(commit) != 0 {
				log.Warn("FetchBlob failed", "peer", pr.ID(), "expected commit", commit.Hex(), "actual commit", val.BlobCommit.Hex())
				continue
			}
			payload = val
		}

		if payload == nil {
			continue
		}

		decodedBlob, success := s.decodeKV(payload)
		if !success {
			continue
		}

		success = s.checkBlobCommit(decodedBlob, payload)
		if !success {
			continue
		}

		return decodedBlob, nil
	}
	return []byte{}, fmt.Errorf("fail to fetch blob from peer")
}

func (s *SyncClient) RequestL2List(indexes []uint64) (uint64, error) {
	if len(indexes) == 0 {
		return 0, nil
	}
	for _, pr := range s.peers {
		id := rand.Uint64()
		var packet BlobsByListPacket
		_, err := pr.RequestBlobsByList(id, s.storageManager.ContractAddress(), indexes[0]/s.storageManager.KvEntries(), indexes, &packet)
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

func (s *SyncClient) mainLoop() {
	defer s.wg.Done()

	s.cleanTasks()
	if !s.syncDone {
		err := s.storageManager.DownloadAllMetas(s.resCtx, s.syncerParams.MetaDownloadBatchSize)
		if err != nil {
			log.Error("Download blob metadata failed", "error", err)
			return
		}
	}

	s.logTime = time.Now()
	for {
		// Remove all completed tasks and terminate sync if everything's done
		s.cleanTasks()
		if s.syncDone {
			s.report(true)
			s.saveSyncStatus()
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
			s.log.Info("Stopped P2P req-resp L2 block sync client")
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
		maxRange := maxRequestSize / ethstorage.ContractToShardManager[t.Contract].MaxKvSize() * 2
		subTaskCount := len(t.SubTasks)
		for idx := 0; idx < subTaskCount; idx++ {
			pr := s.getIdlePeerForTask(t)
			if pr == nil {
				break
			}
			t.nextIdx = t.nextIdx % subTaskCount
			st := t.SubTasks[t.nextIdx]
			t.nextIdx++
			if st.done {
				continue
			}
			// Skip any tasks already running
			if st.isRunning {
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
					s.notifyUpdate()
				}
				s.lock.Unlock()

				if err != nil {
					if e, ok := err.(*yamux.Error); ok && e.Timeout() {
						log.Debug("Request blobs timeout", "peer", pr.id.String(), "err", err)
						pr.tracker.Update(0, 0)
					} else if returnCode == streamError && strings.Contains(err.Error(), "no addresses") {
						log.Debug("Failed to request blobs as newStream failed", "peer", pr.id.String(), "err", err)
					} else {
						log.Info("Failed to request blobs", "peer", pr.id.String(), "err", err)
					}
					return
				}

				if req.id != packet.ID || req.contract != packet.Contract || req.shardId != packet.ShardId {
					log.Info("Req mismatch with res", "reqId", req.id, "packetId", packet.ID,
						"reqContract", req.contract.Hex(), "packetContract", packet.Contract.Hex(),
						"reqShardId", req.shardId, "packetShardId", packet.ShardId)
					return
				}
				res := &blobsByRangeResponse{
					req:   req,
					Blobs: packet.Blobs,
					time:  time.Now(),
				}
				pr.tracker.Update(time.Since(req.time), len(packet.Blobs)*int(s.storageManager.MaxKvSize()))
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
		batch := maxRequestSize / ethstorage.ContractToShardManager[t.Contract].MaxKvSize() * 2

		// kvHealTask pending retrieval, try to find an idle peer. If no such peer
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
			log.Info("Peer for request no found", "contract", t.Contract.Hex(), "shardId",
				t.ShardId, "indexCount", t.healTask.count(), "peers", len(s.peers), "idlers", len(s.idlerPeers))
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
				s.notifyUpdate()
			}
			s.lock.Unlock()

			if err != nil {
				if e, ok := err.(*yamux.Error); ok && e.Timeout() {
					log.Debug("Request blobs timeout", "peer", pr.id.String(), "err", err)
					pr.tracker.Update(0, 0)
				} else if returnCode == streamError && strings.Contains(err.Error(), "no addresses") {
					log.Debug("Failed to request blobs as newStream failed", "peer", pr.id.String(), "err", err)
				} else {
					log.Info("Failed to request blobs", "peer", pr.id.String(), "err", err)
				}
				return
			}
			if req.id != packet.ID || req.contract != packet.Contract || req.shardId != packet.ShardId {
				log.Info("Req mismatch with res", "reqId", req.id, "packetId", packet.ID,
					"reqContract", req.contract.Hex(), "packetContract", packet.Contract.Hex(),
					"reqShardId", req.shardId, "packetShardId", packet.ShardId)
				return
			}
			res := &blobsByListResponse{
				req:   req,
				Blobs: packet.Blobs,
				time:  time.Now(),
			}
			pr.tracker.Update(time.Since(req.time), len(packet.Blobs)*int(s.storageManager.MaxKvSize()))
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
				next, err := s.FillFileWithEmptyBlob(start, limit)
				if err != nil {
					log.Warn("Fill in empty fail", "err", err.Error())
				}
				filled := next - start

				s.lock.Lock()
				state := eTask.task.state
				state.EmptyFilled += filled
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
	idlers := &capacitySort{
		ids:  make([]peer.ID, 0, len(s.idlerPeers)),
		caps: make([]float64, 0, len(s.idlerPeers)),
	}
	for id := range s.idlerPeers {
		if _, ok := t.statelessPeers[id]; ok {
			continue
		}
		p, ok := s.peers[id]
		if ok && p.IsShardExist(t.Contract, t.ShardId) {
			idlers.ids = append(idlers.ids, id)
			idlers.caps = append(idlers.caps, p.tracker.capacity)
		}
	}
	if len(idlers.ids) == 0 {
		return nil
	}
	sort.Sort(sort.Reverse(idlers))

	return s.peers[idlers.ids[0]]
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
	s.log.Debug("OnBlobsByRange: static", "reqId", req.id, "blobCount", len(res.Blobs), "bytes", size)

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
		s.log.Info("Peer rejected get blob by range request")
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

	s.metrics.ClientOnBlobsByRange(req.peer.String(), reqCount, uint64(len(res.Blobs)), synced, time.Since(start))
	log.Debug("Persisted set of kvs", "count", synced, "bytes", syncedBytes)

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
	last := inserted[len(inserted)-1]
	missing := make([]uint64, 0)
	for i, n := 0, res.req.subTask.next; n <= last; n++ {
		if inserted[i] == n {
			i++
		} else if inserted[i] > n {
			missing = append(missing, n)
		}
	}
	s.lock.Lock()
	state := req.subTask.task.state
	state.BlobsSynced += uint64(len(inserted))
	res.req.subTask.task.healTask.insert(missing)
	if last == res.req.subTask.Last-1 {
		res.req.subTask.done = true
	}
	res.req.subTask.next = last + 1
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
	s.log.Debug("OnBlobsByList: static", "reqId", req.id, "blobCount", len(res.Blobs), "bytes", size)

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
		s.log.Info("Peer rejected get blobs by list request")
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

	s.metrics.ClientOnBlobsByList(req.peer.String(), uint64(len(req.indexes)), uint64(len(res.Blobs)),
		synced, time.Since(start))
	log.Debug("Persisted set of kvs", "count", synced, "bytes", syncedBytes)

	s.lock.Lock()
	state := req.healTask.task.state
	state.BlobsSynced += uint64(len(inserted))
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
	var (
		st       = time.Now()
		inserted = uint64(0)
		next     = start
	)
	lastBlobIdx := s.storageManager.LastKvIndex()
	if lastBlobIdx > limit {
		return limit + 1, nil
	}

	if start < lastBlobIdx {
		start = lastBlobIdx
	}
	inserted, next, err := s.storageManager.CommitEmptyBlobs(start, limit)
	if inserted > 0 {
		s.metrics.ClientFillEmptyBlobsEvent(inserted, time.Since(st))
	}

	return next, err
}

func (s *SyncClient) Peers() []peer.ID {
	s.lock.Lock()
	defer s.lock.Unlock()

	peers := make([]peer.ID, 0, len(s.peers))
	for pr := range s.peers {
		peers = append(peers, pr)
	}

	return peers
}

// onResult is exclusively called by the main loop, and has thus direct access to the request bookkeeping state.
// This function verifies if the result is canonical, and either promotes the result or moves the result into quarantine.
func (s *SyncClient) onResult(blobs []*BlobPayload) (uint64, uint64, []uint64, error) {
	var (
		synced       uint64
		syncedBytes  uint64
		inserted     = make([]uint64, 0)
		indices      = make([]uint64, 0)
		decodedBlobs = make([][]byte, 0)
		commits      = make([]common.Hash, 0)
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

		indices = append(indices, payload.BlobIndex)
		decodedBlobs = append(decodedBlobs, decodedBlob)
		commits = append(commits, payload.BlobCommit)
	}

	inserted, err := s.commitBlobs(indices, decodedBlobs, commits)
	return synced, syncedBytes, inserted, err
}

func (s *SyncClient) decodeKV(payload *BlobPayload) ([]byte, bool) {
	recordDur := s.metrics.ClientRecordTimeUsed("decodeKv")
	defer recordDur()

	decodedBlob, found, err := s.storageManager.DecodeKV(payload.BlobIndex, payload.EncodedBlob, payload.BlobCommit,
		payload.MinerAddress, payload.EncodeType)
	if err != nil || !found {
		if err != nil {
			s.log.Error("Failed to decode", "kvIdx", payload.BlobIndex, "error", err)
		} else {
			s.log.Info("Failed to decode", "kvIdx", payload.BlobIndex, "error", "not found")
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
		s.log.Error("Get proof fail", "idx", payload.BlobIndex, "err", err.Error())
		return false
	}
	if !bytes.Equal(root[:ethstorage.HashSizeInContract], payload.BlobCommit[:ethstorage.HashSizeInContract]) {
		s.log.Info("Compare blob failed", "idx", payload.BlobIndex, "err",
			fmt.Sprintf("verify blob fail: root: %s; MetaHash hash (24): %s, providerAddr %s, data len %d",
				common.Bytes2Hex(root[:ethstorage.HashSizeInContract]), common.Bytes2Hex(payload.BlobCommit[:ethstorage.HashSizeInContract]),
				payload.MinerAddress.Hex(), len(payload.EncodedBlob)))
		return false
	}

	return true
}

func (s *SyncClient) commitBlobs(kvIndices []uint64, decodedBlobs [][]byte, commits []common.Hash) ([]uint64, error) {
	recordDur := s.metrics.ClientRecordTimeUsed("commitBlobs")
	defer recordDur()
	return s.storageManager.CommitBlobs(kvIndices, decodedBlobs, commits)
}

// report calculates various status reports and provides it to the user.
func (s *SyncClient) report(force bool) {
	duration := uint64(time.Since(s.logTime).Seconds())
	// Don't report all the events, just occasionally
	if !force && duration < 8 {
		return
	}
	s.logTime = time.Now()

	s.lock.Lock()
	defer s.lock.Unlock()

	s.reportSyncState(duration)
	s.reportFillEmptyState(duration)
}

func (s *SyncClient) reportSyncState(duration uint64) {
	for _, t := range s.tasks {
		blobsToSync := uint64(0)
		for _, st := range t.SubTasks {
			blobsToSync = blobsToSync + (st.Last - st.next)
		}
		t.state.BlobsToSync = blobsToSync + uint64(t.healTask.count())
		if t.state.BlobsSynced+t.state.BlobsToSync != 0 {
			t.state.SyncProgress = t.state.BlobsSynced * 10000 / (t.state.BlobsSynced + t.state.BlobsToSync)
		} else {
			t.state.SyncProgress = 10000
		}

		// If sync is complete, stop adding sync time
		if t.state.BlobsToSync != 0 {
			t.state.SyncedSeconds = t.state.SyncedSeconds + duration
		}

		estTime := "No estimated time"
		progress := fmt.Sprintf("%.2f%%", float64(t.state.SyncProgress)/100)
		if t.state.BlobsSynced != 0 {
			etaSecondsLeft := t.state.SyncedSeconds * t.state.BlobsToSync / t.state.BlobsSynced
			estTime = common.PrettyDuration(time.Duration(etaSecondsLeft) * time.Second).String()
		}

		log.Info("Storage sync in progress", "shardId", t.ShardId, "subTaskRemain", len(t.SubTasks), "peerCount",
			t.state.PeerCount, "progress", progress, "blobsSynced", t.state.BlobsSynced, "blobsToSync", t.state.BlobsToSync,
			"timeUsed", common.PrettyDuration(time.Duration(t.state.SyncedSeconds)*time.Second), "etaTimeLeft", estTime)
	}
}

func (s *SyncClient) reportFillEmptyState(duration uint64) {
	for _, t := range s.tasks {
		if t.state.EmptyFilled == 0 && len(t.SubEmptyTasks) == 0 {
			t.state.FillEmptyProgress = 10000
			continue
		}
		emptyToFill := uint64(0)
		for _, st := range t.SubEmptyTasks {
			emptyToFill = emptyToFill + (st.Last - st.First)
		}
		t.state.EmptyToFill = emptyToFill
		if t.state.EmptyFilled+t.state.EmptyToFill != 0 {
			t.state.FillEmptyProgress = t.state.EmptyFilled * 10000 / (t.state.EmptyFilled + t.state.EmptyToFill)
		}

		// If fill empty is complete, stop adding sync time
		if t.state.EmptyToFill != 0 {
			t.state.FillEmptySeconds = t.state.FillEmptySeconds + duration
		}

		estTime := "No estimated time"
		progress := fmt.Sprintf("%.2f%%", float64(t.state.FillEmptyProgress)/100)
		if t.state.EmptyFilled != 0 {
			etaSecondsLeft := t.state.FillEmptySeconds * t.state.EmptyToFill / t.state.EmptyFilled
			estTime = common.PrettyDuration(time.Duration(etaSecondsLeft) * time.Second).String()
		}

		log.Info("Storage fill empty in progress", "shardId", t.ShardId, "subTaskRemain", len(t.SubEmptyTasks),
			"progress", progress, "emptyFilled", t.state.EmptyFilled, "emptyToFill", t.state.EmptyToFill, "timeUsed",
			common.PrettyDuration(time.Duration(t.state.FillEmptySeconds)*time.Second), "etaTimeLeft", estTime)
	}
}

func (s *SyncClient) ReportPeerSummary() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			inbound, outbound := 0, 0
			s.lock.Lock()
			for _, p := range s.peers {
				if p.direction == network.DirInbound {
					inbound++
				} else if p.direction == network.DirOutbound {
					outbound++
				}
			}
			log.Info("P2P Summary", "activePeers", len(s.peers), "inbound", inbound, "outbound", outbound)
			s.lock.Unlock()
		case <-s.resCtx.Done():
			log.Info("P2P summary stop")
			return
		}
	}
}

func (s *SyncClient) needThisPeer(contractShards map[common.Address][]uint64) bool {
	if contractShards == nil {
		return false
	}
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
				if len(s.peers) < s.maxPeers || t.state.PeerCount < s.minPeersPerShard {
					return true
				}
			}
		}
	}

	return false
}

func (s *SyncClient) addPeerToTask(contractShards map[common.Address][]uint64) {
	for contract, shards := range contractShards {
		for _, shard := range shards {
			for _, t := range s.tasks {
				if t.Contract == contract && shard == t.ShardId {
					t.state.PeerCount++
				}
			}
		}
	}
}

func (s *SyncClient) removePeerFromTask(contractShards map[common.Address][]uint64) {
	for contract, shards := range contractShards {
		for _, shard := range shards {
			for _, t := range s.tasks {
				if t.Contract == contract && shard == t.ShardId {
					t.state.PeerCount--
				}
			}
		}
	}
}
