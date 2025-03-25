// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
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

func (s *Worker) ScanBatch() error {
	kvIndexRange, nextKvIndex, err := s.getBatchIndexRange()
	if err != nil {
		return fmt.Errorf("failed to get batch index range: %w", err)
	}
	metas, err := s.contractReader.GetKvMetas(kvIndexRange, rpc.LatestBlockNumber.Int64())
	if err != nil {
		s.lg.Error("Error getting meta range", "kvIndexRange", kvIndexRange)
		return fmt.Errorf("failed to query kv metas %w", err)
	}
	s.lg.Info("Query KV meta done", "kvIndexRange", kvIndexRange, "metaLen", len(metas))
	for k, meta := range metas {
		var commit common.Hash
		copy(commit[:], meta[32-es.HashSizeInContract:32])
		// query blob and check commit
		_, found, err := s.storageReader.TryRead(kvIndexRange[k], int(s.storageReader.MaxKvSize()), commit)
		if err != nil {
			s.lg.Error("Read blob error", "err", err)
			// TODO fix invalid kv
		}
		if !found {
			s.lg.Warn("Blob not found", "kvIndex", kvIndexRange[k], "blobHash", commit)
		}
		s.lg.Info("Check KV done", "kvIndex", kvIndexRange[k], "blobHash", commit)
	}
	s.nextKvIndex = nextKvIndex
	return nil
}

func (s *Worker) getBatchIndexRange() ([]uint64, uint64, error) {
	localKvs, err := s.getLocalKvs()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get local kv indices: %w", err)
	}
	s.lg.Info("Query local kv done", "localKVs", localKvs)
	localKvTotal := uint64(len(localKvs))

	batchStart := s.nextKvIndex
	if batchStart == localKvTotal {
		batchStart = 0
		s.lg.Info("Scan batch start over")
	}
	batchEnd := batchStart + MetaDownloadBatchSize
	if batchEnd > localKvTotal {
		batchEnd = localKvTotal
	}
	return localKvs[batchStart:batchEnd], batchEnd, nil
}

func (s *Worker) getLocalKvs() ([]uint64, error) {
	total, err := s.contractReader.GetStorageLastBlobIdx(rpc.LatestBlockNumber.Int64())
	if err != nil {
		return nil, fmt.Errorf("failed to query total kv entries: %w", err)
	}
	s.lg.Info("Query total kv entries done", "total", total)
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
				localKvs = append(localKvs, shardId*kvEntries+k)
			}
		}
	}
	return localKvs, nil
}
