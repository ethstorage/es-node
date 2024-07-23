// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package downloader

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/holiman/billy"
)

const (
	blobSize               = params.BlobTxFieldElementsPerBlob * params.BlobTxBytesPerFieldElement
	maxBlobsPerTransaction = params.MaxBlobGasPerBlock / params.BlobTxBlobGasPerBlob
	blobCacheDir           = "cached_blobs"
)

type BlobDiskCache struct {
	store  billy.Database
	lookup map[uint64]uint64 // Lookup table mapping block number to blob billy entries id
	index  map[uint64]uint64 // Lookup table mapping kvIndex to blob billy entries id
	mu     sync.RWMutex      // protects store, lookup and index maps
	lg     log.Logger
}

func NewBlobDiskCache(datadir string, lg log.Logger) *BlobDiskCache {
	cbdir := filepath.Join(datadir, blobCacheDir)
	if err := os.MkdirAll(cbdir, 0700); err != nil {
		lg.Crit("Failed to create cache directory", "dir", cbdir, "err", err)
	}
	c := &BlobDiskCache{
		lookup: make(map[uint64]uint64),
		index:  make(map[uint64]uint64),
		lg:     lg,
	}

	store, err := billy.Open(billy.Options{Path: cbdir, Repair: true}, newSlotter(), nil)
	if err != nil {
		lg.Crit("Failed to open cache directory", "dir", cbdir, "err", err)
	}
	c.store = store

	lg.Info("BlobDiskCache initialized", "dir", cbdir)
	return c
}

func (c *BlobDiskCache) SetBlockBlobs(block *blockBlobs) error {
	rlpBlock, err := rlp.EncodeToBytes(block)
	if err != nil {
		c.lg.Error("Failed to encode blockBlobs into RLP", "block", block.number, "err", err)
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	id, err := c.store.Put(rlpBlock)
	if err != nil {
		c.lg.Error("Failed to write blockBlobs into storage", "block", block.number, "err", err)
		return err
	}
	c.lookup[block.number] = id
	for _, b := range block.blobs {
		c.index[b.kvIndex.Uint64()] = id
		c.lg.Debug("Indexing blob in cache", "kvIdx", b.kvIndex, "hash", b.hash, "id", id)
	}
	c.lg.Info("Set blockBlobs to cache", "block", block.number, "id", id)
	return nil
}

func (c *BlobDiskCache) Blobs(number uint64) []blob {
	c.mu.RLock()
	id, ok := c.lookup[number]
	if !ok {
		c.mu.RUnlock()
		return nil
	}
	block, err := c.getBlockBlobsById(id)
	c.mu.RUnlock()
	if err != nil || block == nil {
		return nil
	}
	c.lg.Info("Blobs from cache", "block", block.number, "id", id)
	res := []blob{}
	for _, blob := range block.blobs {
		res = append(res, *blob)
	}
	return res
}

func (c *BlobDiskCache) GetKeyValueByIndex(idx uint64, hash common.Hash) []byte {
	blob := c.getBlobByIndex(idx)
	if blob != nil &&
		bytes.Equal(blob.hash[0:ethstorage.HashSizeInContract], hash[0:ethstorage.HashSizeInContract]) {
		return blob.data
	}
	return nil
}

// Access without verification through a hash: only for miner sampling
func (c *BlobDiskCache) GetKeyValueByIndexUnchecked(idx uint64) []byte {
	blob := c.getBlobByIndex(idx)
	if blob != nil {
		return blob.data
	}
	return nil
}

func (c *BlobDiskCache) getBlobByIndex(idx uint64) *blob {
	c.mu.RLock()
	id, ok := c.index[idx]
	if !ok {
		c.mu.RUnlock()
		return nil
	}
	block, err := c.getBlockBlobsById(id)
	c.mu.RUnlock()
	if err != nil || block == nil {
		return nil
	}
	for _, blob := range block.blobs {
		if blob != nil && blob.kvIndex.Uint64() == idx {
			return blob
		}
	}
	return nil
}

func (c *BlobDiskCache) Cleanup(finalized uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for hash, id := range c.lookup {
		block, err := c.getBlockBlobsById(id)
		if err != nil {
			c.lg.Error("Failed to get block from id", "id", id, "err", err)
			continue
		}
		if block != nil && block.number <= finalized {
			if err := c.store.Delete(id); err != nil {
				c.lg.Error("Failed to delete block from id", "id", id, "err", err)
			}
			delete(c.lookup, hash)
			for _, blob := range block.blobs {
				if blob != nil && blob.kvIndex != nil {
					delete(c.index, blob.kvIndex.Uint64())
				}
			}
			c.lg.Info("Cleanup deleted", "finalized", finalized, "block", block.number, "id", id)
		}
	}
}

func (c *BlobDiskCache) getBlockBlobsById(id uint64) (*blockBlobs, error) {
	data, err := c.store.Get(id)
	if err != nil {
		c.lg.Error("Failed to get block from id", "id", id, "err", err)
		return nil, err
	}
	if len(data) == 0 {
		c.lg.Warn("BlockBlobs not found", "id", id)
		return nil, fmt.Errorf("not found: id=%d", id)
	}
	item := new(blockBlobs)
	if err := rlp.DecodeBytes(data, item); err != nil {
		c.lg.Error("Failed to decode block", "id", id, "err", err)
		return nil, err
	}
	return item, nil
}

func (c *BlobDiskCache) Close() error {
	c.lg.Warn("Closing BlobDiskCache")
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range c.lookup {
		if err := c.store.Delete(id); err != nil {
			c.lg.Warn("Failed to delete block from id", "id", id, "err", err)
		}
	}
	c.lookup = nil
	c.index = nil
	return c.store.Close()
}

var base = uint32(44)

// newSlotter creates a helper method for the Billy datastore that returns the
// individual shelf sizes used to store blobs in.

// | blobs | shelf size | data size|
// |--|--|--|
// | 1 | 131160 |131158|
// | 2 | 262276 |262275|
// | 3 | 393392 |393391|
// | 4 | 524508 |524505|
// | 5 | 655624 |655618|
// | 6 | 786740 |786734|

func newSlotter() func() (uint32, bool) {
	var (
		slotsize  uint32 = base
		blobCount uint32 = 1
	)

	return func() (size uint32, done bool) {
		slotsize += blobSize + base
		size = slotsize
		done = blobCount == maxBlobsPerTransaction
		blobCount++
		return
	}
}
