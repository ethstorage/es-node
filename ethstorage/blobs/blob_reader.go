// Copyright 2022-2023, es.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package blobs

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
)

type BlobCacheReader interface {
	GetKeyValueByIndex(index uint64, hash common.Hash) []byte
	GetSampleData(kvIndex, sampleIndexInKv uint64) []byte
}

// BlobReader provides unified interface for the miner to read blobs and samples
// from StorageManager and downloader cache.
type BlobReader struct {
	cr BlobCacheReader
	sm *es.StorageManager
	lg log.Logger
}

func NewBlobReader(cr BlobCacheReader, sm *es.StorageManager, lg log.Logger) *BlobReader {
	return &BlobReader{
		cr: cr,
		sm: sm,
		lg: lg,
	}
}

func (n *BlobReader) GetBlob(kvIdx uint64, kvHash common.Hash) ([]byte, error) {
	if blob := n.cr.GetKeyValueByIndex(kvIdx, kvHash); blob != nil {
		n.lg.Debug("Loaded blob from downloader cache", "kvIdx", kvIdx)
		blobDecoded := n.sm.DecodeBlob(blob, kvHash, kvIdx, n.sm.MaxKvSize())
		return blobDecoded, nil
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

func (n *BlobReader) ReadSample(shardIdx, sampleIdx uint64) (common.Hash, error) {
	sampleLenBits := n.sm.MaxKvSizeBits() - es.SampleSizeBits
	kvIdx := sampleIdx >> sampleLenBits
	sampleIdxInKv := sampleIdx % (1 << sampleLenBits)

	if sample := n.cr.GetSampleData(kvIdx, sampleIdxInKv); sample != nil {
		return common.BytesToHash(sample), nil
	}

	sample, err := n.sm.ReadSampleUnlocked(shardIdx, sampleIdx)
	if err != nil {
		return common.Hash{}, err
	}
	return sample, nil
}
