// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package node

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/downloader"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
)

const sampleSizeBits = 5

type BlobQuerier struct {
	dlr *downloader.Downloader
	sm  *ethstorage.StorageManager
	l1  *eth.PollingClient
	lg  log.Logger
}

func NewBlobQuerier(dlr *downloader.Downloader, sm *ethstorage.StorageManager, l1 *eth.PollingClient, lg log.Logger) *BlobQuerier {
	return &BlobQuerier{
		dlr: dlr,
		sm:  sm,
		l1:  l1,
		lg:  lg,
	}
}

func (n *BlobQuerier) GetBlob(kvIdx uint64, kvHash common.Hash) ([]byte, error) {
	blob := n.dlr.Cache.GetKeyValueByIndex(kvIdx, kvHash)
	if blob != nil {
		n.lg.Debug("Loaded blob from downloader cache", "kvIdx", kvIdx)
		return blob, nil
	}
	blob, exist, err := n.sm.TryRead(kvIdx, int(n.sm.MaxKvSize()), kvHash)
	if err != nil {
		return nil, err
	}
	if !exist {
		return nil, fmt.Errorf("kv not found: index=%d", kvIdx)
	}
	n.lg.Debug("Loaded blob from storage manager", "kvIdx", kvIdx)
	return blob, nil
}

func (n *BlobQuerier) ReadSample(shardIdx, sampleIdx uint64) (common.Hash, error) {
	sampleLenBits := n.sm.MaxKvSizeBits() - sampleSizeBits
	kvIdx := sampleIdx >> sampleLenBits

	kvData, kvHash := n.dlr.Cache.GetKeyValueByIndexUnsafe(kvIdx)
	if kvData == nil {
		encodedSample, err := n.sm.ReadSampleUnlocked(shardIdx, sampleIdx)
		if err != nil {
			return common.Hash{}, err
		}
		return encodedSample, nil
	}
	miner, ok := n.sm.GetShardMiner(shardIdx)
	if !ok {
		n.lg.Error("Miner not found for shard", "shard", shardIdx)
	}
	encodingKey := ethstorage.CalcEncodeKey(kvHash, kvIdx, miner)
	sampleIdxInKv := sampleIdx % (1 << sampleLenBits)

	start := time.Now()
	mask, err := prover.GenerateMask(encodingKey, sampleIdxInKv)
	fmt.Printf("kvIdx %d  took %s\n", kvIdx, time.Since(start))

	if err != nil {
		n.lg.Error("Generate mask error", "encodingKey", encodingKey, "sampleIdx", sampleIdxInKv,
			"error", err.Error())
		return common.Hash{}, err
	}
	sampleSize := uint64(1 << sampleSizeBits)
	sampleIdxByte := sampleIdxInKv * sampleSize
	sample := kvData[sampleIdxByte : sampleIdxByte+sampleSize]
	encodedBytes := ethstorage.MaskDataInPlace(mask, sample)
	return common.BytesToHash(encodedBytes), nil
}
