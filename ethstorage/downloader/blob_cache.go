// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package downloader

import (
	"bytes"
	"sync"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethstorage/go-ethstorage/ethstorage"
)

type BlobMemCache struct {
	blocks map[common.Hash]*blockBlobs
	mu     sync.RWMutex
}

func NewBlobMemCache() *BlobMemCache {
	return &BlobMemCache{
		blocks: map[common.Hash]*blockBlobs{},
	}
}

func (c *BlobMemCache) SetBlockBlobs(block *blockBlobs) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blocks[block.hash] = block
}

func (c *BlobMemCache) Blobs(hash common.Hash) []blob {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if _, exist := c.blocks[hash]; !exist {
		return nil
	}

	res := []blob{}
	for _, blob := range c.blocks[hash].blobs {
		res = append(res, *blob)
	}
	return res
}

func (c *BlobMemCache) GetKeyValueByIndex(idx uint64, hash common.Hash) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, block := range c.blocks {
		for _, blob := range block.blobs {
			if blob.kvIndex.Uint64() == idx && bytes.Equal(blob.hash[0:ethstorage.HashSizeInContract], hash[0:ethstorage.HashSizeInContract]) {
				return blob.data
			}
		}
	}
	return nil
}

// TODO: @Qiang An edge case that may need to be handled when Ethereum block is NOT finalized for a long time
// We may need to add a counter in SetBlockBlobs(), if the counter is greater than a threshold which means
// there has been a long time after last Cleanup, so we need to Cleanup anyway in SetBlockBlobs.
func (c *BlobMemCache) Cleanup(finalized uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for hash, block := range c.blocks {
		if block.number <= finalized {
			delete(c.blocks, hash)
		}
	}
}
