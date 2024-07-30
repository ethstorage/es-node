// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package downloader

import (
	"bytes"
	"math/big"
	"os"
	"path/filepath"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethstorage/billy"
	"github.com/ethstorage/go-ethstorage/ethstorage"
)

const (
	itemHeaderSize = 4 // size of the per-item header of billy
	sampleSize     = uint64(1 << ethstorage.SampleSizeBits)
	blobSize       = params.BlobTxFieldElementsPerBlob * params.BlobTxBytesPerFieldElement
	blobCacheDir   = "cached_blobs"
)

type blockBlobsCached struct {
	timestamp uint64
	number    uint64
	blobs     []*blobCached
}

type blobCached struct {
	kvIndex *big.Int
	kvSize  *big.Int
	hash    common.Hash
	dataId  uint64
}

type BlobDiskCache struct {
	store  billy.Database
	lookup map[uint64]*blockBlobsCached // Lookup table mapping block number to blockBlob
	index  map[uint64]uint64            // Lookup table mapping kvIndex to blob billy entries id
	mu     sync.RWMutex                 // protects lookup and index maps
	lg     log.Logger
}

func NewBlobDiskCache(datadir string, lg log.Logger) *BlobDiskCache {
	cbdir := filepath.Join(datadir, blobCacheDir)
	if err := os.MkdirAll(cbdir, 0700); err != nil {
		lg.Crit("Failed to create cache directory", "dir", cbdir, "err", err)
	}
	c := &BlobDiskCache{
		lookup: make(map[uint64]*blockBlobsCached),
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
	c.mu.Lock()
	defer c.mu.Unlock()

	var bcs []*blobCached
	for _, b := range block.blobs {
		id, err := c.store.Put(b.data)
		if err != nil {
			c.lg.Error("Failed to write blockBlobs into storage", "block", block.number, "err", err)
			return err
		}
		c.index[b.kvIndex.Uint64()] = id
		bcs = append(bcs, &blobCached{
			kvIndex: b.kvIndex,
			kvSize:  b.kvSize,
			hash:    b.hash,
			dataId:  id,
		})
	}
	c.lookup[block.number] = &blockBlobsCached{
		timestamp: block.timestamp,
		number:    block.number,
		blobs:     bcs,
	}
	c.lg.Info("Set blockBlobs to cache", "block", block.number)
	return nil
}

func (c *BlobDiskCache) Blobs(number uint64) []blob {
	c.mu.RLock()
	bb, ok := c.lookup[number]
	c.mu.RUnlock()
	if !ok {
		return nil
	}
	c.lg.Info("Blobs from cache", "block", bb.number)
	res := []blob{}
	for _, bc := range bb.blobs {
		data, err := c.store.Get(bc.dataId)
		if err != nil {
			c.lg.Error("Failed to get blockBlobs from storage", "block", number, "err", err)
			return nil
		}
		res = append(res, blob{
			kvIndex: bc.kvIndex,
			kvSize:  bc.kvSize,
			hash:    bc.hash,
			data:    data,
		})
	}
	return res
}

func (c *BlobDiskCache) GetKeyValueByIndex(idx uint64, hash common.Hash) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, bb := range c.lookup {
		for _, b := range bb.blobs {
			if b.kvIndex.Uint64() == idx &&
				bytes.Equal(b.hash[0:ethstorage.HashSizeInContract], hash[0:ethstorage.HashSizeInContract]) {
				data, err := c.store.Get(b.dataId)
				if err != nil {
					c.lg.Error("Failed to get kv from downloader cache", "kvIndex", idx, "id", b.dataId, "err", err)
					return nil
				}
				return data
			}
		}
	}
	return nil
}

func (c *BlobDiskCache) GetSampleData(idx, sampleIdx uint64) []byte {
	c.mu.RLock()
	id, ok := c.index[idx]
	c.mu.RUnlock()
	if !ok {
		return nil
	}

	off := sampleIdx << ethstorage.SampleSizeBits
	data, err := c.store.GetSample(id, off, sampleSize)
	if err != nil {
		c.lg.Error("Failed to get sample from downloader cache", "kvIndex", idx, "id", id, "err", err)
		return nil
	}
	return data
}

func (c *BlobDiskCache) Cleanup(finalized uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for number, block := range c.lookup {
		if number <= finalized {
			delete(c.lookup, number)
			for _, blob := range block.blobs {
				if blob.kvIndex != nil {
					delete(c.index, blob.kvIndex.Uint64())
				}
				if err := c.store.Delete(blob.dataId); err != nil {
					c.lg.Error("Failed to delete block from id", "id", blob.dataId, "err", err)
				}
			}
			c.lg.Info("Cleanup deleted", "finalized", finalized, "block", block.number)
		}
	}
}

func (c *BlobDiskCache) Close() error {
	c.lg.Warn("Closing BlobDiskCache")
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range c.index {
		if err := c.store.Delete(id); err != nil {
			c.lg.Warn("Failed to delete blob from id", "id", id, "err", err)
		}
	}
	c.lookup = nil
	c.index = nil
	return c.store.Close()
}

func newSlotter() func() (uint32, bool) {
	return func() (size uint32, done bool) {
		return blobSize + itemHeaderSize, true
	}
}
