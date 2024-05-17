// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/time/rate"
)

const (
	returnCodeSuccess = iota
	returnCodeReadError
	returnCodeInvalidRequest
	returnCodeServerError
)

const (
	// Do not serve more than 20 requests per second
	globalServerBlocksRateLimit rate.Limit = 20
	// Allow up to 30 concurrent requests to be served, eating into our rate-limit
	globalServerBlocksBurst = 30
	// Do not serve more than 5 requests per second to the same peer, so we can serve other peers at the same time
	peerServerBlocksRateLimit rate.Limit = 5
	// Allow a peer to burst 10 requests, so it does not have to wait
	peerServerBlocksBurst = 10

	// maxMessageSize is the target maximum size of replies to data retrievals.
	maxMessageSize = 8 * 1024 * 1024
)

var (
	ProvidedBlobsKey = []byte("ProvidedBlobsKey")
)

// peerStat maintains rate-limiting data of a peer that requests blocks from us.
type peerStat struct {
	// Requests tokenizes each request to sync
	Requests *rate.Limiter
}

type SyncServerMetrics interface {
	ServerGetBlobsByRangeEvent(peerID string, resultCode byte, duration time.Duration)
	ServerGetBlobsByListEvent(peerID string, resultCode byte, duration time.Duration)
	ServerReadBlobs(peerID string, read, sucRead uint64, timeUse time.Duration)
	ServerRecordTimeUsed(method string) func()
}

type SyncServer struct {
	cfg *rollup.EsConfig

	providedBlobs  map[uint64]uint64
	storageManager StorageManagerReader
	db             ethdb.Database
	metrics        SyncServerMetrics
	exitCh         chan struct{}

	peerRateLimits *simplelru.LRU[peer.ID, *peerStat]
	peerStatsLock  sync.Mutex

	globalRequestsRL *rate.Limiter

	lock sync.Mutex
}

func NewSyncServer(cfg *rollup.EsConfig, storageManager StorageManagerReader, db ethdb.Database, m SyncServerMetrics) *SyncServer {
	// We should never allow over 1000 different peers to churn through quickly,
	// so it's fine to prune rate-limit details past this.

	peerRateLimits, _ := simplelru.NewLRU[peer.ID, *peerStat](1000, nil)
	// 3 sync requests per second, with 2 burst
	globalRequestsRL := rate.NewLimiter(globalServerBlocksRateLimit, globalServerBlocksBurst)

	if m == nil {
		m = metrics.NoopMetrics
	}
	var providedBlobs map[uint64]uint64
	if status, _ := db.Get(ProvidedBlobsKey); status != nil {
		if err := json.Unmarshal(status, &providedBlobs); err != nil {
			log.Error("Failed to decode provided blobs", "err", err)
		}
	}

	server := SyncServer{
		cfg:              cfg,
		storageManager:   storageManager,
		db:               db,
		providedBlobs:    make(map[uint64]uint64),
		exitCh:           make(chan struct{}),
		metrics:          m,
		peerRateLimits:   peerRateLimits,
		globalRequestsRL: globalRequestsRL,
	}

	for _, shardId := range storageManager.Shards() {
		if providedBlobs != nil {
			if blobs, ok := providedBlobs[shardId]; ok {
				server.providedBlobs[shardId] = blobs
				continue
			}
		}
		server.providedBlobs[shardId] = 0
	}
	go server.SaveProvidedBlobs()
	return &server
}

// HandleGetBlobsByRangeRequest is a stream handler function to register the L2 unsafe payloads alt-sync protocol.
// See MakeStreamHandler to transform this into a LibP2P handler function.
//
// Note that the same peer may open parallel streams.
//
// The caller must Close the stream.
func (srv *SyncServer) HandleGetBlobsByRangeRequest(ctx context.Context, log log.Logger, stream network.Stream) {
	// We wait as long as necessary; we throttle the peer instead of disconnecting,
	// unless the delay reaches a threshold that is unreasonable to wait for.
	ctx, cancel := context.WithTimeout(ctx, maxThrottleDelay)
	start := time.Now()
	returnCode, data, err := srv.handleGetBlobsByRangeRequest(ctx, stream)
	srv.metrics.ServerGetBlobsByRangeEvent(stream.Conn().RemotePeer().String(), returnCode, time.Since(start))
	cancel()

	if err != nil {
		log.Warn("Failed to serve p2p sync request", "err", err)
	}
	err = WriteMsg(stream, &Msg{returnCode, data})
	if err != nil {
		log.Debug("write message fail", "err", err.Error())
	} else {
		log.Debug("Sent response for func HandleGetBlobsByRangeRequest", "returnCode", returnCode, "len(Bytes)", len(data), "peer", stream.Conn().RemotePeer().String())
	}
}

