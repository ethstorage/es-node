// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package integration

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/blobs"
	"github.com/ethstorage/go-ethstorage/ethstorage/downloader"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
)

var (
	datadir         string
	randaoSourceURL = os.Getenv("ES_NODE_RANDAO_RPC")
)

const (
	maxBlobsPerTx = 6
	dataFileName  = "shard-%d.dat"
)

func TestMining(t *testing.T) {
	setup(t)
	t.Cleanup(func() {
		teardown(t)
	})

	contract := l1Contract
	lg.Info("Test mining", "l1Endpoint", l1Endpoint, "contract", contract)
	pClient, err := eth.Dial(l1Endpoint, contract, 2, lg)
	if err != nil {
		t.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	storConfig := initStorageConfig(t, pClient, contract, minerAddr)
	lastKv, err := pClient.GetStorageLastBlobIdx(rpc.LatestBlockNumber.Int64())
	if err != nil {
		t.Fatalf("Failed to get lastKvIdx: %v", err)
	}
	lg.Info("lastKv", "lastKv", lastKv)
	curShard := lastKv / storConfig.KvEntriesPerShard
	lg.Info("Current shard", "shardId", curShard)
	shardIds := []uint64{curShard + 1, curShard + 2}
	lg.Info("Shards to mine", "shardIds", shardIds)
	files, err := createDataFiles(storConfig, shardIds)
	if err != nil {
		t.Fatalf("Create data files error: %v", err)
	}
	storConfig.Filenames = files
	miningConfig := initMiningConfig(t, pClient)
	lg.Info("Initialzed mining config", "miningConfig", fmt.Sprintf("%+v", miningConfig))
	shardManager, err := initShardManager(*storConfig)
	if err != nil {
		t.Fatalf("init shard manager error: %v", err)
	}
	storageManager := ethstorage.NewStorageManager(shardManager, pClient)

	resourcesCtx, close := context.WithCancel(context.Background())
	feed := new(event.Feed)

	rc, err := eth.DialRandaoSource(resourcesCtx, randaoSourceURL, l1Endpoint, 2, lg)
	if err != nil {
		t.Fatalf("Failed to connect to the randao source: %v", err)
	}

	l1api := miner.NewL1MiningAPI(pClient, rc, lg)
	pvr := prover.NewKZGPoseidonProver(
		miningConfig.ZKWorkingDir,
		miningConfig.ZKeyFile,
		miningConfig.ZKProverMode,
		miningConfig.ZKProverImpl,
		lg,
	)
	db := rawdb.NewMemoryDatabase()
	br := blobs.NewBlobReader(downloader.NewBlobMemCache(), storageManager, lg)
	mnr := miner.New(miningConfig, db, storageManager, l1api, br, &pvr, feed, lg)
	lg.Info("Initialized miner")

	randaoHeadsSub := event.ResubscribeErr(time.Second*10, func(ctx context.Context, err error) (event.Subscription, error) {
		if err != nil {
			lg.Warn("Resubscribing after failed randao head subscription", "err", err)
		}
		return eth.WatchHeadChanges(resourcesCtx, rc, func(ctx context.Context, sig eth.L1BlockRef) {
			lg.Debug("OnNewRandaoSourceHead", "blockNumber", sig.Number)
			select {
			case mnr.ChainHeadCh <- sig:
			default:
				// Channel is full, skipping
			}
		})
	})
	lg.Info("Randao head subscribed")
	go func() {
		err, ok := <-randaoHeadsSub.Err()
		if !ok {
			return
		}
		lg.Error("Randao heads subscription error", "err", err)
	}()

	if err := fillEmpty(storageManager, shardIds); err != nil {
		t.Fatalf("Failed to fill empty: %v", err)
	}
	prepareData(t, pClient, storageManager)
	mnr.Start()
	var wg sync.WaitGroup
	minedShardSig := make(chan uint64, len(shardIds))
	minedShardCh := make(chan uint64)
	minedShards := make(map[uint64]bool)
	go func() {
		for minedShard := range minedShardCh {
			minedShardSig <- minedShard
			lg.Info("Mined shard", "shard", minedShard)
			if !minedShards[minedShard] {
				minedShards[minedShard] = true
				wg.Done()
			}
		}
	}()
	for i, s := range shardIds {
		feed.Send(protocol.EthStorageSyncDone{
			DoneType: protocol.SingleShardDone,
			ShardId:  s,
		})
		go waitForMined(l1api, contract, mnr.ChainHeadCh, s, minedShardCh)
		wg.Add(1)
		// defer next shard mining so that the started shard can be mined for a while
		if i != len(shardIds)-1 {
			var miningTime time.Duration = 60
			timeout := time.After(miningTime * time.Second)
			select {
			case minedShard := <-minedShardSig:
				lg.Info(fmt.Sprintf("Shard %d successfully mined, will start next shard: %d", minedShard, shardIds[i+1]))
			case <-timeout:
				lg.Info(fmt.Sprintf("Shard %d has been mined for %ds, will start next shard: %d", shardIds[i], miningTime, shardIds[i+1]))
			}
		}
	}
	wg.Wait()
	randaoHeadsSub.Unsubscribe()
	mnr.Close()
	close()
}

func waitForMined(l1api miner.L1API, contract common.Address, chainHeadCh chan eth.L1BlockRef, shardIdx uint64, exitCh chan uint64) {
	for range chainHeadCh {
		info, err := l1api.GetMiningInfo(
			context.Background(),
			contract,
			shardIdx,
		)
		if err != nil {
			lg.Warn("Failed to get es mining info", "error", err.Error())
			continue
		}
		lg.Info("Starting shard mining", "shard", shardIdx, "lastMined", info.BlockMined, "blockNumber", info.LastMineTime)
		if info.BlockMined.Uint64() > 0 {
			lg.Info("Mined new", "shard", shardIdx, "justMined", info.BlockMined)
			exitCh <- shardIdx
			return
		}
	}
}

func initStorageConfig(t *testing.T, client *eth.PollingClient, l1Contract, miner common.Address) *storage.StorageConfig {
	result, err := client.ReadContractField("maxKvSizeBits", nil)
	if err != nil {
		t.Fatal("get maxKvSizeBits", err)
	}
	maxKvSizeBits := new(big.Int).SetBytes(result).Uint64()
	chunkSizeBits := maxKvSizeBits
	result, err = client.ReadContractField("shardEntryBits", nil)
	if err != nil {
		t.Fatal("get shardEntryBits", err)
	}
	shardEntryBits := new(big.Int).SetBytes(result).Uint64()
	return &storage.StorageConfig{
		L1Contract:        l1Contract,
		Miner:             miner,
		KvSize:            1 << maxKvSizeBits,
		ChunkSize:         1 << chunkSizeBits,
		KvEntriesPerShard: 1 << shardEntryBits,
	}
}

func initShardManager(storConfig storage.StorageConfig) (*ethstorage.ShardManager, error) {
	shardManager := ethstorage.NewShardManager(storConfig.L1Contract, storConfig.KvSize, storConfig.KvEntriesPerShard, storConfig.ChunkSize)
	for _, filename := range storConfig.Filenames {
		var err error
		var df *ethstorage.DataFile
		df, err = ethstorage.OpenDataFile(filename)
		if err != nil {
			return nil, fmt.Errorf("open failed: %w", err)
		}
		if df.Miner() != storConfig.Miner {
			lg.Error("Miners mismatch", "fromDataFile", df.Miner(), "fromConfig", storConfig.Miner)
			return nil, fmt.Errorf("miner mismatches datafile")
		}
		shardManager.AddDataFileAndShard(df)
	}

	if shardManager.IsComplete() != nil {
		return nil, fmt.Errorf("shard is not completed")
	}
	return shardManager, nil
}

func fillEmpty(storageMgr *ethstorage.StorageManager, shards []uint64) error {
	start := shards[0] * storageMgr.KvEntries()
	totalEntries := storageMgr.KvEntries()*uint64(len(shards)) - 1
	lg.Info("Filling empty to shards", "start", start, "end", start+totalEntries)
	inserted, next, err := storageMgr.CommitEmptyBlobs(start, start+totalEntries)
	if err != nil {
		return err
	}
	lg.Info("Filling empty done", "inserted", inserted, "next", next)
	return nil
}

func getPayment(l1Client *eth.PollingClient, contract common.Address, batch uint64) (*big.Int, error) {
	uint256Type, _ := abi.NewType("uint256", "", nil)
	dataField, _ := abi.Arguments{{Type: uint256Type}}.Pack(new(big.Int).SetUint64(batch))
	h := crypto.Keccak256Hash([]byte(`upfrontPaymentInBatch(uint256)`))
	calldata := append(h[0:4], dataField...)
	msg := ethereum.CallMsg{
		To:   &contract,
		Data: calldata,
	}
	bs, err := l1Client.CallContract(context.Background(), msg, nil)
	if err != nil {
		lg.Error("Failed to call contract", "error", err.Error())
		return nil, err
	}
	return new(big.Int).SetBytes(bs), nil
}

func prepareData(t *testing.T, l1Client *eth.PollingClient, storageMgr *ethstorage.StorageManager) {
	// fill contract with almost 2 shards of blobs
	data := generateRandomBlobs(int(storageMgr.KvEntries()*2) - 1)
	blobs := utils.EncodeBlobs(data)
	t.Logf("Blobs len %d \n", len(blobs))
	var hashs []common.Hash
	var ids []uint64

	txs := len(blobs) / maxBlobsPerTx
	last := len(blobs) % maxBlobsPerTx
	if last > 0 {
		txs = txs + 1
	}
	t.Logf("tx len %d \n", txs)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	chainID, err := l1Client.ChainID(ctx)
	if err != nil {
		t.Fatalf("Get chain id failed %v", err)
	}
	for i := 0; i < txs; i++ {
		blobsPerTx := maxBlobsPerTx
		if i == txs-1 {
			blobsPerTx = last
		}
		blobGroup := blobs[i*maxBlobsPerTx : i*maxBlobsPerTx+blobsPerTx]
		var blobData []byte
		for _, bd := range blobGroup {
			blobData = append(blobData, bd[:]...)
		}
		if len(blobData) == 0 {
			break
		}
		totalValue, err := getPayment(l1Client, l1Contract, uint64(blobsPerTx))
		if err != nil {
			t.Fatalf("Get payment failed %v", err)
		}
		t.Logf("Get payment totalValue=%v \n", totalValue)
		kvIdxes, dataHashes, err := utils.UploadBlobs(l1Client, l1Endpoint, privateKey, chainID.String(), storageMgr.ContractAddress(), blobData, false, totalValue.String())
		if err != nil {
			t.Fatalf("Upload blobs failed: %v", err)
		}
		t.Logf("kvIdxes=%v \n", kvIdxes)
		t.Logf("dataHashes=%x \n", dataHashes)
		hashs = append(hashs, dataHashes...)
		ids = append(ids, kvIdxes...)
	}
	block, err := l1Client.BlockNumber(context.Background())
	if err != nil {
		t.Fatalf("Failed to get block number %v", err)
	}
	storageMgr.Reset(int64(block))
	err = storageMgr.DownloadAllMetas(context.Background(), 1)
	if err != nil {
		t.Fatalf("Download all metas failed %v", err)
	}
	startKv := storageMgr.Shards()[0] * storageMgr.KvEntries()
	for i := 0; i < len(ids); i++ {
		if ids[i] < startKv {
			continue
		}
		err := storageMgr.CommitBlob(ids[i], blobs[i][:], hashs[i])
		if err != nil {
			t.Fatalf("Failed to commit blob: i=%d, kvIndex=%d, hash=%x, error: %v", i, ids[i], hashs[i], err)
		}
		t.Logf("Committed blob: i=%d, kvIndex=%d, hash=%x", i, ids[i], hashs[i])
	}
}

func createDataFiles(cfg *storage.StorageConfig, shardIds []uint64) ([]string, error) {
	var files []string
	for _, shardIdx := range shardIds {
		fileName := filepath.Join(datadir, fmt.Sprintf(dataFileName, shardIdx))
		if _, err := os.Stat(fileName); err == nil {
			lg.Warn("Creating data file: file already exists, will be overwritten", "file", fileName)
		}
		if cfg.ChunkSize == 0 {
			lg.Crit("Creating data file", "error", "chunk size should not be 0")
		}
		if cfg.KvSize%cfg.ChunkSize != 0 {
			lg.Crit("Creating data file", "error", "max kv size %% chunk size should be 0")
		}
		chunkPerKv := cfg.KvSize / cfg.ChunkSize
		startChunkId := shardIdx * cfg.KvEntriesPerShard * chunkPerKv
		chunkIdxLen := chunkPerKv * cfg.KvEntriesPerShard
		lg.Info("Creating data file", "chunkIdxStart", startChunkId, "chunkIdxLen", chunkIdxLen, "chunkSize", cfg.ChunkSize, "miner", cfg.Miner, "encodeType", ethstorage.ENCODE_BLOB_POSEIDON)

		df, err := ethstorage.Create(fileName, startChunkId, chunkPerKv*cfg.KvEntriesPerShard, 0, cfg.KvSize, ethstorage.ENCODE_BLOB_POSEIDON, cfg.Miner, cfg.ChunkSize)
		if err != nil {
			lg.Crit("Creating data file", "error", err)
		}

		lg.Info("Data file created", "shard", shardIdx, "file", fileName, "datafile", fmt.Sprintf("%+v", df))
		files = append(files, fileName)
	}
	return files, nil
}

func initMiningConfig(t *testing.T, client *eth.PollingClient) *miner.Config {
	miningConfig := &miner.Config{}
	factory, addrFrom, err := signer.SignerFactoryFromConfig(signer.CLIConfig{
		PrivateKey: privateKey,
	})
	if err != nil {
		t.Fatal("SignerFactoryFromConfig err", err)
	}
	miningConfig.SignerFnFactory = factory
	miningConfig.SignerAddr = addrFrom
	result, err := client.ReadContractField("randomChecks", nil)
	if err != nil {
		t.Fatal("get randomChecks", err)
	}
	miningConfig.RandomChecks = new(big.Int).SetBytes(result).Uint64()
	result, err = client.ReadContractField("nonceLimit", nil)
	if err != nil {
		t.Fatal("get nonceLimit", err)
	}
	miningConfig.NonceLimit = new(big.Int).SetBytes(result).Uint64()
	result, err = client.ReadContractField("minimumDiff", nil)
	if err != nil {
		t.Fatal("get minimumDiff", err)
	}
	miningConfig.MinimumDiff = new(big.Int).SetBytes(result)
	result, err = client.ReadContractField("cutoff", nil)
	if err != nil {
		t.Fatal("get cutoff", err)
	}
	miningConfig.Cutoff = new(big.Int).SetBytes(result)
	result, err = client.ReadContractField("diffAdjDivisor", nil)
	if err != nil {
		t.Fatal("get diffAdjDivisor", err)
	}
	miningConfig.DiffAdjDivisor = new(big.Int).SetBytes(result)

	result, err = client.ReadContractField("dcfFactor", nil)
	if err != nil {
		t.Fatal("get dcfFactor", err)
	}
	miningConfig.DcfFactor = new(big.Int).SetBytes(result)
	result, err = client.ReadContractField("startTime", nil)
	if err != nil {
		t.Fatal("get startTime", err)
	}
	miningConfig.StartTime = new(big.Int).SetBytes(result).Uint64()
	result, err = client.ReadContractField("shardEntryBits", nil)
	if err != nil {
		t.Fatal("get shardEntryBits", err)
	}
	miningConfig.ShardEntry = 1 << new(big.Int).SetBytes(result).Uint64()
	result, err = client.ReadContractField("treasuryShare", nil)
	if err != nil {
		t.Fatal("get treasuryShare", err)
	}
	miningConfig.TreasuryShare = new(big.Int).SetBytes(result).Uint64()
	result, err = client.ReadContractField("storageCost", nil)
	if err != nil {
		t.Fatal("get storageCost", err)
	}
	miningConfig.StorageCost = new(big.Int).SetBytes(result)
	result, err = client.ReadContractField("prepaidAmount", nil)
	if err != nil {
		t.Fatal("get prepaidAmount", err)
	}
	miningConfig.PrepaidAmount = new(big.Int).SetBytes(result)
	proverPath, _ := filepath.Abs(prPath)
	zkeyFull := filepath.Join(proverPath, prover.SnarkLib, zkey2Name)
	if _, err := os.Stat(zkeyFull); os.IsNotExist(err) {
		t.Fatalf("%s not found", zkeyFull)
	}
	miningConfig.ZKeyFile = zkeyFull
	miningConfig.ZKWorkingDir = proverPath
	miningConfig.ZKProverMode = 2
	miningConfig.ZKProverImpl = 2
	miningConfig.ThreadsPerShard = 2
	miningConfig.MinimumProfit = new(big.Int).SetInt64(-1e18)
	return miningConfig
}

func setup(t *testing.T) {
	datadir = t.TempDir()
	err := os.MkdirAll(datadir, 0700)
	if err != nil {
		t.Fatalf("Failed to create datadir: %v", err)
	}
	t.Logf("datadir %s", datadir)
}

func teardown(t *testing.T) {
	err := os.RemoveAll(datadir)
	if err != nil {
		t.Errorf("Failed to remove datadir: %v", err)
	}
}
