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
	CheckMeta(kvIdx uint64) (common.Hash, error)
	TryWriteWithMetaCheck(kvIdx uint64, commit common.Hash, blob []byte, fetchBlob es.FetchBlobFunc) error
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
	shards := s.sm.Shards()
	kvEntries := s.sm.KvEntries()
	lastKvIdx := s.sm.LastKvIndex() - 1
	s.lg.Info("Scanner: scan batch started", "lastKvIdx", lastKvIdx, "shards", shards, "kvEntriesPerShard", kvEntries)
	var totalEntries uint64
	for _, shardId := range shards {
		if shardId == lastKvIdx/kvEntries {
			totalEntries += (lastKvIdx + 1) % kvEntries
			break
		}
		totalEntries += kvEntries
	}
	s.lg.Info("Scanner: determining batch index range", "localKvs", previewLocalKvs(shards, kvEntries, lastKvIdx), "totalKvStored", totalEntries)

	sts := stats{}
	sts.total = int(totalEntries)

	batchStart := s.nextIndexOfKvIdx
	if batchStart >= totalEntries {
		batchStart = 0
		s.lg.Info("Scanner: scan batch start over")
	}
	batchEnd := batchStart + uint64(s.cfg.BatchSize)
	if batchEnd > totalEntries {
		batchEnd = totalEntries
	}

	var localKvs []uint64
out:
	for _, shardId := range shards {
		for k := uint64(0); k < kvEntries; k++ {
			kvIdx := shardId*kvEntries + k
			if kvIdx > lastKvIdx {
				break out
			}
			localKvs = append(localKvs, kvIdx)
		}
	}
	kvsInBatch := localKvs[batchStart:batchEnd]
	s.lg.Info("Scanner: batch index range", "batchStart", batchStart, "batchEnd", batchEnd, "kvsInBatch", shortPrt(kvsInBatch))

	// Query the metas from the L1 contract
	metas, err := s.l1.GetKvMetas(kvsInBatch, rpc.FinalizedBlockNumber.Int64())
	if err != nil {
		return nil, fmt.Errorf("failed to query KV metas %w", err)
	}
	s.lg.Info("Scanner: query KV meta done", "kvsInBatch", shortPrt(kvsInBatch))
	for i, meta := range metas {
		select {
		case <-ctx.Done():
			s.lg.Info("Scanner canceled, stopping scan", "ctx.Err", ctx.Err())
			return nil, ctx.Err()
		default:
		}

		var err error
		var found bool
		var commit common.Hash
		copy(commit[:], meta[32-es.HashSizeInContract:32])
		kvIndex := kvsInBatch[i]
		if s.cfg.Mode == modeCheckMeta {
			// check meta only
			var metaLocal []byte
			metaLocal, found, err = s.sm.TryReadMeta(kvIndex)
			if metaLocal != nil && !bytes.Equal(metaLocal[0:es.HashSizeInContract], commit[0:es.HashSizeInContract]) {
				err = es.ErrCommitMismatch
			}
		} else if s.cfg.Mode == modeCheckBlob {
			// query blob and check meta from storage
			_, found, err = s.sm.TryRead(kvIndex, int(s.sm.MaxKvSize()), commit)
		} else {
			panic(fmt.Sprintf("invalid scanner mode: %d", s.cfg.Mode))
		}
		if found && err == nil {
			// happy path
			s.lg.Debug("Scanner: check KV done", "kvIndex", kvIndex, "commit", commit)
			continue
		}
		if !found {
			// the shard does not exist locally
			s.lg.Warn("Scanner: blob not found", "kvIndex", kvIndex, "commit", commit)
			continue
		}
		if err != nil {
			// chances are the blob is downloaded but not written to the storage
			if blob := s.loadKvFromCache(kvIndex, commit); blob != nil {
				s.lg.Debug("Scanner: check KV done from cache", "kvIndex", kvIndex, "commit", commit)
				continue
			}
			s.lg.Error("Scanner: read blob error", "kvIndex", kvIndex, "commit", commit.Hex(), "err", err)
			if err == es.ErrCommitMismatch {
				sts.mismatched = append(sts.mismatched, kvIndex)
				if err := s.fixKv(kvIndex, commit); err != nil {
					sts.failed = append(sts.failed, kvIndex)
					s.lg.Error("Scanner: fix blob error", "kvIndex", kvIndex, "err", err)
					sendError(kvIndex, fmt.Errorf("failed to fix blob: %w", err))
				} else {
					sts.fixed = append(sts.fixed, kvIndex)
					// remove the error from the cache
					sendError(kvIndex, nil)
				}
			} else {
				sendError(kvIndex, fmt.Errorf("failed to read blob: %w", err))
			}
		}
	}
	s.nextIndexOfKvIdx = batchEnd
	if len(kvsInBatch) > 0 {
		s.lg.Info("Scanner: scan batch done", "scanned", shortPrt(kvsInBatch), "count", len(kvsInBatch), "nextIndexOfKvIdx", s.nextIndexOfKvIdx)
	}
	return &sts, nil
}

func (s *Worker) fixKv(kvIndex uint64, commit common.Hash) error {
	s.lg.Info("Scanner: try to fix blob", "kvIndex", kvIndex, "commit", commit)
	commitToFetchBlob := commit
	newCommit, err := s.sm.CheckMeta(kvIndex)
	if err != nil {
		if errors.Is(err, es.ErrCommitMismatch) {
			// the commit in the contract has been updated
			commitToFetchBlob = newCommit
		} else {
			// use the old commit if check meta fails
			s.lg.Warn("Scanner: check meta error", "kvIndex", kvIndex, "err", err)
		}
	} else {
		// the commit in the contract is the same as the one in the storage
		if s.cfg.Mode == modeCheckMeta {
			s.lg.Info("Scanner: KV already fixed by downloader", "kvIndex", kvIndex)
			return nil
		}
		// in case the commit in the contract is same with the one in the storage, and both updated lately
		commitToFetchBlob = newCommit
	}
	blob, err := s.fetchBlob(kvIndex, commitToFetchBlob)
	if err != nil {
		return fmt.Errorf("failed to fetch blob: kvIndex=%d, commit=%x, %w", kvIndex, commitToFetchBlob, err)
	}
	s.lg.Info("Scanner: fetch blob done", "kvIndex", kvIndex, "commit", commitToFetchBlob)
	if err := s.sm.TryWriteWithMetaCheck(kvIndex, commitToFetchBlob, blob, s.fetchBlob); err != nil {
		return fmt.Errorf("failed to write KV: kvIndex=%d, commit=%x, %w", kvIndex, commitToFetchBlob, err)
	}
	s.lg.Info("Scanner: fixed blob successfully!", "kvIndex", kvIndex, "commit", commitToFetchBlob)
	return nil
}
