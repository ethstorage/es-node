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

	"github.com/ethereum/go-ethereum/common"
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

type mockNode struct {
	pollingClient  *eth.PollingClient
	storageConfig  *storage.StorageConfig
	shardManager   *ethstorage.ShardManager
	storageManager *ethstorage.StorageManager
	feed           *event.Feed
	miner          *miner.Miner
	l1api          miner.L1API
}

func (n *mockNode) subscribeL1() event.Subscription {
	l1HeadsSub := event.ResubscribeErr(time.Second*10, func(ctx context.Context, err error) (event.Subscription, error) {
		if err != nil {
			lg.Warn("Resubscribing after failed L1 subscription", "err", err)
		}
		lg.Info("Subscribe L1")
		return eth.WatchHeadChanges(context.Background(), n.pollingClient, func(ctx context.Context, sig eth.L1BlockRef) {
			select {
			case n.miner.ChainHeadCh <- sig:
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
	return l1HeadsSub
}

func (n *mockNode) startAndWaitForMined(shardIdx uint64, exitCh chan uint64) {
	n.feed.Send(protocol.EthStorageSyncDone{
		DoneType: protocol.SingleShardDone,
		ShardId:  shardIdx,
	})
	lastInfo, err := n.l1api.GetMiningInfo(context.Background(), n.shardManager.ContractAddress(), shardIdx)
	if err != nil {
		lg.Crit("Failed to get es mining info", "shard", shardIdx, "error", err.Error())
	}
	lastMined := lastInfo.BlockMined.Uint64()
	for range n.miner.ChainHeadCh {
		info, err := n.l1api.GetMiningInfo(context.Background(), n.shardManager.ContractAddress(), shardIdx)
		if err != nil {
			lg.Warn("Failed to get es mining info", "shard", shardIdx, "error", err.Error())
			continue
		}
		if info.BlockMined.Uint64() > lastMined {
			lg.Info("Mined new", "shard", shardIdx, "lastMined", lastMined, "justMined", info.BlockMined)
			exitCh <- shardIdx
			return
		}
	}
}

func (n *mockNode) waitForInitShard1(exitCh chan uint64, wg *sync.WaitGroup) {
	for range n.miner.ChainHeadCh {
		lastKvIdx, err := n.pollingClient.GetStorageLastBlobIdx(rpc.LatestBlockNumber.Int64())
		if err != nil {
			lg.Warn("Failed to get last kv index", "error", err.Error())
			continue
		}
		lg.Info("Waiting for init new shard", "lastKvIdx", lastKvIdx)
		if lastKvIdx >= n.storageConfig.KvEntriesPerShard {
			break
		}
	}
	fileName := createDataFile(n.storageConfig, 1)
	n.storageConfig.Filenames = append(n.storageConfig.Filenames, fileName)
	df, err := ethstorage.OpenDataFile(fileName)
	if err != nil {
		lg.Crit("Open data file failed", "fileName", fileName)
	}
	n.shardManager.AddDataFileAndShard(df)
	err = fillEmpty(n.storageManager, n.shardManager.KvEntries())
	if err != nil {
		lg.Crit("Failed to fill empty", "error", err)
	}

	go n.startAndWaitForMined(1, exitCh)
	wg.Add(1)
}

func TestMining(t *testing.T) {

	lg.Info("Test mining", "l1Endpoint", l1Endpoint, "contract", l1Contract)
	node, err := initNode()
	if err != nil {
		lg.Crit("Failed to init node", "error", err)
	}
	l1HeadsSub := node.subscribeL1()
	node.miner.Start()

	err = fillEmpty(node.storageManager, node.shardManager.KvEntries())
	if err != nil {
		lg.Crit("Failed to fill empty", "error", err)
	}

	go prepareData(node.pollingClient, node.shardManager, node.storageManager, 14)

	var wg sync.WaitGroup
	minedShardSig := make(chan uint64, 2)
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
	go node.startAndWaitForMined(0, minedShardCh)
	wg.Add(1)

	// defer next shard mining so that the started shard can be mined for a while
	var miningTime time.Duration = 600
	timeout := time.After(miningTime * time.Second)
	select {
	case minedShard := <-minedShardSig:
		lg.Info("Shard successfully mined", "minedShard", minedShard)
	case <-timeout:
		lg.Info(fmt.Sprintf("Shard 0 has been mined for %ds, will start shard 1", miningTime))
	}

	go node.waitForInitShard1(minedShardCh, &wg)

	wg.Wait()
	l1HeadsSub.Unsubscribe()
	node.miner.Close()
	node.storageManager.Close()
}

func initNode() (*mockNode, error) {
	pClient, err := eth.Dial(l1Endpoint, l1Contract, lg)
	if err != nil {
		lg.Crit("Failed to connect L1", "error", err)
	}

	// for now we need a new contract for each test
	lastKv, err := pClient.GetStorageLastBlobIdx(rpc.LatestBlockNumber.Int64())
	if err != nil || lastKv != 0 {
		lg.Crit("A newly deployed storage contract is required", "lastKv", lastKv, "err", err)
	}
	storConfig, err := initStorageConfig(pClient)
	if err != nil {
		lg.Crit("Failed to init storage config", "error", err)
	}
	// init storage with a default shard 0 data file
	fileName := createDataFile(storConfig, 0)
	storConfig.Filenames = append(storConfig.Filenames, fileName)

	shardManager, err := initShardManager(*storConfig)
	if err != nil {
		return nil, err
	}
	miningConfig, err := initMiningConfig(pClient)
	if err != nil {
		return nil, err
	}
	lg.Info("Initialzed mining config")
	pvr := prover.NewKZGPoseidonProver(miningConfig.ZKWorkingDir, miningConfig.ZKeyFileName, 2, lg)
	feed := new(event.Feed)
	l1api := miner.NewL1MiningAPI(pClient, lg)
	storageManager := ethstorage.NewStorageManager(shardManager, pClient)
	mnr := miner.New(miningConfig, storageManager, l1api, &pvr, feed, lg)
	lg.Info("Initialized miner")

	node := &mockNode{
		pollingClient:  pClient,
		storageConfig:  storConfig,
		shardManager:   shardManager,
		storageManager: storageManager,
		feed:           feed,
		miner:          mnr,
		l1api:          l1api,
	}
	return node, nil
}

func prepareData(pollingClient *eth.PollingClient, shardManager *ethstorage.ShardManager, storageManager *ethstorage.StorageManager, blobCount int) error {
	result, err := pollingClient.ReadContractField("storageCost")
	if err != nil {
		return err
	}
	blobs := 3
	cost := new(big.Int).SetBytes(result).String()
	for j := blobCount; j > 0; j = j - 3 {
		time.Sleep(3 * time.Minute)
		if j < 3 {
			blobs = j
		}
		err := uploadBlobs(pollingClient, storageManager, cost, uint64(blobs))
		if err != nil {
			return err
		}
	}
	return nil
}

func initStorageConfig(client *eth.PollingClient) (*storage.StorageConfig, error) {
	result, err := client.ReadContractField("maxKvSizeBits")
	if err != nil {
		return nil, err
	}
	maxKvSizeBits := new(big.Int).SetBytes(result).Uint64()
	chunkSizeBits := maxKvSizeBits
	result, err = client.ReadContractField("shardEntryBits", nil)
	if err != nil {
		return nil, err
	}
	shardEntryBits := new(big.Int).SetBytes(result).Uint64()
	return &storage.StorageConfig{
		L1Contract:        l1Contract,
		Miner:             minerAddr,
		KvSize:            1 << maxKvSizeBits,
		ChunkSize:         1 << chunkSizeBits,
		KvEntriesPerShard: 1 << shardEntryBits,
	}, nil
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

func fillEmpty(storageMgr *ethstorage.StorageManager, entriesToFill uint64) error {
	lastKvIdx := storageMgr.LastKvIndex()
	lg.Info("Filling empty", "lastKvIdx", lastKvIdx, "entriesToFill", entriesToFill)
	inserted, next, err := storageMgr.CommitEmptyBlobs(lastKvIdx, entriesToFill+lastKvIdx-1)
	if err != nil {
		return err
	}
	lg.Info("Filling empty done", "inserted", inserted, "next", next)
	return nil
}

func uploadBlobs(l1Client *eth.PollingClient, storageMgr *ethstorage.StorageManager, value string, blobLen uint64) error {
	data := generateRandomContent(124 * int(blobLen))
	blobs := utils.EncodeBlobs(data)
	var hashs []common.Hash
	var ids []uint64

	txs := len(blobs) / maxBlobsPerTx
	last := len(blobs) % maxBlobsPerTx
	if last > 0 {
		txs = txs + 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	chainID, err := l1Client.ChainID(ctx)
	if err != nil {
		return err
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
			return err
		}
		for j := 0; j < len(kvIdxes); j++ {
			lg.Info("Upload data", "id", kvIdxes[j], "hash", dataHashes[j])
		}
		hashs = append(hashs, dataHashes...)
		ids = append(ids, kvIdxes...)
	}
	block, err := l1Client.BlockNumber(context.Background())
	if err != nil {
		return err
	}
	storageMgr.Reset(int64(block))
	err = storageMgr.DownloadAllMetas(context.Background(), 1)
	if err != nil {
		return err
	}
	limit := len(ids) - 1
	for i := 0; i < limit; i++ {
		err := storageMgr.CommitBlob(ids[i], blobs[i][:], hashs[i])
		if err != nil {
			lg.Crit("Failed to commit blob", "i", i, "id", ids[i], "error", err)
		}
		lg.Info("Commit blob", "id", ids[i], "hash", hashs[i])
	}
	return nil
}

func createDataFile(cfg *storage.StorageConfig, shardIdx uint64) string {
	fileName := fmt.Sprintf(dataFileName, shardIdx)
	if _, err := os.Stat(fileName); err == nil {
		lg.Warn("Creating data file: file already exists, will be overwritten", "file", fileName)
	}
	chunkPerKv := cfg.KvSize / cfg.ChunkSize
	startChunkId := shardIdx * cfg.KvEntriesPerShard * chunkPerKv
	chunkIdxLen := chunkPerKv * cfg.KvEntriesPerShard
	lg.Info("Creating data file", "chunkIdxLen", chunkIdxLen, "miner", cfg.Miner)

	df, err := ethstorage.Create(fileName, startChunkId, chunkPerKv*cfg.KvEntriesPerShard, 0, cfg.KvSize, ethstorage.ENCODE_BLOB_POSEIDON, cfg.Miner, cfg.ChunkSize)
	if err != nil {
		lg.Crit("Create data file failed", "error", err)
	}
	lg.Info("Data file created", "shard", shardIdx, "file", fileName, "startKv", df.KvIdxStart(), "endKv", df.KvIdxEnd())
	cfg.Filenames = append(cfg.Filenames, fileName)
	return fileName
}

func initMiningConfig(client *eth.PollingClient) (*miner.Config, error) {
	miningConfig := &miner.Config{}
	factory, addrFrom, err := signer.SignerFactoryFromConfig(signer.CLIConfig{
		PrivateKey: privateKey,
	})
	if err != nil {
		return nil, err
	}
	miningConfig.SignerFnFactory = factory
	miningConfig.SignerAddr = addrFrom
	result, err := client.ReadContractField("randomChecks", nil)
	if err != nil {
		return nil, err
	}
	miningConfig.RandomChecks = new(big.Int).SetBytes(result).Uint64()
	result, err = client.ReadContractField("nonceLimit", nil)
	if err != nil {
		return nil, err
	}
	miningConfig.NonceLimit = new(big.Int).SetBytes(result).Uint64()
	result, err = client.ReadContractField("minimumDiff", nil)
	if err != nil {
		return nil, err
	}
	miningConfig.MinimumDiff = new(big.Int).SetBytes(result)
	result, err = client.ReadContractField("cutoff", nil)
	if err != nil {
		return nil, err
	}
	miningConfig.Cutoff = new(big.Int).SetBytes(result)
	result, err = client.ReadContractField("diffAdjDivisor", nil)
	if err != nil {
		return nil, err
	}
	miningConfig.DiffAdjDivisor = new(big.Int).SetBytes(result)

	result, err = client.ReadContractField("dcfFactor", nil)
	if err != nil {
		return nil, err
	}
	miningConfig.DcfFactor = new(big.Int).SetBytes(result)
	result, err = client.ReadContractField("startTime", nil)
	if err != nil {
		return nil, err
	}
	miningConfig.StartTime = new(big.Int).SetBytes(result).Uint64()
	result, err = client.ReadContractField("shardEntryBits", nil)
	if err != nil {
		return nil, err
	}
	miningConfig.ShardEntry = 1 << new(big.Int).SetBytes(result).Uint64()
	result, err = client.ReadContractField("treasuryShare", nil)
	if err != nil {
		return nil, err
	}
	miningConfig.TreasuryShare = new(big.Int).SetBytes(result).Uint64()
	result, err = client.ReadContractField("storageCost", nil)
	if err != nil {
		return nil, err
	}
	miningConfig.StorageCost = new(big.Int).SetBytes(result)
	result, err = client.ReadContractField("prepaidAmount", nil)
	if err != nil {
		return nil, err
	}
	miningConfig.PrepaidAmount = new(big.Int).SetBytes(result)

	miningConfig.ZKeyFileName = zkey2Name
	proverPath, _ := filepath.Abs(prPath)
	zkeyFull := filepath.Join(proverPath, prover.SnarkLib, miningConfig.ZKeyFileName)
	if _, err := os.Stat(zkeyFull); os.IsNotExist(err) {
		return nil, err
	}
	miningConfig.ZKWorkingDir = proverPath
	miningConfig.ZKProverMode = 2
	miningConfig.ThreadsPerShard = 2
	miningConfig.MinimumProfit = new(big.Int).SetInt64(-1e18)
	return miningConfig, nil
}
