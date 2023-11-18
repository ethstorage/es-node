// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package miner

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
	"golang.org/x/term"
)

const (
	rpcUrl        = "http://65.108.236.27:8545"
	chainID       = "7011893058"
	kvSizeBits    = 17
	sampleLenBits = 12
	kvEntriesBits = 1
	private       = "95eb6ffd2ae0b115db4d1f0d58388216f9d026896696a5211d77b5f14eb5badf"
)

var (
	// Note a new contract need to be deployed each time the test is being executed.
	contractAddr        = common.HexToAddress("0x8FA1872c159DD8681119000d1C7a8Df52a8C128F")
	minerAddr           = common.HexToAddress("0x04580493117292ba13361D8e9e28609ec112264D")
	fileName            = "test_shard_0.dat"
	kvSize       uint64 = 1 << kvSizeBits
	kvEntries    uint64 = 1 << kvEntriesBits
	shardID             = uint64(0)
	value               = hexutil.EncodeUint64(10000000000000)
	lg                  = esLog.NewLogger(esLog.CLIConfig{
		Level:  "debug",
		Format: "text",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	})
)

func TestMine(t *testing.T) {
	client, err := eth.Dial(rpcUrl, contractAddr, lg)
	if err != nil {
		t.Fatalf("failed to get client L1: %v", err)
	}
	storageMgr := initStorageManager(t, client, true)
	defer func() {
		client.Close()
		storageMgr.Close()
		err := os.Remove(fileName)
		if err != nil {
			t.Errorf("cannot remove %s: %v", fileName, err)
		}
	}()
	miner := newMiner(t, storageMgr, client)
	eth.WatchHeadChanges(context.Background(), client, func(ctx context.Context, sig eth.L1BlockRef) {
		select {
		case miner.ChainHeadCh <- sig:
		default:
		}
	})
	miner.Start()
	miner.feed.Send(protocol.EthStorageSyncDone{
		DoneType: protocol.SingleShardDone,
		ShardId:  shardID,
	})
	time.Sleep(300 * time.Second)
	miner.Close()
	checkMiningState(t, miner, false)
}

func initStorageManager(t *testing.T, client *eth.PollingClient, withData bool) *es.StorageManager {
	df, err := es.Create(fileName, shardID, kvEntries, 0, kvSize, es.ENCODE_BLOB_POSEIDON, minerAddr, kvSize)
	if err != nil {
		t.Fatalf("Create failed %v", err)
	}
	shardMgr := es.NewShardManager(contractAddr, kvSize, kvEntries, kvSize)
	shardMgr.AddDataShard(shardID)
	shardMgr.AddDataFile(df)
	if withData {
		dataRaw, err := readFile()
		if err != nil {
			t.Fatalf("Read raw data failed %v", err)
		}
		kvIdxes, dataHashes, err := utils.UploadBlobs(client, rpcUrl, private, chainID, contractAddr, dataRaw, true, value)
		if err != nil {
			t.Fatalf("Upload blobs failed %v", err)
		}
		blobs := utils.EncodeBlobs(dataRaw)
		for i, blob := range blobs {
			kvIdx := kvIdxes[i] % kvEntries
			t.Logf("TryWrite kvIdx: %d kvHash: %x\n", kvIdx, dataHashes[i])
			_, err := shardMgr.TryWrite(kvIdx, blob[:], dataHashes[i])
			if err != nil {
				t.Fatalf("write failed: %v", err)
			}
		}
	}
	return es.NewStorageManager(shardMgr, client)
}

func newMiner(t *testing.T, storageMgr *es.StorageManager, client *eth.PollingClient) *Miner {
	key, _ := crypto.HexToECDSA(private)
	signerAddr := crypto.PubkeyToAddress(key.PublicKey)
	signerFnFactory, signerAddr, err := signer.SignerFactoryFromConfig(
		signer.CLIConfig{},
	)
	if err != nil {
		t.Fatalf("failed to get signer: %v", err)
	}
	defaultConfig := &Config{
		RandomChecks:     2,
		NonceLimit:       1048576,
		MinimumDiff:      new(big.Int).SetUint64(5000000),
		Cutoff:           new(big.Int).SetUint64(60),
		DiffAdjDivisor:   new(big.Int).SetUint64(1024),
		GasPrice:         nil,
		PriorityGasPrice: new(big.Int).SetUint64(10),
		ThreadsPerShard:  1,
		ZKeyFileName:     "blob_poseidon.zkey",
		SignerFnFactory:  signerFnFactory,
		SignerAddr:       signerAddr,
	}
	l1api := NewL1MiningAPI(client, lg)
	zkWorkingDir, _ := filepath.Abs("../prover")
	pvr := prover.NewKZGPoseidonProver(zkWorkingDir, defaultConfig.ZKeyFileName, lg)
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

func readFile() ([]byte, error) {
	txt_170k := "https://www.gutenberg.org/cache/epub/11/pg11.txt"
	resp, err := http.Get(txt_170k)
	if err != nil {
		return nil, fmt.Errorf("error reading blob txtUrl: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading blob txtUrl: %v", err)
	}
	return data, nil
}

func TestMiner_update(t *testing.T) {
	var shard = []uint64{0, 1, 2}
	// Case: happy path
	storageMgr := initStorageManager(t, nil, false)
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
}
