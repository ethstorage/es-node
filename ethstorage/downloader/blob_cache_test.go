package downloader

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestBlobCache(t *testing.T) {
	tmpDir := t.TempDir()
	datadir := filepath.Join(tmpDir, "datadir")
	err := os.MkdirAll(datadir, 0700)
	if err != nil {
		t.Fatalf("Failed to create datadir: %v", err)
	}
	t.Logf("datadir %s", datadir)
	cache := NewBlobCache()

	err = cache.Init(datadir)
	if err != nil {
		t.Fatalf("Failed to initialize BlobCache: %v", err)
	}

	var blobLen uint64 = 4
	block := newBlockBlobs(0, blobLen)
	block.number = 10

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
		if !reflect.DeepEqual(blobData, blob.data) {
			t.Fatalf("Unexpected blob data at index %d: got %+v, want %+v", i, blobData, blob.data)
		}
	}

	cache.Cleanup(5)
	blobsAfterCleanup := cache.Blobs(block.hash)
	if len(blobsAfterCleanup) != len(block.blobs) {
		t.Fatalf("Unexpected number of blobs after cleanup: got %d, want %d", len(blobsAfterCleanup), len(block.blobs))
	}

	err = cache.Close()
	if err != nil {
		t.Fatalf("Failed to close BlobCache: %v", err)
	}
}

func newBlockBlobs(blockIdx, blobLen uint64) *blockBlobs {
	block := &blockBlobs{
		hash:  common.BigToHash(new(big.Int).SetUint64(blockIdx)),
		blobs: make([]*blob, blobLen),
	}
	for i := uint64(0); i < blobLen; i++ {
		kvIdx := new(big.Int).SetUint64(blockIdx*blobLen + i)
		blob := &blob{
			kvIndex: kvIdx,
			hash:    common.BigToHash(kvIdx),
			data:    []byte(fmt.Sprintf("blob data %d", i)),
		}
		block.blobs[i] = blob
	}
	return block
}

func TestNewSlotter(t *testing.T) {
	slotter := newSlotter()
	var lastSize uint32
	for i := 0; i < 10; i++ {
		size, done := slotter()
		lastSize = size
		if done {
			break
		}
	}
	expected := uint32(maxBlobsPerTransaction * blobSize)
	if lastSize != expected {
		t.Errorf("Slotter returned incorrect total size: got %d, want %d", lastSize, expected)
	}
}
