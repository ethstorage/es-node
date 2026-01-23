// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
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

type IL1 interface {
	GetKvMetas(kvIndices []uint64, blockNumber int64) ([][32]byte, error)
	GetUpdatedKvIndices(startBlock, endBlock *big.Int) ([]uint64, error)
	HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
}

type Worker struct {
	sm        IStorageManager
	fetchBlob es.FetchBlobFunc
	l1        IL1
	lg        log.Logger
}

func NewWorker(
	sm IStorageManager,
	fetch es.FetchBlobFunc,
	l1 IL1,
	batchSize uint64,
	lg log.Logger,
) *Worker {
	return &Worker{
		sm:        sm,
		fetchBlob: fetch,
		l1:        l1,
		lg:        lg,
	}
}

func (s *Worker) scanBatch(ctx context.Context, runtime *scanLoopRuntime, onUpdate scanUpdateFn) error {
	start := time.Now()
	var kvsInBatch []uint64
	defer func(stt time.Time) {
		if len(kvsInBatch) > 0 {
			s.lg.Info("Scan batch done",
				"mode", runtime.mode,
				"scanned", shortPrt(kvsInBatch),
				"count", len(kvsInBatch),
				"nextIndexOfKvIdx", runtime.nextIndex,
				"duration", time.Since(stt).String(),
			)
		}
	}(start)

	// Determine the batch of KV indices to scan
	kvsInBatch, batchEndExclusive := runtime.nextBatch(runtime.batchSize, runtime.nextIndex)
	if len(kvsInBatch) == 0 {
		s.lg.Info("No KV entries to scan in this batch", "mode", runtime.mode)
		return nil
	}
	s.lg.Info("Scan batch started", "mode", runtime.mode, "startIndexOfKvIdx", runtime.nextIndex, "kvsInBatch", shortPrt(kvsInBatch))

	// Query the metas from the L1 contract
	metas, err := s.l1.GetKvMetas(kvsInBatch, rpc.FinalizedBlockNumber.Int64())
	if err != nil {
		s.lg.Error("Failed to query KV metas for scan batch", "error", err)
		return fmt.Errorf("failed to query KV metas: %w", err)
	}
	s.lg.Debug("Query KV meta done", "kvsInBatch", shortPrt(kvsInBatch))

	for i, meta := range metas {
		select {
		case <-ctx.Done():
			s.lg.Warn("Scanner canceled, stopping scan", "ctx.Err", ctx.Err())
			return ctx.Err()
		default:
		}

		mode := runtime.mode
		if mode == modeCheckBlock {
			// since we done parsing blob info from block
			mode = modeCheckBlob
		}
		var commit common.Hash
		copy(commit[:], meta[32-es.HashSizeInContract:32])
		s.scanKv(mode, kvsInBatch[i], commit, onUpdate)
	}

	runtime.nextIndex = batchEndExclusive
	return nil
}

func (s *Worker) getKvsInBatch(batchSize uint64, startIndexOfKvIdx uint64) ([]uint64, uint64) {
	localKvCount, _ := s.summaryLocalKvs()
	if localKvCount == 0 {
		return []uint64{}, 0
	}
	shards := s.sm.Shards()
	kvEntries := s.sm.KvEntries()
	return getKvsInBatch(shards, kvEntries, localKvCount, batchSize, startIndexOfKvIdx, s.lg)
}

