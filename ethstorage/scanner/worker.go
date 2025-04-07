// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"context"
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"

	es "github.com/ethstorage/go-ethstorage/ethstorage"
)

type Worker struct {
	sm              *es.StorageManager
	loadKvFromCache func(uint64, common.Hash) []byte
	l1              es.Il1Source
	rpcEndpoint     string
	nextKvIndex     uint64
	lg              log.Logger
}

func NewWorker(sm *es.StorageManager, f func(uint64, common.Hash) []byte, l1 es.Il1Source, rpc string, lg log.Logger) *Worker {
	return &Worker{
		sm:              sm,
		loadKvFromCache: f,
		l1:              l1,
		rpcEndpoint:     rpc,
		lg:              lg,
	}
}

func (s *Worker) ScanBatch(ctx context.Context) error {
	kvsInBatch, nextKvIndex, err := s.determineBatchIndexRange()
	if err != nil {
		return fmt.Errorf("failed to get batch index range: %w", err)
	}
	metas, err := s.l1.GetKvMetas(kvsInBatch, rpc.LatestBlockNumber.Int64())
	if err != nil {
		s.lg.Error("Scanner: error getting meta range", "kvsInBatch", shortPrt(kvsInBatch))
		return fmt.Errorf("failed to query KV metas %w", err)
	}
	s.lg.Info("Scanner: query KV meta done", "kvsInBatch", shortPrt(kvsInBatch))
	for i, meta := range metas {
		select {
		case <-ctx.Done():
			s.lg.Info("Scanner canceled, stopping scan", "ctx.Err", ctx.Err())
			return ctx.Err()
		default:
		}

		kvIndex := kvsInBatch[i]
		var commit common.Hash
		copy(commit[:], meta[32-es.HashSizeInContract:32])

		// query blob and check commit from storage
		_, found, err := s.sm.TryRead(kvIndex, int(s.sm.MaxKvSize()), commit)
		if found && err == nil {
			s.lg.Debug("Scanner: check KV done", "kvIndex", kvIndex, "commit", commit)
			continue
		}
		if !found {
			// the shard does not exist locally
			s.lg.Warn("Scanner: blob not found", "kvIndex", kvIndex, "commit", commit)
			continue
		}
		if err != nil {
			// query blob and check commit from downloader cache
			if blob := s.loadKvFromCache(kvIndex, commit); blob != nil {
				s.lg.Debug("Scanner: check KV done from cache", "kvIndex", kvIndex, "commit", commit)
				continue
			}
			s.lg.Error("Scanner: read blob error", "kvIndex", kvIndex, "commit", commit.Hex(), "err", err)
			if err == es.ErrCommitMismatch {
				if err := s.fixKv(kvIndex, commit); err != nil {
					s.lg.Error("Scanner: fix blob error", "kvIndex", kvIndex, "commit", commit.Hex(), "err", err)
				}
			}
		}
	}
	s.nextKvIndex = nextKvIndex
	if len(kvsInBatch) > 0 {
		s.lg.Info("Scanner: scan batch done", "from", kvsInBatch[0], "to", kvsInBatch[len(kvsInBatch)-1], "count", len(kvsInBatch), "nextKvIndex", nextKvIndex)
	}
	return nil
}

func (s *Worker) determineBatchIndexRange() ([]uint64, uint64, error) {
	localKvs, err := s.queryLocalKvs()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get local KV indices: %w", err)
	}
	s.lg.Info("Scanner: query local KV entries done", "localKVs", shortPrt(localKvs))
	localKvTotal := uint64(len(localKvs))

	batchStart := s.nextKvIndex
	if batchStart == localKvTotal {
		batchStart = 0
		s.lg.Info("Scanner: scan batch start over")
	}
	batchEnd := batchStart + ScanBatchSize
	if batchEnd > localKvTotal {
		batchEnd = localKvTotal
	}
	return localKvs[batchStart:batchEnd], batchEnd, nil
}

func (s *Worker) queryLocalKvs() ([]uint64, error) {
	kvEntryCount, err := s.l1.GetStorageLastBlobIdx(rpc.LatestBlockNumber.Int64())
	if err != nil {
		return nil, fmt.Errorf("failed to query total KV entries: %w", err)
	}
	s.lg.Info("Scanner: query KV entry count done", "kvEntryCount", kvEntryCount)
	var localKvs []uint64
	kvEntries := s.sm.KvEntries()
	localShards := s.sm.Shards()
	sort.Slice(localShards, func(i, j int) bool {
		return localShards[i] < localShards[j]
	})
	for _, shardId := range localShards {
		for k := range kvEntries {
			kvIdx := shardId*kvEntries + k
			if kvIdx < kvEntryCount {
				// only check non-empty data
				localKvs = append(localKvs, kvIdx)
			}
		}
	}
	return localKvs, nil
}

func (s *Worker) fixKv(kvIndex uint64, commit common.Hash) error {
	blob, err := DownloadBlobFromRPC(s.rpcEndpoint, kvIndex, commit)
	if err != nil {
		return fmt.Errorf("failed to download blob from RPC: %w", err)
	}
	s.lg.Info("Download blob from RPC done", "kvIndex", kvIndex, "commit", commit.Hex())
	commit = es.PrepareCommit(commit)
	if err := s.sm.TryWrite(kvIndex, blob, commit); err != nil {
		return fmt.Errorf("failed to write KV: %w", err)
	}
	s.lg.Info("Fix blob done", "kvIndex", kvIndex, "commit", commit.Hex())
	return nil
}
