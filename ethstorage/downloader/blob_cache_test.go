// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package downloader

import (
	"bytes"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	sampleLen        = blobSize / sampleSize
	minerAddr        = common.BigToAddress(common.Big1)
	kvSize    uint64 = 1 << 17
	kvEntries uint64 = 16
	shardID          = uint64(0)
)

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

	blobs := cache.Blobs(block.number)
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
	blobsAfterCleanup := cache.Blobs(block.number)
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
	blobsAfterCleanup = cache.Blobs(block.number)
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
			blobEncoded := cache.GetKeyValueByIndex(kvIndex, kvHash)
			blobDecoded := sm.DecodeBlob(blobEncoded, kvHash, kvIndex, kvSize)
			bytesWant := []byte(fmt.Sprintf(blobData, kvIndex))
			if !bytes.Equal(blobDecoded[:len(bytesWant)], bytesWant) {
				t.Errorf("GetKeyValueByIndex and decoded = %s, want %s", blobDecoded[:len(bytesWant)], bytesWant)
			}
		})
	}
}

func TestBlobDiskCache_GetSampleData(t *testing.T) {
	setup(t)
	t.Cleanup(func() {
		teardown(t)
	})

	const blockStart = 10000000
	rand.New(rand.NewSource(time.Now().UnixNano()))
	kvIndex2BlockNumber := map[uint64]uint64{}
	kvIndex2BlobIndex := map[uint64]uint64{}

	newBlockBlobsFilled := func(blockNumber, blobLen uint64) (*blockBlobs, error) {
		block := &blockBlobs{
			number: blockNumber,
			blobs:  make([]*blob, blobLen),
		}
		for i := uint64(0); i < blobLen; i++ {
			kvIndex := uint64(len(kvHashes))
			blob := &blob{
				kvIndex: new(big.Int).SetUint64(kvIndex),
				data:    fill(blockNumber, i),
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
			kvIndex2BlockNumber[kvIndex] = blockNumber
			kvIndex2BlobIndex[kvIndex] = i
		}
		t.Log("Block created", "number", block.number, "blobs", blobLen)
		return block, nil
	}
	for i := 0; i < 10; i++ {
		blockn, blobn := blockStart+i, rand.Intn(6)+1
		block, err := newBlockBlobsFilled(uint64(blockn), uint64(blobn))
		if err != nil {
			t.Fatalf("Failed to create new block blobs: %v", err)
		}
		if err := cache.SetBlockBlobs(block); err != nil {
			t.Fatalf("Failed to set block blobs: %v", err)
		}
	}

	for kvi := range kvHashes {
		kvIndex := uint64(kvi)
		sampleIndex := rand.Intn(int(sampleLen))
		sample := cache.GetSampleData(kvIndex, uint64(sampleIndex))
		sampleWant := make([]byte, sampleSize)
		copy(sampleWant, fmt.Sprintf("%d_%d_%d", kvIndex2BlockNumber[kvIndex], kvIndex2BlobIndex[kvIndex], sampleIndex))
		t.Run(fmt.Sprintf("test sample: kvIndex=%d, sampleIndex=%d", kvIndex, sampleIndex), func(t *testing.T) {
			if !bytes.Equal(sample, sampleWant) {
				t.Errorf("GetSampleData got %x, want %x", sample, sampleWant)
			}
		})
	}

}

func fill(blockNumber, blobIndex uint64) []byte {
	var content []byte
	for i := uint64(0); i < sampleLen; i++ {
		sample := make([]byte, sampleSize)
		copy(sample, fmt.Sprintf("%d_%d_%d", blockNumber, blobIndex, i))
		content = append(content, sample...)
	}
	return content
}

func newBlockBlobs(blockNumber, blobLen uint64) (*blockBlobs, error) {
	block := &blockBlobs{
		number: blockNumber,
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
