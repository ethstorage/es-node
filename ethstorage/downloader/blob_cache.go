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
	"github.com/holiman/billy"

	"github.com/ethstorage/go-ethstorage/ethstorage"
)

const (
	blobSize               = params.BlobTxFieldElementsPerBlob * params.BlobTxBytesPerFieldElement
	maxBlobsPerTransaction = params.MaxBlobGasPerBlock / params.BlobTxBlobGasPerBlob
	blobCacheDir           = "cached_blobs"
)

type BlobCache struct {
	store  billy.Database
	lookup map[common.Hash]uint64 // Lookup table mapping hashes to blob billy entries
	mu     sync.RWMutex
}

func NewBlobCache() *BlobCache {
	return &BlobCache{
		lookup: make(map[common.Hash]uint64),
	}
}

func (c *BlobCache) Init(datadir string) error {
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

func (c *BlobCache) SetBlockBlobs(block *blockBlobs) error {
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
	return nil
}

func (c *BlobCache) Blobs(hash common.Hash) []blob {
	c.mu.RLock()
	id, ok := c.lookup[hash]
	c.mu.RUnlock()
	if !ok {
		return nil
	}
	block, err := c.fromIdToBlock(id)
	if err != nil {
		return nil
	}

	res := []blob{}
	for _, blob := range block.blobs {
		res = append(res, *blob)
	}
	return res
}

func (c *BlobCache) GetKeyValueByIndex(idx uint64, hash common.Hash) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, id := range c.lookup {
		block, err := c.fromIdToBlock(id)
		if err != nil {
			return nil
		}
		for _, blob := range block.blobs {
			if blob.kvIndex.Uint64() == idx &&
				bytes.Equal(blob.hash[0:ethstorage.HashSizeInContract], hash[0:ethstorage.HashSizeInContract]) {
				return blob.data
			}
		}
	}
	return nil
}

// TODO: @Qiang An edge case that may need to be handled when Ethereum block is NOT finalized for a long time
// We may need to add a counter in SetBlockBlobs(), if the counter is greater than a threshold which means
// there has been a long time after last Cleanup, so we need to Cleanup anyway in SetBlockBlobs.
func (c *BlobCache) Cleanup(finalized uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for hash, id := range c.lookup {
		block, err := c.fromIdToBlock(id)
		if err != nil {
			log.Warn("Failed to get block from id", "id", id, "err", err)
			continue
		}
		if block.number <= finalized {
			if err := c.store.Delete(id); err != nil {
				log.Error("Failed to delete block from id", "id", id, "err", err)
			}
			delete(c.lookup, hash)
		}
	}
}

func (c *BlobCache) fromIdToBlock(id uint64) (*blockBlobs, error) {
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
	return item, nil
}

func (c *BlobCache) Close() error {
	return c.store.Close()
}

// newSlotter creates a helper method for the Billy datastore that returns the
// individual shelf sizes used to store blobs in.
func newSlotter() func() (uint32, bool) {
	var slotsize uint32

	return func() (size uint32, done bool) {
		slotsize += blobSize
		finished := slotsize > maxBlobsPerTransaction*blobSize
		return slotsize, finished
	}
}
