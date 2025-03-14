// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package ethstorage

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/detailyang/go-fallocate"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	prv "github.com/ethstorage/go-ethstorage/ethstorage/prover"
)

const (
	metafileName      = "metafile.dat.meta"
	defaultEncodeType = ENCODE_BLOB_POSEIDON
	kvEntries         = uint64(16)
	lastKvIndex       = uint64(16)
)

var (
	contractAddress = common.HexToAddress("0x0000000000000000000000000000000003330001")
	testLog         = log.New("TestStorageManager")
	prover          = prv.NewKZGProver(testLog)
	storageManager  *StorageManager
)

type mockL1Source struct {
	lastBlobIndex uint64
	metaFile      *os.File
}

func (l1 *mockL1Source) getMetadata(idx uint64) ([32]byte, error) {
	bs := make([]byte, 32)
	l, err := l1.metaFile.ReadAt(bs, int64(idx*32))
	if err != nil {
		return common.Hash{}, fmt.Errorf("get metadata fail, err %s", err.Error())
	}
	if l != 32 {
		return common.Hash{}, errors.New("get metadata fail, err read less than 32 bytes")
	}
	return common.BytesToHash(bs), nil
}

func (l1 *mockL1Source) GetKvMetas(kvIndices []uint64, blockNumber int64) ([][32]byte, error) {
	metas := make([][32]byte, 0)
	for _, idx := range kvIndices {
		meta, err := l1.getMetadata(idx)
		if err != nil {
			log.Debug("read meta fail", "err", err.Error())
			continue
		}
		metas = append(metas, meta)
	}
	return metas, nil
}

func (l1 *mockL1Source) GetStorageLastBlobIdx(blockNumber int64) (uint64, error) {
	return l1.lastBlobIndex, nil
}

func createMetaFile(filename string, len int64) (*os.File, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}
	err = fallocate.Fallocate(file, int64((32)*len), int64(32))
	if err != nil {
		return nil, err
	}
	return file, nil
}

func newMockL1Source(lastBlobIndex uint64, metafile string) Il1Source {
	if len(metafile) == 0 {
		panic("metafile param is needed when using mock l1")
	}

	file, err := os.OpenFile(metafile, os.O_RDONLY, 0600)
	if err != nil {
		panic(fmt.Sprintf("open metafile faiil with err %s", err.Error()))
	}
	return &mockL1Source{lastBlobIndex: lastBlobIndex, metaFile: file}
}

func createEthStorage(contract common.Address, shardIdxList []uint64, chunkSize, kvSize, kvEntries uint64,
	miner common.Address, encodeType uint64) (*ShardManager, []string) {
	sm := NewShardManager(contract, kvSize, kvEntries, chunkSize)
	ContractToShardManager[contract] = sm
	chunkPerKv := kvSize / chunkSize

	files := make([]string, 0)
	for _, shardIdx := range shardIdxList {
		sm.AddDataShard(shardIdx)
		fileName := fmt.Sprintf(".\\ss%d.dat", shardIdx)
		files = append(files, fileName)
		startChunkId := shardIdx * chunkPerKv * kvEntries
		_, err := Create(fileName, startChunkId, kvEntries*chunkPerKv, 0, kvSize, encodeType, miner, sm.ChunkSize())
		if err != nil {
			log.Crit("open failed", "error", err)
		}

		var df *DataFile
		df, err = OpenDataFile(fileName)
		if err != nil {
			log.Crit("open failed", "error", err)
		}
		sm.AddDataFile(df)
	}

	return sm, files
}

func createBlob(kvIndex uint64) (blob []byte, hash common.Hash) {
	val := make([]byte, 131072)
	copy(val[:20], contractAddress.Bytes())
	binary.BigEndian.PutUint64(val[20:28], kvIndex)
	root := common.Hash{}
	root, _ = prover.GetRoot(val, 1, 131072)
	return val, root
}

