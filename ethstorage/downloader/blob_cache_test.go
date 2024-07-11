// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package downloader

import (
	"bytes"
	"fmt"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/protolambda/go-kzg/eth"
)

var (
	bc        BlobCache
	kvHashes  []common.Hash
	fileName         = "test_shard_0.dat"
	blobData         = "blob data of kvIndex %d"
	minerAddr        = common.BigToAddress(common.Big0)
	kvSize    uint64 = 1 << 17
	kvEntries uint64 = 16
	shardID          = uint64(0)
)

func init() {
	bc = NewBlobMemCache()
}

func TestBlobCache_Encoding(t *testing.T) {
	blockBlobsParams := []struct {
		blockNum uint64
		blobLen  uint64
	}{
		{0, 1},
		{1, 5},
		{1000, 4},
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

	for _, tt := range blockBlobsParams {
		// download and save to cache
		bb, err := newBlockBlobs(tt.blockNum, tt.blobLen)
		if err != nil {
			t.Fatalf("failed to create block blobs: %v", err)
		}
		for i, b := range bb.blobs {
			bb.blobs[i].data = sm.EncodeBlob(b.data, b.hash, b.kvIndex.Uint64(), kvSize)
		}
		bc.SetBlockBlobs(bb)
	}

	// load from cache and verify
	for i, kvHash := range kvHashes {
		kvIndex := uint64(i)
		t.Run(fmt.Sprintf("test kv: %d", i), func(t *testing.T) {
			blobEncoded := bc.GetKeyValueByIndexUnchecked(kvIndex)
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
