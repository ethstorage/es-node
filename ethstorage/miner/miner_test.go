// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package miner

import (
	"math/big"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/event"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
)

const (
	kvSizeBits    = 17
	kvEntriesBits = 1
)

var (
	contractAddr        = common.HexToAddress("0x8FA1872c159DD8681119000d1C7a8Df52a8C128F")
	minerAddr           = common.HexToAddress("0x04580493117292ba13361D8e9e28609ec112264D")
	fileName            = "test_shard_0.dat"
	kvSize       uint64 = 1 << kvSizeBits
	kvEntries    uint64 = 1 << kvEntriesBits
	shardID             = uint64(0)
	value               = hexutil.EncodeUint64(10000000000000)
	lg                  = esLog.NewLogger(esLog.DefaultCLIConfig())
)

func initStorageManager(t *testing.T, client *eth.PollingClient) *es.StorageManager {
	df, err := es.Create(fileName, shardID, kvEntries, 0, kvSize, es.ENCODE_BLOB_POSEIDON, minerAddr, kvSize)
	if err != nil {
		t.Fatalf("Create failed %v", err)
	}
	shardMgr := es.NewShardManager(contractAddr, kvSize, kvEntries, kvSize)
	shardMgr.AddDataShard(shardID)
	shardMgr.AddDataFile(df)
	return es.NewStorageManager(shardMgr, client)
}

func newMiner(t *testing.T, storageMgr *es.StorageManager, client *eth.PollingClient) *Miner {
	defaultConfig := &Config{
		RandomChecks:     2,
		NonceLimit:       1048576,
		MinimumDiff:      new(big.Int).SetUint64(5000000),
		Cutoff:           new(big.Int).SetUint64(60),
		DiffAdjDivisor:   new(big.Int).SetUint64(1024),
		GasPrice:         nil,
		PriorityGasPrice: new(big.Int).SetUint64(10),
		ThreadsPerShard:  1,
		ZKProverMode:     2,
		ZKProverImpl:     1,
		ZKeyFileName:     "blob_poseidon2.zkey",
	}
	l1api := NewL1MiningAPI(client, lg)
	zkWorkingDir, _ := filepath.Abs("../prover")
	pvr := prover.NewKZGPoseidonProver(zkWorkingDir, defaultConfig.ZKeyFileName, defaultConfig.ZKProverMode, defaultConfig.ZKProverImpl, lg)
	fd := new(event.Feed)
	miner := New(defaultConfig, storageMgr, l1api, &pvr, fd, lg)
	return miner
}

func checkMiningState(t *testing.T, m *Miner, mining bool) {
	var state bool
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		if state = m.Mining(); state == mining {
			return
		}
	}
	debug.PrintStack()
	t.Fatalf("Mining() == %t, want %t", state, mining)
}

func TestMiner_update(t *testing.T) {
	var shard = []uint64{0, 1, 2}
	// Case: happy path
	storageMgr := initStorageManager(t, nil)
	miner := newMiner(t, storageMgr, nil)
	miner.Start()
	// waiting for sync done
	checkMiningState(t, miner, false)
	miner.feed.Send(protocol.EthStorageSyncDone{
		DoneType: protocol.SingleShardDone,
		ShardId:  shard[0],
	})
	// worker started
	checkMiningState(t, miner, true)
	if _, ok := miner.worker.shardTaskMap[shard[0]]; !ok {
		t.Error("Shard should be in the shardTaskMap")
	}
	miner.Stop()
	checkMiningState(t, miner, false)

	// Case: syncdone before start
	miner.feed.Send(protocol.EthStorageSyncDone{
		DoneType: protocol.SingleShardDone,
		ShardId:  shard[1],
	})
	// not start
	checkMiningState(t, miner, false)
	// but task is ready to start
	if _, ok := miner.worker.shardTaskMap[shard[1]]; !ok {
		t.Errorf("Shard %d should be in the shardTaskMap", shard[1])
	}
	miner.Start()
	checkMiningState(t, miner, true)

	//  Case: unsubscribe after AllShardDone
	miner.feed.Send(protocol.EthStorageSyncDone{
		DoneType: protocol.AllShardDone,
	})
	checkMiningState(t, miner, true)
	miner.feed.Send(protocol.EthStorageSyncDone{
		DoneType: protocol.SingleShardDone,
		ShardId:  shard[2],
	})
	checkMiningState(t, miner, true)
	// No effect since unsubscribed
	if _, ok := miner.worker.shardTaskMap[shard[2]]; ok {
		t.Errorf("Shard %d should NOT be in the shardTaskMap", shard[1])
	}
	miner.Close()
	checkMiningState(t, miner, false)
	storageMgr.Close()
	os.Remove(fileName)
}
