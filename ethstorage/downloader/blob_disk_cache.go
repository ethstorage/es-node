// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package downloader

import (
	"bytes"
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
	lookup map[common.Hash]uint64 // Lookup table mapping hashes to blob billy entries id
	mu     sync.RWMutex
}

func NewBlobDiskCache() *BlobDiskCache {
	return &BlobDiskCache{
		lookup: make(map[common.Hash]uint64),
	}
}

func (c *BlobDiskCache) Init(datadir string) error {
	cbdir := filepath.Join(datadir, blobCacheDir)
	if err := os.MkdirAll(cbdir, 0700); err != nil {
		return err
	}
	store, err := billy.Open(billy.Options{Path: cbdir, Repair: true}, newSlotter(), nil)
	if err != nil {
		return err
	}
	c.store = store
	return nil
}

func (c *BlobDiskCache) SetBlockBlobs(block *blockBlobs) error {
	rlpBlock, err := rlp.EncodeToBytes(block)
	if err != nil {
		log.Error("Failed to encode transaction for storage", "hash", block.hash, "err", err)
		return err
	}
	id, err := c.store.Put(rlpBlock)
	if err != nil {
		log.Error("Failed to write blob into storage", "hash", block.hash, "err", err)
		return err
	}

	c.mu.Lock()
	c.lookup[block.hash] = id
	c.mu.Unlock()

	log.Info("Set blockBlobs to cache", "id", id, "block", block.number)
	return nil
}

func (c *BlobDiskCache) Blobs(hash common.Hash) []blob {
	c.mu.RLock()
	id, ok := c.lookup[hash]
	c.mu.RUnlock()
	if !ok {
		return nil
	}
	log.Info("Blobs from cache", "hash", hash, "id", id)
	block, err := c.getBlockBlobsById(id)
	if err != nil {
		return nil
	}

	res := []blob{}
	for _, blob := range block.blobs {
		res = append(res, *blob)
	}
	return res
}

func (c *BlobDiskCache) GetKeyValueByIndex(idx uint64, hash common.Hash) []byte {
	blob := c.getBlobByIndex(idx)
	if blob != nil ||
		bytes.Equal(blob.hash[0:ethstorage.HashSizeInContract], hash[0:ethstorage.HashSizeInContract]) {
		return blob.data
	}
	return nil
}

func (c *BlobDiskCache) GetKeyValueByIndexUnchecked(idx uint64) []byte {
	blob := c.getBlobByIndex(idx)
	if blob != nil {
		return blob.data
	}
	return nil
}

func (c *BlobDiskCache) getBlobByIndex(idx uint64) *blob {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, id := range c.lookup {
		block, err := c.getBlockBlobsById(id)
		if err != nil {
			return nil
		}
		for _, blob := range block.blobs {
			if blob.kvIndex.Uint64() == idx {
				return blob
			}
		}
	}
	return nil
}

// TODO: @Qiang An edge case that may need to be handled when Ethereum block is NOT finalized for a long time
// We may need to add a counter in SetBlockBlobs(), if the counter is greater than a threshold which means
// there has been a long time after last Cleanup, so we need to Cleanup anyway in SetBlockBlobs.
func (c *BlobDiskCache) Cleanup(finalized uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for hash, id := range c.lookup {
		block, err := c.getBlockBlobsById(id)
		if err != nil {
			log.Warn("Failed to get block from id", "id", id, "err", err)
			continue
		}
		if block.number <= finalized {
			if err := c.store.Delete(id); err != nil {
				log.Error("Failed to delete block from id", "id", id, "err", err)
			}
			delete(c.lookup, hash)
			log.Info("Cleanup deleted", "finalized", finalized, "block", block.number, "id", id)
		}
	}
}

func (c *BlobDiskCache) getBlockBlobsById(id uint64) (*blockBlobs, error) {
	data, err := c.store.Get(id)
	if err != nil {
		log.Error("Failed to get block from id", "id", id, "err", err)
		return nil, err
	}
	item := new(blockBlobs)
	if err := rlp.DecodeBytes(data, item); err != nil {
		log.Error("Failed to decode block", "id", id, "err", err)
		return nil, err
	}
	log.Debug("Get blockBlobs by id", "id", id, "blockBlobs", item)
	return item, nil
}

func (c *BlobDiskCache) Close() error {
	return c.store.Close()
}

// newSlotter creates a helper method for the Billy datastore that returns the
// individual shelf sizes used to store blobs in.
func newSlotter() func() (uint32, bool) {
	var slotsize uint32

	return func() (size uint32, done bool) {
		slotsize += blobSize
		finished := slotsize >= maxBlobsPerTransaction*blobSize
		log.Debug("new slotter", "slotSize", slotsize, "finished", finished)
		return slotsize, finished
	}
}