func (srv *SyncServer) HandleGetBlobsByListRequest(ctx context.Context, log log.Logger, stream network.Stream) {
	// We wait as long as necessary; we throttle the peer instead of disconnecting,
	// unless the delay reaches a threshold that is unreasonable to wait for.
	ctx, cancel := context.WithTimeout(ctx, maxThrottleDelay)
	start := time.Now()
	returnCode, data, err := srv.handleGetBlobsByListRequest(ctx, stream)
	srv.metrics.ServerGetBlobsByListEvent(stream.Conn().RemotePeer().String(), returnCode, time.Since(start))
	cancel()

	if err != nil {
		log.Warn("Failed to serve p2p sync request", "err", err)
	}
	err = WriteMsg(stream, &Msg{returnCode, data})
	if err != nil {
		log.Debug("write message fail", "err", err.Error())
	} else {
		log.Debug("Sent response for func HandleGetBlobsByListRequest", "returnCode", returnCode, "len(Bytes)", len(data), "peer", stream.Conn().RemotePeer().String())
	}
}

func (srv *SyncServer) handleGetBlobsByRangeRequest(ctx context.Context, stream network.Stream) (byte, []byte, error) {
	peerID := stream.Conn().RemotePeer()

	err := srv.limitPeer(ctx, peerID)
	if err != nil {
		return returnCodeServerError, []byte{}, err
	}

	msg, _, err := ReadMsg(stream)
	if err != nil {
		return returnCodeReadError, []byte{}, fmt.Errorf("read msg from stream fail: %w", err)
	}

	var req GetBlobsByRangePacket
	if err := rlp.DecodeBytes(msg, &req); err != nil {
		return returnCodeInvalidRequest, []byte{}, fmt.Errorf("decode message fail, msg: %v, error: %v", common.Bytes2Hex(msg), err)
	}

	res := BlobsByRangePacket{
		ID:       req.ID,
		Contract: req.Contract,
		ShardId:  req.ShardId,
		Blobs:    make([]*BlobPayload, 0),
	}
	read, sucRead, readBytes := uint64(0), uint64(0), uint64(0)
	start := time.Now()
	for id := req.Origin; id <= req.Limit; id++ {
		payload, err := srv.BlobByIndex(id)
		read++
		if err != nil {
			log.Debug("Get blob fail", "id", id, "error", err.Error())
			continue
		}
		sucRead++
		res.Blobs = append(res.Blobs, payload)
		readBytes += uint64(len(payload.EncodedBlob))
		if uint64(len(res.Blobs)) >= req.Size || readBytes >= maxMessageSize {
			break
		}
	}
	srv.metrics.ServerReadBlobs(peerID.String(), read, sucRead, time.Since(start))
	srv.lock.Lock()
	srv.providedBlobs[req.ShardId] += uint64(len(res.Blobs))
	srv.lock.Unlock()

	recordDur := srv.metrics.ServerRecordTimeUsed("encodeResult")
	data, err := rlp.EncodeToBytes(&res)
	recordDur()
	if err != nil {
		return returnCodeServerError, []byte{}, fmt.Errorf("failed to write payload to sync response: %w", err)
	}

	return returnCodeSuccess, data, nil
}

func (srv *SyncServer) handleGetBlobsByListRequest(ctx context.Context, stream network.Stream) (byte, []byte, error) {
	peerID := stream.Conn().RemotePeer()

	err := srv.limitPeer(ctx, peerID)
	if err != nil {
		return returnCodeServerError, []byte{}, err
	}

	msg, _, err := ReadMsg(stream)
	if err != nil {
		return returnCodeReadError, []byte{}, fmt.Errorf("read msg from stream fail: %w", err)
	}

	var req GetBlobsByListPacket
	if err := rlp.DecodeBytes(msg, &req); err != nil {
		return returnCodeInvalidRequest, []byte{}, fmt.Errorf("decode message fail, msg: %v, error: %v", common.Bytes2Hex(msg), err)
	}

	res := BlobsByListPacket{
		ID:       req.ID,
		Contract: req.Contract,
		ShardId:  req.ShardId,
		Blobs:    make([]*BlobPayload, 0),
	}
	read, sucRead, readBytes := uint64(0), uint64(0), uint64(0)
	start := time.Now()
	for _, idx := range req.BlobList {
		payload, err := srv.BlobByIndex(idx)
		read++
		if err != nil {
			log.Debug("Get blob fail", "idx", idx, "error", err.Error())
			continue
		}
		sucRead++
		res.Blobs = append(res.Blobs, payload)
		readBytes += uint64(len(payload.EncodedBlob))
		if uint64(len(res.Blobs)) >= req.Size || readBytes >= maxMessageSize {
			break
		}
	}
	srv.metrics.ServerReadBlobs(peerID.String(), read, sucRead, time.Since(start))
	srv.lock.Lock()
	srv.providedBlobs[req.ShardId] += uint64(len(res.Blobs))
	srv.lock.Unlock()

	recordDur := srv.metrics.ServerRecordTimeUsed("encodeResult")
	data, err := rlp.EncodeToBytes(&res)
	recordDur()
	if err != nil {
		return returnCodeServerError, []byte{}, fmt.Errorf("failed to write payload to sync response: %w", err)
	}

	return returnCodeSuccess, data, nil
}

