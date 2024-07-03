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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
)

const (
	maxBlobsPerTx = 4
	dataFileName  = "shard-%d.dat"
)

// TODO: test 2 shards
var shardIds = []uint64{0}

func TestMining(t *testing.T) {
	contract := l1Contract
	lg.Info("Test mining", "l1Endpoint", l1Endpoint, "contract", contract)
	pClient, err := eth.Dial(l1Endpoint, contract, 12, lg)
	if err != nil {
		t.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	lastKv, err := pClient.GetStorageLastBlobIdx(rpc.LatestBlockNumber.Int64())
	if err != nil {
		lg.Error("Failed to get lastKvIdx", "error", err)
	} else {
		lg.Info("lastKv", "lastKv", lastKv)
	}
	if lastKv != 0 {
		t.Fatalf("A newly deployed storage contract is required")
	}
	storConfig := initStorageConfig(t, pClient, contract, minerAddr)
	files, err := createDataFiles(storConfig)
	if err != nil {
		t.Fatalf("Create data files error: %v", err)
	}
	storConfig.Filenames = files
	miningConfig := initMiningConfig(t, contract, pClient)
	lg.Info("Initialzed mining config", "miningConfig", fmt.Sprintf("%+v", miningConfig))
	defer cleanFiles(miningConfig.ZKWorkingDir)
	shardManager, err := initShardManager(*storConfig)
	if err != nil {
		t.Fatalf("init shard manager error: %v", err)
	}
	storageManager := ethstorage.NewStorageManager(shardManager, pClient)

	resourcesCtx, close := context.WithCancel(context.Background())
	feed := new(event.Feed)

	l1api := miner.NewL1MiningAPI(pClient, nil, lg)
	pvr := prover.NewKZGPoseidonProver(
		miningConfig.ZKWorkingDir,
		miningConfig.ZKeyFileName,
		miningConfig.ZKProverMode,
		miningConfig.ZKProverImpl,
		lg,
	)
	db := rawdb.NewMemoryDatabase()
	mnr := miner.New(miningConfig, db, storageManager, l1api, func(kvIdx uint64, kvHash common.Hash) ([]byte, bool, error) {
		kvData, exist, err := storageManager.TryRead(kvIdx, int(storageManager.MaxKvSize()), kvHash)
		if err != nil {
			return nil, false, err
		}
		if !exist {
			return nil, false, fmt.Errorf("kv not found: index=%d", kvIdx)
		}
		return kvData, false, nil
	}, &pvr, feed, lg)
	lg.Info("Initialized miner")

	l1HeadsSub := event.ResubscribeErr(time.Second*10, func(ctx context.Context, err error) (event.Subscription, error) {
		if err != nil {
			lg.Warn("Resubscribing after failed L1 subscription", "err", err)
		}
		return eth.WatchHeadChanges(resourcesCtx, pClient, func(ctx context.Context, sig eth.L1BlockRef) {
			select {
			case mnr.ChainHeadCh <- sig:
			default:
				// Channel is full, skipping
			}
		})
	})
	go func() {
		err, ok := <-l1HeadsSub.Err()
		if !ok {
			return
		}
		lg.Error("L1 heads subscription error", "err", err)
	}()

	mnr.Start()
	prepareData(t, pClient, storageManager, miningConfig.StorageCost.String())
	fillEmpty(t, pClient, storageManager)

	var wg sync.WaitGroup
	minedShardCh := make(chan uint64)
	for _, s := range shardIds {
		feed.Send(protocol.EthStorageSyncDone{
			DoneType: protocol.SingleShardDone,
			ShardId:  s,
		})
		info, err := l1api.GetMiningInfo(
			context.Background(),
			contract,
			s,
		)
		if err != nil {
			t.Fatalf("Failed to get es mining info for shard %d: %v", s, err)
		}
		go waitForMined(l1api, contract, mnr.ChainHeadCh, s, info.BlockMined.Uint64(), minedShardCh)
		wg.Add(1)
		time.Sleep(360 * time.Second)
	}

	go func() {
		minedShards := make(map[uint64]bool)
		for minedShard := range minedShardCh {
			if !minedShards[minedShard] {
				lg.Info("Mined shard", "shard", minedShard)
				minedShards[minedShard] = true
				wg.Done()
				lg.Info("wait group done")
			}
		}
	}()
	lg.Info("wait group waiting")
	wg.Wait()
	l1HeadsSub.Unsubscribe()
	mnr.Close()
	close()
}

func cleanFiles(proverDir string) {
	for _, shardId := range shardIds {
		fileName := fmt.Sprintf(dataFileName, shardId)
		if _, err := os.Stat(fileName); !os.IsNotExist(err) {
			err = os.Remove(fileName)
			if err != nil {
				fmt.Println(err)
			}
		}
	}

	folderPath := filepath.Join(proverDir, "snarkbuild")
	files, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, file := range files {
		if !strings.HasPrefix(file.Name(), ".") {
			err = os.RemoveAll(filepath.Join(folderPath, file.Name()))
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}

func waitForMined(l1api miner.L1API, contract common.Address, chainHeadCh chan eth.L1BlockRef, shardIdx, lastMined uint64, exitCh chan uint64) {
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
		if info.BlockMined.Uint64() > lastMined {
			lg.Info("Mined new", "shard", shardIdx, "lastMined", lastMined, "justMined", info.BlockMined)
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

func fillEmpty(t *testing.T, l1Client *eth.PollingClient, storageMgr *ethstorage.StorageManager) {
	lg.Info("Filling empty started")
	totalBlobs := storageMgr.KvEntries() * uint64(len(shardIds))
	lastKvIdx := storageMgr.LastKvIndex()
	lg.Info("Filling empty", "lastBlobIdx", lastKvIdx, "totalBlobs", totalBlobs)

	inserted, next, err := storageMgr.CommitEmptyBlobs(lastKvIdx, totalBlobs-1)
	if err != nil {
		t.Fatalf("Commit empty blobs failed %v", err)
	}
	lg.Info("Filling empty done", "inserted", inserted, "next", next)
}

func prepareData(t *testing.T, l1Client *eth.PollingClient, storageMgr *ethstorage.StorageManager, value string) {
	data := generateRandomContent(124 * 10)
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
		max := maxBlobsPerTx
		if i == txs-1 {
			max = last
		}
		blobGroup := blobs[i*maxBlobsPerTx : i*maxBlobsPerTx+max]
		var blobData []byte
		for _, bd := range blobGroup {
			blobData = append(blobData, bd[:]...)
		}
		if len(blobData) == 0 {
			break
		}
		kvIdxes, dataHashes, err := utils.UploadBlobs(l1Client, l1Endpoint, privateKey, chainID.String(), storageMgr.ContractAddress(), blobData, false, value)
		if err != nil {
			t.Fatalf("Upload blobs failed %v", err)
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
	totalKvs := len(shardIds) * int(storageMgr.KvEntries())
	limit := totalKvs
	if limit > len(ids) {
		limit = len(ids)
	}
	for i := 0; i < limit; i++ {
		err := storageMgr.CommitBlob(ids[i], blobs[i][:], hashs[i])
		if err != nil {
			t.Fatalf("Failed to commit blob: i=%d, id=%d, error: %v", i, ids[i], err)
		}
	}
	_, _, err = storageMgr.CommitEmptyBlobs(uint64(limit), uint64(totalKvs)-1)
	if err != nil {
		t.Fatalf("Commit empty blobs failed %v", err)
	}
}

func createDataFiles(cfg *storage.StorageConfig) ([]string, error) {
	var files []string
	for _, shardIdx := range shardIds {
		fileName := fmt.Sprintf(dataFileName, shardIdx)
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

func initMiningConfig(t *testing.T, l1Contract common.Address, client *eth.PollingClient) *miner.Config {
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

	miningConfig.ZKeyFileName = zkey2Name
	proverPath, _ := filepath.Abs(prPath)
	miningConfig.ZKWorkingDir = proverPath
	miningConfig.ZKProverMode = 2
	miningConfig.ZKProverImpl = 1
	miningConfig.ThreadsPerShard = 2
	miningConfig.MinimumProfit = new(big.Int).SetInt64(-1e18)
	return miningConfig
}
