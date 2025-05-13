// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"bytes"
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
	loadKvFromCache  LoadKvFromCacheFunc
	fetchBlob        es.FetchBlobFunc
	l1               es.Il1Source
	cfg              Config
	nextIndexOfKvIdx uint64
	lg               log.Logger
}

func NewWorker(
	sm IStorageManager,
	load LoadKvFromCacheFunc,
	fetch es.FetchBlobFunc,
	l1 es.Il1Source,
	cfg Config,
	lg log.Logger,
) *Worker {
	return &Worker{
		sm:              sm,
		loadKvFromCache: load,
		fetchBlob:       fetch,
		l1:              l1,
		cfg:             cfg,
		lg:              lg,
	}
}

func (s *Worker) ScanBatch(ctx context.Context, sendError func(kvIndex uint64, err error)) (*stats, error) {
	kvsInBatch, totalEntries, batchEnd := s.getKvsInBatch()
	sts := stats{}
	sts.total = int(totalEntries)

	// Query the metas from the L1 contract
	metas, err := s.l1.GetKvMetas(kvsInBatch, rpc.FinalizedBlockNumber.Int64())
	if err != nil {
		s.lg.Error("Scanner: failed to query KV metas", "error", err)
		return nil, fmt.Errorf("failed to query KV metas: %w", err)
	}
	s.lg.Info("Scanner: query KV meta done", "kvsInBatch", shortPrt(kvsInBatch))

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
			if metaLocal != nil && !bytes.Equal(metaLocal[0:es.HashSizeInContract], commit[0:es.HashSizeInContract]) {
				err = es.NewCommitMismatchError(commit, common.BytesToHash(metaLocal))
			}
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
			s.lg.Warn("Scanner: blob not found", "kvIndex", kvIndex, "commit", commit)
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

	s.nextIndexOfKvIdx = batchEnd
	if len(kvsInBatch) > 0 {
		s.lg.Info("Scanner: scan batch done", "scanned", shortPrt(kvsInBatch), "count", len(kvsInBatch), "nextIndexOfKvIdx", s.nextIndexOfKvIdx)
	}
	return &sts, nil
}

func (s *Worker) getKvsInBatch() ([]uint64, uint64, uint64) {
	shards := s.sm.Shards()
	kvEntries := s.sm.KvEntries()
	// LastKvIndex() actually returns the total number of kv entries stored in the contract
	lastKvIdx := s.sm.LastKvIndex() - 1
	s.lg.Info("Scanner: scan batch started", "lastKvIdx", lastKvIdx, "shards", shards, "kvEntriesPerShard", kvEntries)

	// Calculate the total number of KV entries stored locally
	var totalEntries uint64
	for _, shardId := range shards {
		if shardId == lastKvIdx/kvEntries {
			totalEntries += (lastKvIdx + 1) % kvEntries
			break
		}
		totalEntries += kvEntries
	}
	summary := summaryLocalKvs(shards, kvEntries, lastKvIdx)
	s.lg.Info("Scanner: determining batch index range", "localKvs", summary, "totalKvStored", totalEntries)

	// Determine batch start and end indices
	batchStart := s.nextIndexOfKvIdx
	if batchStart >= totalEntries {
		batchStart = 0
		s.lg.Info("Scanner: scan batch start over")
	}
	batchEnd := batchStart + uint64(s.cfg.BatchSize)
	if batchEnd > totalEntries {
		batchEnd = totalEntries
	}

	// Collect KV indices for the current batch
	var kvsInBatch []uint64
	var currentIndex uint64
out:
	for _, shardId := range shards {
		for k := uint64(0); k < kvEntries; k++ {
			kvIdx := shardId*kvEntries + k
			if kvIdx > lastKvIdx {
				break out
			}
			if currentIndex >= batchStart && currentIndex < batchEnd {
				kvsInBatch = append(kvsInBatch, kvIdx)
			}
			currentIndex++
			if currentIndex >= batchEnd {
				break out
			}
		}
	}
	s.lg.Info("Scanner: batch index range", "batchStart", batchStart, "batchEnd", batchEnd, "kvsInBatch", shortPrt(kvsInBatch))
	return kvsInBatch, totalEntries, batchEnd
}

func (s *Worker) fixKv(kvIndex uint64, commit common.Hash) error {
	s.lg.Info("Scanner: try to fix blob", "kvIndex", kvIndex, "commit", commit)
	if err := s.sm.TryWriteWithMetaCheck(kvIndex, commit, s.fetchBlob); err != nil {
		return fmt.Errorf("failed to write KV: kvIndex=%d, commit=%x, %w", kvIndex, commit, err)
	}
	s.lg.Info("Scanner: fixed blob successfully!", "kvIndex", kvIndex)
	return nil
}