func (srv *SyncServer) limitPeer(ctx context.Context, peerId peer.ID) error {
	// take a token from the global rate-limiter,
	// to make sure there's not too much concurrent server work between different peers.
	if err := srv.globalRequestsRL.Wait(ctx); err != nil {
		return fmt.Errorf("timed out waiting for global sync rate limit: %w", err)
	}

	// find rate limiting data of peer, or add otherwise
	srv.peerStatsLock.Lock()
	defer srv.peerStatsLock.Unlock()
	ps, _ := srv.peerRateLimits.Get(peerId)
	if ps == nil {
		ps = &peerStat{
			Requests: rate.NewLimiter(peerServerBlocksRateLimit, peerServerBlocksBurst),
		}
		srv.peerRateLimits.Add(peerId, ps)
		ps.Requests.Reserve() // count the hit, but make it delay the next request rather than immediately waiting
	} else {
		// Only wait if it's an existing peer, otherwise the instant rate-limit Wait call always errors.

		// If the requester thinks we're taking too long, then it's their problem and they can disconnect.
		// We'll disconnect ourselves only when failing to read/write,
		// if the work is invalid (range validation), or when individual sub tasks timeout.
		if err := ps.Requests.Wait(ctx); err != nil {
			return fmt.Errorf("timed out waiting for global sync rate limit: %w", err)
		}
	}

	return nil
}

func (srv *SyncServer) BlobByIndex(idx uint64) (*BlobPayload, error) {
	recordDur := srv.metrics.ServerRecordTimeUsed("readBlobByIndex")
	defer recordDur()

	shardIdx := idx / srv.storageManager.KvEntries()
	blob, found, err := srv.storageManager.TryReadEncoded(idx, int(srv.storageManager.MaxKvSize()))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ethereum.NotFound
	}
	commit, _, err := srv.storageManager.TryReadMeta(idx)
	if err != nil {
		return nil, err
	}

	miner, _ := srv.storageManager.GetShardMiner(shardIdx)
	encodeType, _ := srv.storageManager.GetShardEncodeType(shardIdx)
	return &BlobPayload{
		MinerAddress: miner,
		BlobIndex:    idx,
		BlobCommit:   common.BytesToHash(commit),
		EncodeType:   encodeType,
		EncodedBlob:  blob,
	}, nil
}

func (srv *SyncServer) HandleRequestShardList(ctx context.Context, log log.Logger, stream network.Stream) {
	rCode := byte(0)
	bs, err := rlp.EncodeToBytes(ConvertToContractShards(ethstorage.Shards()))
	if err != nil {
		log.Warn("Encode shard list fail", "err", err.Error())
		rCode = returnCodeServerError
	}

	err = WriteMsg(stream, &Msg{rCode, bs})
	if err != nil {
		log.Warn("Write response failed for HandleRequestShardList", "err", err.Error())
	}
	log.Debug("Write response done for HandleRequestShardList")
}

func (srv *SyncServer) saveProvidedBlobs() {
	srv.lock.Lock()
	states, err := json.Marshal(srv.providedBlobs)
	srv.lock.Unlock()
	if err != nil {
		log.Error("Failed to marshal provided blobs states", "err", err)
		return
	}

	err = srv.db.Put(ProvidedBlobsKey, states)
	if err != nil {
		log.Error("Failed to store provided blobs states", "err", err)
		return
	}
}

func (srv *SyncServer) SaveProvidedBlobs() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			srv.saveProvidedBlobs()
		case <-srv.exitCh:
			log.Info("Stopped P2P req-resp L2 block sync server")
			return
		}
	}
}

func (srv *SyncServer) Close() {
	close(srv.exitCh)
	srv.saveProvidedBlobs()
}
