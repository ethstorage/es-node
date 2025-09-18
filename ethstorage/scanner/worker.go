// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"context"
	"errors"
	"fmt"

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
	sm               IStorageManager
	fetchBlob        es.FetchBlobFunc
	l1               es.Il1Source
	cfg              Config
	nextIndexOfKvIdx uint64
	mismatching      map[uint64]struct{} // kv indices that are mismatching (not fixed yet)
	lg               log.Logger
}

func NewWorker(
	sm IStorageManager,
	fetch es.FetchBlobFunc,
	l1 es.Il1Source,
	cfg Config,
	lg log.Logger,
) *Worker {
	return &Worker{
		sm:          sm,
		fetchBlob:   fetch,
		l1:          l1,
		cfg:         cfg,
		mismatching: make(map[uint64]struct{}),
		lg:          lg,
	}
}

func (s *Worker) ScanBatch(ctx context.Context, sendError func(kvIndex uint64, err error)) (*stats, error) {
	// Query local storage info
	shards := s.sm.Shards()
	kvEntries := s.sm.KvEntries()
	entryCount := s.sm.KvEntryCount()
	if entryCount == 0 {
		s.lg.Info("Scanner: no KV entries found in local storage")
		return nil, nil
	}
	lastKvIdx := entryCount - 1
	s.lg.Info("Scanner: local storage info", "lastKvIdx", lastKvIdx, "shards", shards, "kvEntriesPerShard", kvEntries)

	// Determine the batch of KV indices to scan
	kvsInBatch, totalEntries, batchEndExclusive := getKvsInBatch(shards, kvEntries, lastKvIdx, uint64(s.cfg.BatchSize), s.nextIndexOfKvIdx, s.lg)
	sts := stats{}
	sts.localKvs = summaryLocalKvs(shards, kvEntries, lastKvIdx)
	sts.total = int(totalEntries)

	// Query the metas from the L1 contract
	metas, err := s.l1.GetKvMetas(kvsInBatch, rpc.FinalizedBlockNumber.Int64())
	if err != nil {
		s.lg.Error("Scanner: failed to query KV metas", "error", err)
		return nil, fmt.Errorf("failed to query KV metas: %w", err)
	}
	s.lg.Debug("Scanner: query KV meta done", "kvsInBatch", shortPrt(kvsInBatch))

	for i, meta := range metas {
		select {
		case <-ctx.Done():
			s.lg.Warn("Scanner canceled, stopping scan", "ctx.Err", ctx.Err())
			return nil, ctx.Err()
		default:
		}

		var found bool
		var commit common.Hash
		copy(commit[:], meta[32-es.HashSizeInContract:32])
		kvIndex := kvsInBatch[i]
		if s.cfg.Mode == modeCheckMeta {
			// Check meta only
			var metaLocal []byte
			metaLocal, found, err = s.sm.TryReadMeta(kvIndex)
			if err != nil {
				s.lg.Error("Scanner: failed to read meta", "kvIndex", kvIndex, "error", err)
				sendError(kvIndex, fmt.Errorf("failed to read meta: %w", err))
				continue
			}
			err = es.CompareCommits(commit.Bytes(), metaLocal)
		} else if s.cfg.Mode == modeCheckBlob {
			// Query blob and check meta from storage
			_, found, err = s.sm.TryRead(kvIndex, int(s.sm.MaxKvSize()), commit)
		} else {
			s.lg.Error("Scanner: invalid scanner mode", "mode", s.cfg.Mode)
			return nil, fmt.Errorf("invalid scanner mode: %d", s.cfg.Mode)
		}

		if found && err == nil {
			// Happy path
			s.lg.Debug("Scanner: KV check completed successfully", "kvIndex", kvIndex, "commit", commit)
			continue
		}

		if !found {
			// The shard is not stored locally
			s.lg.Error("Scanner: blob not found locally", "kvIndex", kvIndex, "commit", commit)
			continue
		}

		if err != nil {
			var commitErr *es.CommitMismatchError
			if errors.As(err, &commitErr) {
				s.lg.Warn("Scanner: commit mismatch detected", "kvIndex", kvIndex, "error", err)
				if _, ok := s.mismatching[kvIndex]; !ok {
					// Skip on first occurrence as it may be caused by KV update and delayed download
					s.mismatching[kvIndex] = struct{}{}
					s.lg.Debug("Scanner: first-time mismatch, skipping fix attempt", "kvIndex", kvIndex)
				} else {
					sts.mismatched = append(sts.mismatched, kvIndex)
					s.lg.Info("Scanner: attempting to fix blob", "kvIndex", kvIndex, "commit", commit)
					if fixErr := s.fixKv(kvIndex, commit); fixErr != nil {
						sts.failed = append(sts.failed, kvIndex)
						s.lg.Error("Scanner: failed to fix blob", "kvIndex", kvIndex, "error", fixErr)
						sendError(kvIndex, fmt.Errorf("failed to fix blob: %w", fixErr))
					} else {
						s.lg.Info("Scanner: blob fixed successfully", "kvIndex", kvIndex)
						sts.fixed = append(sts.fixed, kvIndex)
						// Clear the error state
						sendError(kvIndex, nil)
						delete(s.mismatching, kvIndex)
					}
				}
			} else {
				s.lg.Error("Scanner: unexpected error occurred", "kvIndex", kvIndex, "error", err)
				sendError(kvIndex, fmt.Errorf("unexpected error: %w", err))
			}
		}
	}

	s.nextIndexOfKvIdx = batchEndExclusive
	if len(kvsInBatch) > 0 {
		s.lg.Info("Scanner: scan batch done", "scanned", shortPrt(kvsInBatch), "count", len(kvsInBatch), "nextIndexOfKvIdx", s.nextIndexOfKvIdx)
	}
	return &sts, nil
}

func (s *Worker) fixKv(kvIndex uint64, commit common.Hash) error {
	if err := s.sm.TryWriteWithMetaCheck(kvIndex, commit, s.fetchBlob); err != nil {
		return fmt.Errorf("failed to write KV: kvIndex=%d, commit=%x, %w", kvIndex, commit, err)
	}
	return nil
}

func getKvsInBatch(shards []uint64, kvEntries, lastKvIdx, batchSize, batchStartIndex uint64, lg log.Logger) ([]uint64, uint64, uint64) {
	// Calculate the total number of KV entries stored locally
	var totalEntries uint64
	// Shard indices are sorted but may not be continuous: e.g. [0, 1, 3, 4] indicates shard 2 is missing
	for _, shardIndex := range shards {
		// The last shard may contain fewer than the full kvEntries
		if shardIndex == lastKvIdx/kvEntries {
			totalEntries += lastKvIdx%kvEntries + 1
			break
		}
		// Complete shards
		totalEntries += kvEntries
	}
	lg.Debug("Scanner: KV entries stored locally", "totalKvStored", totalEntries)

	// Determine batch start and end KV indices
	startKvIndex := batchStartIndex
	if startKvIndex >= totalEntries {
		startKvIndex = 0
		lg.Debug("Scanner: restarting scan from the beginning")
	}
	endKvIndexExclusive := min(startKvIndex+batchSize, totalEntries)
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
	return kvsInBatch, totalEntries, endKvIndexExclusive
}
