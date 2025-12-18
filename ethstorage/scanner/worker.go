// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"

	es "github.com/ethstorage/go-ethstorage/ethstorage"
)

type IStorageManager interface {
	TryRead(kvIdx uint64, readLen int, commit common.Hash) ([]byte, bool, error)
	TryReadMeta(kvIdx uint64) ([]byte, bool, error)
	TryWriteWithMetaCheck(kvIdx uint64, commit common.Hash, fetchBlob es.FetchBlobFunc) error
	MaxKvSize() uint64
	KvEntries() uint64
	KvEntryCount() uint64
	Shards() []uint64
}

type Worker struct {
	sm        IStorageManager
	fetchBlob es.FetchBlobFunc
	l1        es.Il1Source
	batchSize uint64
	lg        log.Logger
}

func NewWorker(
	sm IStorageManager,
	fetch es.FetchBlobFunc,
	l1 es.Il1Source,
	batchSize uint64,
	lg log.Logger,
) *Worker {
	return &Worker{
		sm:        sm,
		fetchBlob: fetch,
		l1:        l1,
		batchSize: batchSize,
		lg:        lg,
	}
}

func (s *Worker) ScanBatch(ctx context.Context, state *scanLoopState, onUpdate scanUpdateFn) error {
	if onUpdate == nil {
		onUpdate = func(scanUpdate) {}
	}

	start := time.Now()
	var kvsInBatch []uint64
	defer func(stt time.Time) {
		if len(kvsInBatch) > 0 {
			s.lg.Info("Scanner: scan batch done", "mode", state.mode, "scanned", shortPrt(kvsInBatch), "count", len(kvsInBatch), "nextIndexOfKvIdx", state.nextIndex, "duration", time.Since(stt).String())
		}
	}(start)

	localKvCount, _ := s.summaryLocalKvs()
	if localKvCount == 0 {
		s.lg.Info("Scanner: no KV entries found in local storage")
		return nil
	}
	// Query local storage info
	shards := s.sm.Shards()
	kvEntries := s.sm.KvEntries()
	lastKvIdx := s.sm.KvEntryCount() - 1
	startIndexOfKvIdx := state.nextIndex
	s.lg.Info("Scanner: scan batch started", "mode", state.mode, "startIndexOfKvIdx", startIndexOfKvIdx, "lastKvIdxOnChain", lastKvIdx, "shardsInLocal", shards)
	// Determine the batch of KV indices to scan
	kvsInBatch, batchEndExclusive := getKvsInBatch(shards, kvEntries, localKvCount, s.batchSize, startIndexOfKvIdx, s.lg)

	// Query the metas from the L1 contract
	metas, err := s.l1.GetKvMetas(kvsInBatch, rpc.FinalizedBlockNumber.Int64())
	if err != nil {
		s.lg.Error("Scanner: failed to query KV metas", "error", err)
		return fmt.Errorf("failed to query KV metas: %w", err)
	}
	s.lg.Debug("Scanner: query KV meta done", "kvsInBatch", shortPrt(kvsInBatch))

	for i, meta := range metas {
		select {
		case <-ctx.Done():
			s.lg.Warn("Scanner canceled, stopping scan", "ctx.Err", ctx.Err())
			return ctx.Err()
		default:
		}

		var commit common.Hash
		copy(commit[:], meta[32-es.HashSizeInContract:32])
		kvIndex := kvsInBatch[i]
		s.scanKv(state.mode, kvIndex, commit, onUpdate)
	}

	state.nextIndex = batchEndExclusive
	return nil
}