func (s *Worker) latestUpdated(blocksToScan uint64, lastScannedBlock uint64) ([]uint64, uint64) {
	latestFinalized, err := s.l1.HeaderByNumber(context.Background(), big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil {
		s.lg.Error("Failed to get latest finalized block header", "error", err)
		return []uint64{}, lastScannedBlock
	}
	startBlock := lastScannedBlock + 1
	endBlock := latestFinalized.Number.Uint64()
	if lastScannedBlock == 0 {
		s.lg.Info(fmt.Sprintf("No last scanned block recorded, starting from %d slots ago", blocksToScan))
		startBlock = endBlock - blocksToScan
	}
	if startBlock > endBlock {
		s.lg.Info("No new finalized blocks to scan", "lastScannedBlock", lastScannedBlock, "latestFinalized", endBlock)
		return []uint64{}, lastScannedBlock
	}
	kvsIndices, err := s.l1.GetUpdatedKvIndices(big.NewInt(int64(startBlock)), big.NewInt(int64(endBlock)))
	if err != nil {
		s.lg.Error("Failed to get updated KV indices", "startBlock", startBlock, "endBlock", endBlock, "error", err)
		return []uint64{}, lastScannedBlock
	}
	// filter out kv indices that are not stored in local storage
	shardSet := make(map[uint64]struct{})
	for _, shard := range s.sm.Shards() {
		shardSet[shard] = struct{}{}
	}
	kvEntries := s.sm.KvEntries()
	var locallyStored []uint64
	for _, kvi := range kvsIndices {
		shardIdx := kvi / kvEntries
		if _, ok := shardSet[shardIdx]; ok {
			locallyStored = append(locallyStored, kvi)
		}
	}
	s.lg.Info("Latest updated KV indices fetched", "startBlock", startBlock, "endBlock", endBlock, "totalUpdatedKvs", len(kvsIndices), "locallyStored", len(locallyStored))
	return locallyStored, endBlock
}

func (s *Worker) scanKv(mode scanMode, kvIndex uint64, commit common.Hash, onUpdate scanUpdateFn) {
	var err error
	switch mode {
	case modeCheckMeta:
		// Check meta only
		metaLocal, found, readErr := s.sm.TryReadMeta(kvIndex)
		if metaLocal != nil {
			err = es.CompareCommits(commit.Bytes(), metaLocal)
		} else {
			if readErr != nil {
				err = fmt.Errorf("failed to read meta: %w", readErr)
			} else if !found {
				err = fmt.Errorf("meta not found locally: %x", commit)
			}
		}
	case modeCheckBlob:
		// Query blob and check meta from storage
		_, found, readErr := s.sm.TryRead(kvIndex, int(s.sm.MaxKvSize()), commit)
		if readErr != nil {
			// Could be CommitMismatchError
			err = readErr
		} else if !found {
			err = fmt.Errorf("blob not found locally: %x", commit)
		}
	default:
		// Other modes are handled outside
		s.lg.Crit("Invalid scanner mode", "mode", mode)
	}
	if err != nil {
		marker := newScanMarker(kvIndex, onUpdate)
		var commitErr *es.CommitMismatchError
		if errors.As(err, &commitErr) {
			s.lg.Warn("Commit mismatch detected", "kvIndex", kvIndex, "error", err)
			marker.markMismatched()
			return
		}
		s.lg.Error("Failed to scan KV", "mode", mode, "kvIndex", kvIndex, "error", err)
		marker.markError(commit, err)
		return
	}

	// Happy path
	s.lg.Debug("KV check completed successfully", "kvIndex", kvIndex, "commit", commit)
}

func (s *Worker) fixBatch(ctx context.Context, kvIndices []uint64, onUpdate scanUpdateFn) error {
	metas, err := s.l1.GetKvMetas(kvIndices, rpc.FinalizedBlockNumber.Int64())
	if err != nil {
		s.lg.Error("Failed to query KV metas for scan batch", "error", err)
		return fmt.Errorf("failed to query KV metas: %w", err)
	}
	s.lg.Debug("Query KV meta done", "kvsInBatch", shortPrt(kvIndices))

	for i, meta := range metas {
		var commit common.Hash
		copy(commit[:], meta[32-es.HashSizeInContract:32])
		s.fixKv(kvIndices[i], commit, onUpdate)
	}
	return nil
}

func (s *Worker) fixKv(kvIndex uint64, commit common.Hash, onUpdate scanUpdateFn) {
	marker := newScanMarker(kvIndex, onUpdate)
	// check blob again before fix
	_, found, err := s.sm.TryRead(kvIndex, int(s.sm.MaxKvSize()), commit)
	if !found && err == nil {
		err = fmt.Errorf("blob not found locally: %x", commit)
	}
	if err != nil {
		var commitErr *es.CommitMismatchError
		if errors.As(err, &commitErr) {
			s.lg.Info("Fixing mismatched KV", "kvIndex", kvIndex)
			if err := s.sm.TryWriteWithMetaCheck(kvIndex, commit, s.fetchBlob); err != nil {
				fixErr := fmt.Errorf("failed to fix KV: kvIndex=%d, commit=%x, %w", kvIndex, commit, err)
				marker.markFailed(commit, fixErr)
				s.lg.Error("Failed to fix KV", "error", fixErr)
				return
			}
			marker.markFixed()
			s.lg.Info("KV fixed successfully", "kvIndex", kvIndex)
			return
		}
		s.lg.Error("Failed to scan KV to fix", "kvIndex", kvIndex, "error", err)
		marker.markError(commit, err)
		return
	}
	marker.markRecovered()
	s.lg.Info("KV recovered", "kvIndex", kvIndex, "commit", commit)
}

func (s *Worker) summaryLocalKvs() (uint64, string) {
	kvEntryCountOnChain := s.sm.KvEntryCount()
	if kvEntryCountOnChain == 0 {
		s.lg.Info("No KV entries found in local storage")
		return 0, "[]"
	}
	return summaryLocalKvs(s.sm.Shards(), s.sm.KvEntries(), kvEntryCountOnChain-1)
}

func getKvsInBatch(shards []uint64, kvEntries, localKvCount, batchSize, startKvIndex uint64, lg log.Logger) ([]uint64, uint64) {
	// Determine batch start and end KV indices
	if startKvIndex >= localKvCount {
		startKvIndex = 0
		lg.Debug("Restarting scan from the beginning")
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
	lg.Debug("Scan batch index range determined", "batchStart", startKvIndex, "batchEnd(exclusive)", endKvIndexExclusive, "kvsInBatch", shortPrt(kvsInBatch))
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
