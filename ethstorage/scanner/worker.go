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
	LastKvIndex() uint64
	Shards() []uint64
}

type Worker struct {
	sm               IStorageManager
	fetchBlob        es.FetchBlobFunc
	l1               es.Il1Source
	cfg              Config
	nextIndexOfKvIdx uint64
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
		sm:        sm,
		fetchBlob: fetch,
		l1:        l1,
		cfg:       cfg,
		lg:        lg,
	}
}

func (s *Worker) ScanBatch(ctx context.Context, sendError func(kvIndex uint64, err error)) (*stats, error) {
	// Query local storage info
	shards := s.sm.Shards()
	kvEntries := s.sm.KvEntries()
	// LastKvIndex() actually returns the total number of kv entries stored in the contract
	entryCount := s.sm.LastKvIndex()
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
			s.lg.Debug("Scanner: check KV done", "kvIndex", kvIndex, "commit", commit)
			continue
		}

		if !found {
			// The shard does not exist locally
			s.lg.Error("Scanner: blob not found", "kvIndex", kvIndex, "commit", commit)
			continue
		}

		if err != nil {
			var commitErr *es.CommitMismatchError
			if errors.As(err, &commitErr) {
				s.lg.Warn("Scanner: commit mismatch found", "kvIndex", kvIndex, "error", err)
				sts.mismatched = append(sts.mismatched, kvIndex)
				if fixErr := s.fixKv(kvIndex, commit); fixErr != nil {
					sts.failed = append(sts.failed, kvIndex)
					s.lg.Error("Scanner: fix blob error", "kvIndex", kvIndex, "error", fixErr)
					sendError(kvIndex, fmt.Errorf("failed to fix blob: %w", fixErr))
				} else {
					sts.fixed = append(sts.fixed, kvIndex)
					sendError(kvIndex, nil) // Clear the error
				}
			} else {
				s.lg.Error("Scanner: unexpected error", "kvIndex", kvIndex, "error", err)
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
	s.lg.Info("Scanner: try to fix blob", "kvIndex", kvIndex, "commit", commit)
	if err := s.sm.TryWriteWithMetaCheck(kvIndex, commit, s.fetchBlob); err != nil {
		return fmt.Errorf("failed to write KV: kvIndex=%d, commit=%x, %w", kvIndex, commit, err)
	}
	s.lg.Info("Scanner: fixed blob successfully!", "kvIndex", kvIndex)
	return nil
}

func getKvsInBatch(shards []uint64, kvEntries, lastKvIdx, batchSize, batchStartIndex uint64, lg log.Logger) ([]uint64, uint64, uint64) {
	// Calculate the total number of KV entries stored locally
	var totalEntries uint64
	// The shard indices in shards are sorted but may not be continuous: e.g. [0, 1, 3, 4] means shard 2 is missing.
	for _, shardIndex := range shards {
		// the last shard may not have full kvEntries
		if shardIndex == lastKvIdx/kvEntries {
			totalEntries += lastKvIdx%kvEntries + 1
			break
		}
		// full shards
		totalEntries += kvEntries
	}
	lg.Debug("Scanner: KV entries stored locally", "totalKvStored", totalEntries)

	// Determine batch start and end KV indices
	startKvIndex := batchStartIndex
	if startKvIndex >= totalEntries {
		startKvIndex = 0
		lg.Info("Scanner: restarting scan from beginning")
	}
	endKvIndexExclusive := startKvIndex + batchSize
	if endKvIndexExclusive > totalEntries {
		endKvIndexExclusive = totalEntries
	}
	// The actual batch would be [startKvIndex, endKvIndexExclusive) or [startKvIndex, endIndex]
	endIndex := endKvIndexExclusive - 1

	// Calculate starting and ending shard indices and offsets where KV starts and ends on the current shard.
	startShardPos := startKvIndex / kvEntries
	startKvOffset := startKvIndex % kvEntries

	endShardPos := endIndex / kvEntries
	endKvOffset := endIndex%kvEntries + 1 // +1 because the end KV is exclusive

	// Collect KV indices for the current batch
	kvsInBatch := make([]uint64, 0, endKvIndexExclusive-startKvIndex)
	// Loop through the shards from startShardPos to endShardPos(inclusive)
	for i := startShardPos; i <= endShardPos; i++ {
		// Default range of the full shards
		localStart := uint64(0)
		localEnd := kvEntries
		// If the shard is the first or last shard in the batch, adjust the start and end offsets
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
	lg.Info("Scanner: batch index range", "batchStart", startKvIndex, "batchEnd(exclusive)", endKvIndexExclusive, "kvsInBatch", shortPrt(kvsInBatch))
	return kvsInBatch, totalEntries, endKvIndexExclusive
}
