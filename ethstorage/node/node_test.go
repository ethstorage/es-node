// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package node

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/db"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
)

func createSstorage(shardIdxList []uint64, cfg storage.StorageConfig) {
	files := make([]string, 0)
	for _, shardIdx := range shardIdxList {
		fileName := fmt.Sprintf(".\\ss%d.dat", shardIdx)
		files = append(files, fileName)
		chunkPerfile := cfg.KvSize / cfg.ChunkSize
		startChunkId := shardIdx * cfg.KvEntriesPerShard * chunkPerfile
		_, err := ethstorage.Create(fileName, startChunkId, chunkPerfile*cfg.KvEntriesPerShard, 0, cfg.KvSize,
			ethstorage.ENCODE_ETHASH, cfg.Miner, cfg.ChunkSize)
		if err != nil {
			log.Crit("Open failed", "error", err)
		}
	}
	cfg.Filenames = files
}

func test_InitDB(test *testing.T, dataDir string) {
	var (
		key = []byte("key")
		bs  = []byte("value to store")
		val []byte
	)
	storConfig := storage.StorageConfig{
		KvSize:            uint64(131072),
		ChunkSize:         uint64(4096),
		KvEntriesPerShard: uint64(1024),
		L1Contract:        common.HexToAddress("0x0000000000000000000000000000000003330001"),
		Miner:             common.HexToAddress("0x0000000000000000000000000000000000000001"),
	}
	createSstorage([]uint64{0}, storConfig)
	cfg := Config{
		DataDir:  dataDir,
		DBConfig: db.DefaultDBConfig(),
		Storage:  storConfig,
	}

	n := &EsNode{
		lg:         log.New("unittest"),
		appVersion: "unittest",
		metrics:    metrics.NoopMetrics,
	}
	err := n.initDatabase(&cfg)
	if err != nil {
		test.Error(err.Error())
	}

	err = n.db.Put(key, bs)
	if err != nil {
		test.Error(err.Error())
	}

	val, err = n.db.Get(key)
	if bytes.Compare(val, bs) != 0 {
		test.Error("val and bs is not match")
	}
}

func Test_InitDB_MemoryDB(test *testing.T) {
	dataDir := ""
	test_InitDB(test, dataDir)
}

func Test_InitDB_LevelDB(test *testing.T) {
	dataDir := ".\\"
	test_InitDB(test, dataDir)
}