func (s *Worker) scanKv(mode scanMode, kvIndex uint64, commit common.Hash, onUpdate scanUpdateFn) {
	var err error
	var found bool
	switch mode {
	case modeCheckMeta:
		// Check meta only
		var metaLocal []byte
		metaLocal, found, err = s.sm.TryReadMeta(kvIndex)
		if err != nil {
			s.lg.Error("Scanner: failed to read meta", "kvIndex", kvIndex, "error", err)
			errWrapped := fmt.Errorf("failed to read meta: %w", err)
			onUpdate(scanUpdate{kvIndex: kvIndex, err: errWrapped})
			return
		}
		err = es.CompareCommits(commit.Bytes(), metaLocal)
	case modeCheckBlob:
		// Query blob and check meta from storage
		_, found, err = s.sm.TryRead(kvIndex, int(s.sm.MaxKvSize()), commit)
	default:
		// Other modes are handled outside
		s.lg.Crit("Scanner: invalid scanner mode", "mode", mode)
	}

	if !found {
		// The shard is not stored locally
		errWrapped := fmt.Errorf("shard not found locally: commit=%x", commit)
		onUpdate(scanUpdate{kvIndex: kvIndex, err: errWrapped})
		s.lg.Error("Scanner: blob not found locally", "kvIndex", kvIndex, "commit", commit)
		return
	}

	if err == nil {
		// Happy path
		s.lg.Debug("Scanner: KV check completed successfully", "kvIndex", kvIndex, "commit", commit)
		return
	}

	var commitErr *es.CommitMismatchError
	if errors.As(err, &commitErr) {
		s.lg.Warn("Scanner: commit mismatch detected", "kvIndex", kvIndex, "error", err)
		newStatus := pending
		onUpdate(scanUpdate{kvIndex: kvIndex, status: &newStatus, err: nil})
		return
	}

	s.lg.Error("Scanner: unexpected error occurred", "kvIndex", kvIndex, "error", err)
	errWrapped := fmt.Errorf("unexpected error: %w", err)
	onUpdate(scanUpdate{kvIndex: kvIndex, err: errWrapped})
}

func (s *Worker) summaryLocalKvs() (uint64, string) {
	kvEntryCountOnChain := s.sm.KvEntryCount()
	if kvEntryCountOnChain == 0 {
		s.lg.Info("Scanner: no KV entries found in local storage")
		return 0, "(none)"
	}
	return summaryLocalKvs(s.sm.Shards(), s.sm.KvEntries(), kvEntryCountOnChain-1)
}

func getKvsInBatch(shards []uint64, kvEntries, localKvCount, batchSize, startKvIndex uint64, lg log.Logger) ([]uint64, uint64) {
	// Determine batch start and end KV indices
	if startKvIndex >= localKvCount {
		startKvIndex = 0
		lg.Debug("Scanner: restarting scan from the beginning")
	}
	endKvIndexExclusive := min(startKvIndex+batchSize, localKvCount)
	// The actual batch range is [startKvIndex, endKvIndexExclusive) or [startKvIndex, endIndex]
	endIndex := endKvIndexExclusive - 1

	// Calculate starting and ending shard indices and offsets where KV starts and ends on the current shard.
	startShardPos := startKvIndex / kvEntries
	startKvOffset := startKvIndex % kvEntries

	endShardPos := endIndex / kvEntries
	endKvOffset := endIndex%kvEntries + 1 // +1 because the end KV index is exclusive

	// Collect KV indices for the current batch
	kvsInBatch := make([]uint64, 0, endKvIndexExclusive-startKvIndex)
	// Iterate through shards from startShardPos to endShardPos (inclusive)
	for i := startShardPos; i <= endShardPos; i++ {
		// Default range for complete shards
		localStart := uint64(0)
		localEnd := kvEntries
		// Adjust start and end offsets for the first or last shard in the batch
		if i == startShardPos {
			localStart = startKvOffset
		}
		if i == endShardPos {
			localEnd = endKvOffset
		}
		for k := localStart; k < localEnd; k++ { // shards[shardPos]=shardIndex
			kvsInBatch = append(kvsInBatch, shards[i]*kvEntries+k)
		}
	}
	lg.Debug("Scanner: batch index range determined", "batchStart", startKvIndex, "batchEnd(exclusive)", endKvIndexExclusive, "kvsInBatch", shortPrt(kvsInBatch))
	return kvsInBatch, endKvIndexExclusive
}

// Calculate the total number of KV entries stored locally
func summaryLocalKvs(shards []uint64, kvEntries, lastKvIdx uint64) (uint64, string) {
	var totalEntries uint64
	var res []string
	// Shard indices are sorted but may not be continuous: e.g. [0, 1, 3, 4] indicates shard 2 is missing
	for _, shard := range shards {
		shardOfLastKv := lastKvIdx / kvEntries
		if shard > shardOfLastKv {
			// Skip empty shards
			break
		}
		var lastEntry uint64
		// The last shard may contain fewer than the full kvEntries
		if shard == shardOfLastKv {
			totalEntries += lastKvIdx%kvEntries + 1
			lastEntry = lastKvIdx
		} else {
			// Complete shards
			totalEntries += kvEntries
			lastEntry = (shard+1)*kvEntries - 1
		}
		shardView := fmt.Sprintf("shard%d%s", shard, formatRange(shard*kvEntries, lastEntry))
		res = append(res, shardView)
	}
	return totalEntries, strings.Join(res, ",")
}
