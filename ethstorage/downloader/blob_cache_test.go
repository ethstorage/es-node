// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package downloader

import (
	"bytes"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/protolambda/go-kzg/eth"
)

var (
	cache     BlobCache
	kvHashes  []common.Hash
	datadir   string
	fileName         = "test_shard_0.dat"
	blobData         = "blob data of kvIndex %d"
	minerAddr        = common.BigToAddress(common.Big1)
	kvSize    uint64 = 1 << 17
	kvEntries uint64 = 16
	shardID          = uint64(0)
)

func TestNewSlotter(t *testing.T) {
	slotter := newSlotter()
	var lastSize uint32
	for i := 0; i < 10; i++ {
		size, done := slotter()
		// shelf0 is for block with 1 blob
		if !(size > uint32((i+1)*blobSize) && size < uint32((i+2)*blobSize)) {
			t.Errorf("Slotter returned incorrect size at shelf %d", i)
		}
		if done {
			lastSize = size
			break
		}
	}
	if lastSize/blobSize != maxBlobsPerTransaction {
		t.Errorf("Slotter did not return correct last size")
	}
}

func TestDiskBlobCache(t *testing.T) {
	setup(t)
	t.Cleanup(func() {
		teardown(t)
	})

	block, err := newBlockBlobs(10, 4)
	if err != nil {
		t.Fatalf("Failed to create new block blobs: %v", err)
	}

	err = cache.SetBlockBlobs(block)
	if err != nil {
		t.Fatalf("Failed to set block blobs: %v", err)
	}

	blobs := cache.Blobs(block.hash)
	if len(blobs) != len(block.blobs) {
		t.Fatalf("Unexpected number of blobs: got %d, want %d", len(blobs), len(block.blobs))
	}

	for i, blob := range block.blobs {
		blobData := cache.GetKeyValueByIndex(uint64(i), blob.hash)
		if !bytes.Equal(blobData, blob.data) {
			t.Fatalf("Unexpected blob data at index %d: got %x, want %x", i, blobData, blob.data)
		}
	}

	cache.Cleanup(5)
	blobsAfterCleanup := cache.Blobs(block.hash)
	if len(blobsAfterCleanup) != len(block.blobs) {
		t.Fatalf("Unexpected number of blobs after cleanup: got %d, want %d", len(blobsAfterCleanup), len(block.blobs))
	}

	block, err = newBlockBlobs(20, 6)
	if err != nil {
		t.Fatalf("Failed to create new block blobs: %v", err)
	}

	err = cache.SetBlockBlobs(block)
	if err != nil {
		t.Fatalf("Failed to set block blobs: %v", err)
	}

	cache.Cleanup(15)
	blobsAfterCleanup = cache.Blobs(block.hash)
	if len(blobsAfterCleanup) != len(block.blobs) {
		t.Fatalf("Unexpected number of blobs after cleanup: got %d, want %d", len(blobsAfterCleanup), len(block.blobs))
	}
}

func TestEncoding(t *testing.T) {
	setup(t)
	t.Cleanup(func() {
		teardown(t)
	})

	blockBlobsParams := []struct {
		blockNum uint64
		blobLen  uint64
	}{
		{0, 1},
		{1000, 4},
		{1, 5},
		{222, 6},
		{12345, 2},
		{2000000, 3},
	}

	df, err := ethstorage.Create(fileName, shardID, kvEntries, 0, kvSize, ethstorage.ENCODE_BLOB_POSEIDON, minerAddr, kvSize)
	if err != nil {
		t.Fatalf("Create failed %v", err)
	}
	shardMgr := ethstorage.NewShardManager(common.Address{}, kvSize, kvEntries, kvSize)
	shardMgr.AddDataShard(shardID)
	shardMgr.AddDataFile(df)
	sm := ethstorage.NewStorageManager(shardMgr, nil)
	defer func() {
		sm.Close()
		os.Remove(fileName)
	}()

	// download and save to cache
	for _, tt := range blockBlobsParams {
		bb, err := newBlockBlobs(tt.blockNum, tt.blobLen)
		if err != nil {
			t.Fatalf("failed to create block blobs: %v", err)
		}
		for i, b := range bb.blobs {
			bb.blobs[i].data = sm.EncodeBlob(b.data, b.hash, b.kvIndex.Uint64(), kvSize)
		}
		if err := cache.SetBlockBlobs(bb); err != nil {
			t.Fatalf("failed to set block blobs: %v", err)
		}
	}

	// load from cache and verify
	for i, kvHash := range kvHashes {
		kvIndex := uint64(i)
		t.Run(fmt.Sprintf("test kv: %d", i), func(t *testing.T) {
			blobEncoded := cache.GetKeyValueByIndexUnchecked(kvIndex)
			blobDecoded := sm.DecodeBlob(blobEncoded, kvHash, kvIndex, kvSize)
			bytesWant := []byte(fmt.Sprintf(blobData, kvIndex))
			if !bytes.Equal(blobDecoded[:len(bytesWant)], bytesWant) {
				t.Errorf("GetKeyValueByIndex and decoded = %s, want %s", blobDecoded[:len(bytesWant)], bytesWant)
			}
		})
	}
}

func newBlockBlobs(blockNumber, blobLen uint64) (*blockBlobs, error) {
	block := &blockBlobs{
		number: blockNumber,
		hash:   common.BigToHash(new(big.Int).SetUint64(blockNumber)),
		blobs:  make([]*blob, blobLen),
	}
	for i := uint64(0); i < blobLen; i++ {
		kvIndex := len(kvHashes)
		kvIdx := big.NewInt(int64(kvIndex))
		blob := &blob{
			kvIndex: kvIdx,
			data:    []byte(fmt.Sprintf(blobData, kvIndex)),
		}
		kzgBlob := kzg4844.Blob{}
		copy(kzgBlob[:], blob.data)
		commitment, err := kzg4844.BlobToCommitment(kzgBlob)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create commitment for blob %d: %w", kvIndex, err)
		}
		blob.hash = common.Hash(eth.KZGToVersionedHash(eth.KZGCommitment(commitment)))
		block.blobs[i] = blob
		kvHashes = append(kvHashes, blob.hash)
	}
	return block, nil
}

func setup(t *testing.T) {
	// cache = NewBlobMemCache()
	tmpDir := t.TempDir()
	datadir = filepath.Join(tmpDir, "datadir")
	err := os.MkdirAll(datadir, 0700)
	if err != nil {
		t.Fatalf("Failed to create datadir: %v", err)
	}
	t.Logf("datadir %s", datadir)
	cache = NewBlobDiskCache(datadir, log.NewLogger(log.CLIConfig{
		Level:  "warn",
		Format: "text",
	}))
}

func teardown(t *testing.T) {
	err := cache.Close()
	if err != nil {
		t.Errorf("Failed to close BlobCache: %v", err)
	}
	err = os.RemoveAll(datadir)
	if err != nil {
		t.Errorf("Failed to remove datadir: %v", err)
	}
	kvHashes = nil
}
