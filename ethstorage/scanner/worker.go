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
	storageReader  StorageReader
	contractReader es.Il1Source
	nextKvIndex    uint64
	lg             log.Logger
}

func NewWorker(sr StorageReader, cr es.Il1Source, lg log.Logger) *Worker {
	return &Worker{
		storageReader:  sr,
		contractReader: cr,
		lg:             lg,
	}
}

func (s *Worker) ScanBatch(ctx context.Context) error {
	kvsInBatch, nextKvIndex, err := s.determineBatchIndexRange()
	if err != nil {
		return fmt.Errorf("failed to get batch index range: %w", err)
	}
	metas, err := s.contractReader.GetKvMetas(kvsInBatch, rpc.LatestBlockNumber.Int64())
	if err != nil {
		s.lg.Error("Scanner: error getting meta range", "kvsInBatch", shortPrt(kvsInBatch))
		return fmt.Errorf("failed to query kv metas %w", err)
	}
	s.lg.Info("Scanner: query KV meta done", "kvsInBatch", shortPrt(kvsInBatch), "metaLen", len(metas))
	for k, meta := range metas {
		select {
		case <-ctx.Done():
			s.lg.Info("Scanner canceled, stopping scan", "ctx.Err", ctx.Err())
			return ctx.Err()
		default:
		}

		var commit common.Hash
		copy(commit[:], meta[32-es.HashSizeInContract:32])
		// query blob and check commit
		_, found, err := s.storageReader.TryRead(kvsInBatch[k], int(s.storageReader.MaxKvSize()), commit)

		if !found {
			s.lg.Warn("Scanner: blob not found", "kvIndex", kvsInBatch[k], "blobHash", commit)
		} else if err != nil {
			s.lg.Error("Scanner: read blob error", "kvIndex", kvsInBatch[k], "blobHash", fmt.Sprintf("0x%x", commit), "err", err)
			// TODO fix invalid kv
		} else {
			s.lg.Debug("Scanner: check KV done", "kvIndex", kvsInBatch[k], "blobHash", commit)
		}
	}
	s.nextKvIndex = nextKvIndex
	return nil
}

func (s *Worker) determineBatchIndexRange() ([]uint64, uint64, error) {
	localKvs, err := s.queryLocalKvs()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get local kv indices: %w", err)
	}
	s.lg.Info("Scanner: query local kv done", "localKVs", shortPrt(localKvs))
	localKvTotal := uint64(len(localKvs))

	batchStart := s.nextKvIndex
	if batchStart == localKvTotal {
		batchStart = 0
		s.lg.Info("Scanner: scan batch start over")
	}
	batchEnd := batchStart + MetaDownloadBatchSize
	if batchEnd > localKvTotal {
		batchEnd = localKvTotal
	}
	return localKvs[batchStart:batchEnd], batchEnd, nil
}

func (s *Worker) queryLocalKvs() ([]uint64, error) {
	total, err := s.contractReader.GetStorageLastBlobIdx(rpc.LatestBlockNumber.Int64())
	if err != nil {
		return nil, fmt.Errorf("failed to query total kv entries: %w", err)
	}
	s.lg.Info("Scanner: query total kv entries done", "kvEntryCount", total)
	var localKvs []uint64
	kvEntries := s.storageReader.KvEntries()
	localShards := s.storageReader.Shards()
	sort.Slice(localShards, func(i, j int) bool {
		return localShards[i] < localShards[j]
	})
	for _, shardId := range localShards {
		for k := range kvEntries {
			kvIdx := shardId*kvEntries + k
			if kvIdx < total {
				// only check non-empty data
				localKvs = append(localKvs, kvIdx)
			}
		}
	}
	return localKvs, nil
}

func shortPrt(arr []uint64) string {
	if len(arr) <= 6 {
		return fmt.Sprintf("%v", arr)
	}
	return fmt.Sprintf("%v ... %v\n", arr[:3], arr[len(arr)-3:])
}
