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
	storageReader   StorageReader
	loadKvFromCache func(uint64, common.Hash) []byte
	l1              es.Il1Source
	nextKvIndex     uint64
	lg              log.Logger
}

func NewWorker(sr StorageReader, f func(uint64, common.Hash) []byte, l1 es.Il1Source, lg log.Logger) *Worker {
	return &Worker{
		storageReader:   sr,
		loadKvFromCache: f,
		l1:              l1,
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
		return fmt.Errorf("failed to query kv metas %w", err)
	}
	s.lg.Info("Scanner: query KV meta done", "kvsInBatch", shortPrt(kvsInBatch), "metaLen", len(metas))
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

		// query blob and check commit from downloader cache
		if blob := s.loadKvFromCache(kvIndex, commit); blob != nil {
			s.lg.Debug("Scanner: loaded blob from downloader cache", "kvIndex", kvIndex)
			continue
		}
		// query blob and check commit from storage
		_, found, err := s.storageReader.TryRead(kvIndex, int(s.storageReader.MaxKvSize()), commit)
		if !found {
			s.lg.Warn("Scanner: blob not found", "kvIndex", kvIndex, "blobHash", commit)
		} else if err != nil {
			s.lg.Error("Scanner: read blob error", "kvIndex", kvIndex, "blobHash", fmt.Sprintf("0x%x", commit), "err", err)
			// TODO fix invalid kv
		} else {
			s.lg.Debug("Scanner: check KV done", "kvIndex", kvIndex, "blobHash", commit)
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
	s.lg.Info("Scanner: query local kv entries done", "localKVs", shortPrt(localKvs))
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
	kvEntryCount, err := s.l1.GetStorageLastBlobIdx(rpc.LatestBlockNumber.Int64())
	if err != nil {
		return nil, fmt.Errorf("failed to query total kv entries: %w", err)
	}
	s.lg.Info("Scanner: query kv entry count done", "kvEntryCount", kvEntryCount)
	var localKvs []uint64
	kvEntries := s.storageReader.KvEntries()
	localShards := s.storageReader.Shards()
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

func shortPrt(arr []uint64) string {
	if len(arr) <= 6 {
		return fmt.Sprintf("%v", arr)
	}
	return fmt.Sprintf("[%d %d %d... %d %d %d]", arr[0], arr[1], arr[2], arr[len(arr)-3], arr[len(arr)-2], arr[len(arr)-1])
}
