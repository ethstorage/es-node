// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package blobs

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/downloader"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
)

const sampleSizeBits = 5

type BlobQuerier struct {
	dlr   *downloader.Downloader
	sm    *ethstorage.StorageManager
	l1    *eth.PollingClient
	cache sync.Map
	lg    log.Logger
}

func NewBlobQuerier(dlr *downloader.Downloader, sm *ethstorage.StorageManager, l1 *eth.PollingClient, lg log.Logger) *BlobQuerier {
	n := &BlobQuerier{
		dlr: dlr,
		sm:  sm,
		l1:  l1,
		lg:  lg,
	}
	n.init()
	return n
}

func (n *BlobQuerier) init() {
	ch := make(chan common.Hash)
	downloader.SubscribeNewBlobs("blob_querier", ch)
	go func() {
		for {
			blockHash := <-ch
			n.lg.Info("Handling block from downloader cache", "blockHash", blockHash)
			for _, blob := range n.dlr.Cache.Blobs(blockHash) {
				encodedBlob := n.encodeBlob(blob)
				n.cache.Store(blob.KvIdx(), encodedBlob)
			}
		}
	}()
}

func (n *BlobQuerier) encodeBlob(blob downloader.Blob) []byte {
	shardIdx := blob.KvIdx() >> n.sm.KvEntriesBits()
	encodeType, _ := n.sm.GetShardEncodeType(shardIdx)
	miner, _ := n.sm.GetShardMiner(shardIdx)
	encodeKey := ethstorage.CalcEncodeKey(blob.Hash(), blob.KvIdx(), miner)
	encodedBlob := ethstorage.EncodeChunk(blob.Size(), blob.Data(), encodeType, encodeKey)
	n.lg.Info("Encoded blob", "kvIdx", blob.KvIdx(), "miner", miner)
	return encodedBlob
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

	if value, ok := n.cache.Load(kvIdx); ok {
		encodedBlob := value.([]byte)
		sampleIdxInKv := sampleIdx % (1 << sampleLenBits)
		sampleSize := uint64(1 << sampleSizeBits)
		sampleIdxByte := sampleIdxInKv * sampleSize
		sample := encodedBlob[sampleIdxByte : sampleIdxByte+sampleSize]
		return common.BytesToHash(sample), nil
	}

	encodedSample, err := n.sm.ReadSampleUnlocked(shardIdx, sampleIdx)
	if err != nil {
		return common.Hash{}, err
	}
	return encodedSample, nil
}
