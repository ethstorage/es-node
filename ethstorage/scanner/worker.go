// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

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
	Shards() []uint64
}

type Worker struct {
	sm              IStorageManager
	loadKvFromCache func(uint64, common.Hash) []byte
	l1              es.Il1Source
	cfg             Config
	nextKvIndex     uint64
	lg              log.Logger
}

func NewWorker(sm IStorageManager, f func(uint64, common.Hash) []byte, l1 es.Il1Source, cfg Config, lg log.Logger) *Worker {
	return &Worker{
		sm:              sm,
		loadKvFromCache: f,
		l1:              l1,
		cfg:             cfg,
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
				if s.cfg.EsRpc == "" {
					s.lg.Warn("Scanner: unable to fix blob: no RPC endpoint provided")
					continue
				}
				fetchBlob := func(kvIndex uint64, commit common.Hash) ([]byte, error) {
					return DownloadBlobFromRPC(s.cfg.EsRpc, kvIndex, commit)
				}
				if err := s.fixKv(kvIndex, commit, fetchBlob); err != nil {
					s.lg.Error("Scanner: fix blob error", "kvIndex", kvIndex, "err", err)
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
	if batchStart >= localKvTotal {
		batchStart = 0
		s.lg.Info("Scanner: scan batch start over")
	}
	batchEnd := batchStart + uint64(s.cfg.BatchSize)
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
	s.lg.Info("Scanner: local shards", "shards", localShards)
	sort.Slice(localShards, func(i, j int) bool {
		return localShards[i] < localShards[j]
	})
	for _, shardId := range localShards {
		for k := uint64(0); k < kvEntries; k++ {
			kvIdx := shardId*kvEntries + k
			if kvIdx < kvEntryCount {
				// only check non-empty data
				localKvs = append(localKvs, kvIdx)
			}
		}
	}
	return localKvs, nil
}

func (s *Worker) fixKv(kvIndex uint64, commit common.Hash, fetchBlob es.FetchBlobFunc) error {
	s.lg.Info("Scanner: try to fix kv", "kvIndex", kvIndex, "commit", commit)
	commitToFetchBlob := commit
	// check if the commit in the contract has been updated
	newCommit, err := s.sm.CheckMeta(kvIndex)
	if err != nil {
		if errors.Is(err, es.ErrCommitMismatch) {
			commitToFetchBlob = newCommit
		}
	} else {
		// the commit in the contract is the same as the one in the storage
		if s.cfg.Mode == modeCheckMeta {
			s.lg.Info("Scanner: KV already fixed by downloader", "kvIndex", kvIndex)
			return nil
		}
		commitToFetchBlob = newCommit
	}
	blob, err := fetchBlob(kvIndex, commitToFetchBlob)
	if err != nil {
		return fmt.Errorf("failed to fetch blob: kvIndex=%d, commit=%x, %w", kvIndex, commitToFetchBlob, err)
	}
	s.lg.Info("Scanner: fetch blob done", "kvIndex", kvIndex, "commit", commitToFetchBlob)
	if err := s.sm.TryWriteWithMetaCheck(kvIndex, commitToFetchBlob, blob, fetchBlob); err != nil {
		return fmt.Errorf("failed to write KV: kvIndex=%d, commit=%x, %w", kvIndex, commitToFetchBlob, err)
	}
	s.lg.Info("Scanner: fixed blob successfully!", "kvIndex", kvIndex, "commit", commitToFetchBlob)
	return nil
}
