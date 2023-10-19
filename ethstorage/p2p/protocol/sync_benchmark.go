// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package protocol

import (
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
)

type Params struct {
	PeerCount   int
	KVSize      uint64
	ChunkSize   uint64
	KVEntries   uint64
	LastKVIndex uint64
	EncodeType  uint64
}

func TestSyncPerfTest(arg Params) {
	var (
		start       = time.Now()
		t           = &testing.T{}
		encodeType  = arg.EncodeType
		testLog     = log.New("TestPerf", "benchmark")
		db          = rawdb.NewMemoryDatabase()
		ctx, cancel = context.WithCancel(context.Background())
		mux         = new(event.Feed)
		shards      = []uint64{0}
		shardMap    = make(map[common.Address][]uint64)
		rollupCfg   = &rollup.EsConfig{
			L2ChainID: new(big.Int).SetUint64(3333),
		}
	)

	metafile, err := CreateMetaFile(metafileName, int64(arg.KVEntries))
	if err != nil {
		t.Error("Create metafileName fail", err.Error())
	}
	defer metafile.Close()

	shardMap[contract] = shards
	shardManager, files := createEthStorage(contract, shards, arg.ChunkSize, arg.KVSize, arg.KVEntries, common.Address{}, encodeType)
	if shardManager == nil {
		t.Fatalf("createEthStorage failed")
	}

	defer func(files []string) {
		for _, file := range files {
			os.Remove(file)
		}
	}(files)

	data := makeKVStorage(contract, shards, arg.ChunkSize, arg.KVSize, arg.KVEntries, arg.LastKVIndex, common.Address{}, encodeType, metafile)

	l1 := NewMockL1Source(arg.LastKVIndex, metafileName)
	sm := ethstorage.NewStorageManager(shardManager, l1)
	testLog.Info("Test prepared", "time", time.Since(start))
	start = time.Now()

	localHost, syncCl := createLocalHostAndSyncClient(t, testLog, rollupCfg, db, sm, nil, mux)
	syncCl.Start()

	for i := 0; i < arg.PeerCount; i++ {
		smr := &mockStorageManagerReader{
			kvEntries:       arg.KVEntries,
			maxKvSize:       arg.KVSize,
			encodeType:      encodeType,
			shards:          shards,
			contractAddress: contract,
			shardMiner:      common.Address{},
			blobPayloads:    data[contract],
		}
		remoteHost := createRemoteHost(t, ctx, rollupCfg, smr, nil, testLog)
		connect(t, localHost, remoteHost, shardMap, shardMap)
	}

	metrics.Enabled = true
	go metrics.CollectProcessMetrics(time.Second)

	checkStall(t, 3600, mux, cancel)

	if !syncCl.syncDone {
		testLog.Error("sync state %v is not match with expected state %v, peer count %d", syncCl.syncDone, false, len(syncCl.peers))
	}
	testLog.Info("Test done", "time", time.Since(start))
	metrics.PrintRuntimeMetrics()
}