func generateMetadata(idx, size uint64, hash []byte) common.Hash {
	meta := make([]byte, 0)
	idx_bs := make([]byte, 8)
	binary.BigEndian.PutUint64(idx_bs, idx)
	meta = append(meta, idx_bs[3:]...)
	size_bs := make([]byte, 8)
	binary.BigEndian.PutUint64(size_bs, size)
	meta = append(meta, size_bs[5:]...)
	meta = append(meta, hash[:24]...)
	return common.BytesToHash(meta)
}

func setup(t *testing.T) {
	// create l1
	metafile, err := createMetaFile(metafileName, int64(kvEntries))
	if err != nil {
		t.Error("Create metafileName fail", err.Error())
	}
	defer func(file *os.File) {
		file.Close()
		os.Remove(file.Name())
	}(metafile)
	l1 := newMockL1Source(lastKvIndex, metafileName)

	// create shard manage
	sm, files := createEthStorage(contractAddress, []uint64{0},
		131072, 131072, kvEntries, common.Address{}, defaultEncodeType)
	if sm == nil {
		t.Fatalf("createEthStorage failed")
	}
	defer func(files []string) {
		for _, file := range files {
			os.Remove(file)
		}
	}(files)

	storageManager = NewStorageManager(sm, l1)
	storageManager.DownloadThreadNum = 1

	kvIndexes := []uint64{1, 2, 3}
	blobs := make([][]byte, len(kvIndexes))
	hashes := make([]common.Hash, len(kvIndexes))
	for i, idx := range kvIndexes {
		blob, hash := createBlob(idx)
		blobs[i] = blob
		hashes[i] = hash
		meta := generateMetadata(uint64(i), kvIndexes[i], hash[:])
		metafile.WriteAt(meta.Bytes(), int64(i*32))
	}
	err = storageManager.DownloadFinished(97528, kvIndexes, blobs, hashes)
	if err != nil {
		t.Fatal("init error")
	}
}

func TestStorageManager_LastKvIndex(t *testing.T) {
	setup(t)
	idx := storageManager.LastKvIndex()
	t.Log("lastKvIndex", idx)
}

func TestStorageManager_DownloadFinished(t *testing.T) {
	setup(t)
	h := common.Hash{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	err := storageManager.DownloadFinished(97529, []uint64{2}, [][]byte{{10}}, []common.Hash{h})

	if err != nil {
		t.Fatal("failed to Download Finished", err)
	}

	bs, success, err := storageManager.TryReadMeta(2)
	if err != nil || !success {
		t.Fatal("failed to read meta", err)
	}

	meta := common.Hash{}
	copy(meta[:], bs)
	if meta != PrepareCommit(h) {
		t.Fatal("failed to write meta", err)
	}
}

func TestStorageManager_CommitBlobs(t *testing.T) {
	setup(t)

	kvIndex := uint64(2)
	b, h := createBlob(kvIndex)
	successCommitted, err := storageManager.CommitBlobs([]uint64{kvIndex}, [][]byte{b}, []common.Hash{h})
	if err != nil {
		t.Fatal("failed to commit blob", err)
	}

	if len(successCommitted) < 1 {
		t.Fatal("should commit all the blobs")
	}

	bs, success, err := storageManager.TryReadMeta(kvIndex)
	if err != nil || !success {
		t.Fatal("failed to read meta", err)
	}

	meta := common.Hash{}
	copy(meta[:], bs)
	if meta != PrepareCommit(h) {
		t.Fatal("failed to write meta", err)
	}
}

func TestStorageManager_DownloadAllMeta(t *testing.T) {
	setup(t)
	err := storageManager.DownloadAllMetas(context.Background(), 4)
	if err != nil {
		t.Fatal("failed to Downloand Finished", err)
	}

	kvIndex := uint64(3)
	_, h := createBlob(kvIndex)
	bs, success, err := storageManager.TryReadMeta(kvIndex)
	if err != nil || !success {
		t.Fatal("failed to read meta", err)
	}

	meta := common.Hash{}
	copy(meta[:], bs)
	if meta != PrepareCommit(h) {
		t.Fatal("failed to compare meta", err)
	}
}
