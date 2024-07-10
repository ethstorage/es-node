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
	"github.com/ethstorage/go-ethstorage/ethstorage"
)

var (
	kvIndex   uint64 = 0
	fileName         = "test_shard_0.dat"
	blobData         = "blob data of kvIndex %d"
	minerAddr        = common.BigToAddress(common.Big0)
	kvSize    uint64 = 1 << 17
	kvEntries uint64 = 16
	shardID          = uint64(0)
	bc        BlobCache
)

func init() {
	bc = NewBlobMemCache()
}

func TestBlobCache_GetKeyValueByIndex(t *testing.T) {
	tests := []struct {
		blockNum  uint64
		blobLen   uint64
		kvIdxWant uint64
	}{
		{0, 1, 0},
		{1000, 5, 5},
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

	for i, tt := range tests {
		bb := newBlockBlobs(tt.blockNum, tt.blobLen)
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			for i, b := range bb.blobs {
				bb.blobs[i].data = sm.EncodeBlob(b.data, b.hash, b.kvIndex.Uint64())
			}
			bc.SetBlockBlobs(bb)
			kvHash, ok, err := sm.TryReadMeta(tt.kvIdxWant)
			if err != nil {
				t.Fatalf("TryReadMeta() error = %v", err)
			}
			if !ok {
				t.Fatalf("TryReadMeta() got = %v, want %v", ok, true)
			}
			blobEncoded := bc.GetKeyValueByIndex(tt.kvIdxWant, common.Hash(kvHash))
			blobDecoded := sm.DecodeBlob(blobEncoded, common.Hash(kvHash), tt.kvIdxWant)
			bytesWant := []byte(fmt.Sprintf(blobData, tt.kvIdxWant))
			if !bytes.Equal(blobDecoded[:len(bytesWant)], bytesWant) {
				t.Errorf("BlobMemCache.GetKeyValueByIndex() and decoded = %s, want %s", blobDecoded[:len(bytesWant)], bytesWant)
			}
		})
	}

}

func newBlockBlobs(blockNumber, blobLen uint64) *blockBlobs {
	block := &blockBlobs{
		number: blockNumber,
		hash:   common.BigToHash(new(big.Int).SetUint64(blockNumber)),
		blobs:  make([]*blob, blobLen),
	}
	for i := uint64(0); i < blobLen; i++ {
		kvIdx := new(big.Int).SetUint64(kvIndex)
		blob := &blob{
			kvIndex: kvIdx,
			hash:    common.BigToHash(kvIdx),
			data:    []byte(fmt.Sprintf(blobData, kvIndex)),
		}
		block.blobs[i] = blob
		kvIndex = kvIndex + 1
	}
	return block
}
