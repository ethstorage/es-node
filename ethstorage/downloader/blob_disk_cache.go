// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package downloader

import (
	"bytes"
	"errors"
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

type BlobDiskCache struct {
	store         billy.Database
	storePath     string
	blockLookup   map[uint64]*blockBlobs // Lookup table mapping block number to blockBlob
	kvIndexLookup map[uint64]uint64      // Lookup table mapping kvIndex to blob billy entries id
	mu            sync.RWMutex           // protects lookup and index maps
	lg            log.Logger
}

func NewBlobDiskCache(datadir string, lg log.Logger) *BlobDiskCache {
	cbdir := filepath.Join(datadir, blobCacheDir)
	if err := os.MkdirAll(cbdir, 0700); err != nil {
		lg.Crit("Failed to create cache directory", "dir", cbdir, "err", err)
	}
	c := &BlobDiskCache{
		blockLookup:   make(map[uint64]*blockBlobs),
		kvIndexLookup: make(map[uint64]uint64),
		storePath:     cbdir,
		lg:            lg,
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

	if blockOld, ok := c.blockLookup[block.number]; ok {
		for _, b := range blockOld.blobs {
			if err := c.store.Delete(b.dataId); err != nil {
				c.lg.Warn("Failed to delete blob from cache", "kvIndex", b.kvIndex, "id", b.dataId, "err", err)
			}
		}
	}
	var blbs []*blob
	for _, b := range block.blobs {
		if b.data == nil {
			continue
		}
		kvi := b.kvIndex.Uint64()
		id, err := c.store.Put(b.data)
		if err != nil {
			c.lg.Error("Failed to put blob into cache", "block", block.number, "kvIndex", kvi, "err", err)
			return err
		}
		c.kvIndexLookup[kvi] = id
		blbs = append(blbs, &blob{
			kvIndex: b.kvIndex,
			kvSize:  b.kvSize,
			hash:    b.hash,
			dataId:  id,
		})
	}
	c.blockLookup[block.number] = &blockBlobs{
		timestamp: block.timestamp,
		number:    block.number,
		blobs:     blbs,
	}
	c.lg.Info("Set blockBlobs to cache", "block", block.number)
	return nil
}

func (c *BlobDiskCache) Blobs(number uint64) []blob {
	c.mu.RLock()
	bb, ok := c.blockLookup[number]
	c.mu.RUnlock()
	if !ok {
		return nil
	}
	c.lg.Info("Blobs from cache", "block", bb.number)
	res := []blob{}
	for _, blb := range bb.blobs {
		data, err := c.store.Get(blb.dataId)
		if err != nil {
			c.lg.Error("Failed to get blockBlobs from storage", "block", number, "err", err)
			return nil
		}
		res = append(res, blob{
			kvIndex: blb.kvIndex,
			kvSize:  blb.kvSize,
			hash:    blb.hash,
			data:    data,
		})
	}
	return res
}

func (c *BlobDiskCache) GetKeyValueByIndex(idx uint64, hash common.Hash) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, bb := range c.blockLookup {
		for _, b := range bb.blobs {
			if b.kvIndex.Uint64() == idx &&
				bytes.Equal(b.hash[0:ethstorage.HashSizeInContract], hash[0:ethstorage.HashSizeInContract]) {
				data, err := c.store.Get(b.dataId)
				if err != nil {
					c.lg.Error("Failed to get kv from downloader cache", "kvIndex", idx, "hash", b.hash, "expected", hash)
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
	id, ok := c.kvIndexLookup[idx]
	c.mu.RUnlock()
	if !ok {
		return nil
	}

	off := sampleIdx << ethstorage.SampleSizeBits
	data, err := c.store.GetSample(id, off, sampleSize)
	if err != nil {
		c.lg.Error("Failed to get sample from downloader cache", "kvIndex", idx, "sampleIndex", sampleIdx, "id", id, "err", err)
		return nil
	}
	return data
}

func (c *BlobDiskCache) Cleanup(finalized uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var blocksCleaned, blobsCleaned int
	for number, block := range c.blockLookup {
		if number <= finalized {
			delete(c.blockLookup, number)
			for _, blob := range block.blobs {
				if blob.kvIndex != nil {
					delete(c.kvIndexLookup, blob.kvIndex.Uint64())
				}
				if err := c.store.Delete(blob.dataId); err != nil {
					c.lg.Error("Failed to delete block from id", "id", blob.dataId, "err", err)
				}
				blobsCleaned++
			}
			blocksCleaned++
		}
	}
	c.lg.Debug("Downloader cache cleaned up", "blockFinalized", finalized, "blocksCleaned", blocksCleaned, "blobsCleaned", blobsCleaned)
}

func (c *BlobDiskCache) Close() error {
	var er error
	if err := c.store.Close(); err != nil {
		c.lg.Error("Failed to close cache", "err", err)
		er = err
	}
	if err := os.RemoveAll(c.storePath); err != nil {
		c.lg.Error("Failed to remove cache dir", "err", err)
		if er == nil {
			er = err
		} else {
			er = errors.New(er.Error() + "; " + err.Error())
		}
	}
	if er != nil {
		return er
	}
	c.lg.Info("BlobDiskCache closed.")
	return nil
}

func newSlotter() func() (uint32, bool) {
	return func() (size uint32, done bool) {
		return blobSize + itemHeaderSize, true
	}
}
