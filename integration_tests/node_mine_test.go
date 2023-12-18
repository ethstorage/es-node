// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
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

var shardIds = []uint64{0}

// to use a new contract and override data files:
// go test -run ^TestMining$ github.com/ethstorage/go-ethstorage/ethstorage/node -v count=1 -contract=0x9BE8dEbb712A3c0dD163Da85D3aF867793Aef3E6 -init=true
func TestMining(t *testing.T) {
	contract := contractAddr1GB
	// contract := common.HexToAddress("0xf8928E47ab82912025FD45589f2444fF3a0FDa73")
	flagContract := getArg("-contract")
	if flagContract != "" {
		contract = common.HexToAddress(flagContract)
		lg.Info("Use the contract address from flag", "contract", flagContract)
	}
	init := false
	flagInitStorage := getArg("-init")
	if flagInitStorage == "true" {
		lg.Info("Data files will be removed")
		init = true
	}

	intialized := false
	existFile := 0
	for _, shardId := range shardIds {
		fileName := fmt.Sprintf(dataFileName, shardId)
		if _, err := os.Stat(fileName); !os.IsNotExist(err) {
			if init {
				os.Remove(fileName)
			} else {
				existFile++
			}
		}
	}

	if existFile == 2 {
		intialized = true
		lg.Info("Data files already exist, will start mining")
	}
	// if existFile == 1 {
	// 	lg.Crit("One of the data files is missing, please check")
	// }
	if existFile == 0 {
		lg.Info("Will initialize the data files, please make sure you use a new contract")
	}

	pClient, err := eth.Dial(l1Endpoint, contract, lg)
	if err != nil {
		t.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	storConfig, err := initStorageConfig(context.Background(), pClient.Client, contract, minerAddr)
	if err != nil {
		t.Fatalf("Failed to init storage client: %v", err)
	}
	var files []string
	if !intialized {
		f, err := CreateDataFiles(storConfig)
		if err != nil {
			t.Fatalf("Create data files error: %v", err)
		}
		files = f
	} else {
		for _, shardIdx := range shardIds {
			fileName := fmt.Sprintf(dataFileName, shardIdx)
			files = append(files, fileName)
		}
	}
	storConfig.Filenames = files
	if os.Getenv(pkName) == "" {
		t.Fatal("No private key in env", pkName)
	}
	signerCfg := signer.CLIConfig{
		PrivateKey: os.Getenv(pkName),
	}
	factory, addrFrom, err := signer.SignerFactoryFromConfig(signerCfg)
	if err != nil {
		t.Fatal("SignerFactoryFromConfig err", err)
	}
	miningConfig := &miner.DefaultConfig
	miningConfig.SignerFnFactory = factory
	miningConfig.SignerAddr = addrFrom
	zkWorkingDir, err := filepath.Abs("../prover")
	if err != nil {
		t.Fatalf("Get zkWorkingDir error: %v", err)
	}
	miningConfig.ZKWorkingDir = zkWorkingDir

	shardManager, err := initShardManager(*storConfig)
	if err != nil {
		t.Fatalf("init shard manager error: %v", err)
	}
	storageManager := ethstorage.NewStorageManager(shardManager, pClient)

	resourcesCtx, close := context.WithCancel(context.Background())
	feed := new(event.Feed)

	l1api := miner.NewL1MiningAPI(pClient, lg)
	pvr := prover.NewKZGPoseidonProver(zkWorkingDir, "blob_poseidon.zkey", lg)
	mnr := miner.New(miningConfig, storageManager, l1api, &pvr, feed, lg)
	log.Info("Initialized miner")

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
	if !intialized {
		prepareData(t, pClient, storageManager)
		// fillEmpty(t, pClient, storageManager)
	}
	mnr.Start()
	feed.Send(protocol.EthStorageSyncDone{
		DoneType: protocol.SingleShardDone,
		ShardId:  0,
	})
	time.Sleep(360 * time.Second)
	feed.Send(protocol.EthStorageSyncDone{
		DoneType: protocol.SingleShardDone,
		ShardId:  1,
	})

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	mnr.Close()

	l1HeadsSub.Unsubscribe()
	close()
}

func initShardManager(storConfig storage.StorageConfig) (*es.ShardManager, error) {
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
	empty := make([]byte, 0)
	block, err := l1Client.BlockNumber(context.Background())
	if err != nil {
		t.Fatalf("Failed to get block number %v", err)
	}
<<<<<<< HEAD:integration_tests/node_mine_test.go
	storageMgr.Reset(int64(block))
	lastBlobIdx, err := storageMgr.LastKvIndex()
	if err != nil {
		t.Fatalf("get lastBlobIdx for FillEmptyKV fail, err: %s", err.Error())
	}
	limit := storageMgr.KvEntries() * uint64(len(shardIds))
=======
	n.storageManager.Reset(int64(block))
	lastBlobIdx := n.storageManager.LastKvIndex()
	limit := n.storageManager.KvEntries() * uint64(len(shardIds))
>>>>>>> 6985e6968ebf0cb626a7764a9f74907a6870feec:ethstorage/node/node_mine_test.go
	for idx := lastBlobIdx; idx < limit; idx++ {
		err = storageMgr.CommitBlob(idx, empty, common.Hash{})
		if err != nil {
			t.Fatalf("write empty to kv file fail, index: %d; error: %s", idx, err.Error())
		}
	}
}

func prepareData(t *testing.T, l1Client *eth.PollingClient, storageMgr *ethstorage.StorageManager) {
	data, err := getSourceData()
	if err != nil {
		t.Fatalf("Get source data failed %v", err)
	}
	blobs := utils.EncodeBlobs(data)
	t.Logf("Blobs len %d \n", len(blobs))
	var hashs []common.Hash
	var ids []uint64

	value := hexutil.EncodeUint64(500000000000000)
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
		kvIdxes, dataHashes, err := utils.UploadBlobs(l1Client, l1Endpoint, os.Getenv(pkName), chainID.String(), storageMgr.ContractAddress(), blobData, false, value)
		if err != nil {
			t.Fatalf("Upload blobs failed %v", err)
		}
		t.Logf("kvIdxes=%v \n", kvIdxes)
		t.Logf("dataHashes=%x \n", dataHashes)
		hashs = append(hashs, dataHashes...)
		ids = append(ids, kvIdxes...)
	}
	block, err := l1Client.BlockNumber(ctx)
	if err != nil {
		t.Fatalf("Failed to get block number %v", err)
	}
	storageMgr.Reset(int64(block))
	for i := 0; i < len(shardIds)*int(storageMgr.KvEntries()); i++ {
		err := storageMgr.CommitBlob(ids[i], blobs[i][:], hashs[i])
		if err != nil {
			t.Fatalf("Failed to commit blob: id %d, error: %v", ids[i], err)
		}
	}
}

func CreateDataFiles(cfg *storage.StorageConfig) ([]string, error) {
	var files []string
	for _, shardIdx := range shardIds {
		fileName := fmt.Sprintf(dataFileName, shardIdx)
		if _, err := os.Stat(fileName); err == nil {
			lg.Crit("Creating data file", "error", "file already exists, will not overwrite", "file", fileName)
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
		log.Info("Creating data file", "chunkIdxStart", startChunkId, "chunkIdxLen", chunkIdxLen, "chunkSize", cfg.ChunkSize, "miner", cfg.Miner, "encodeType", es.ENCODE_BLOB_POSEIDON)

		df, err := es.Create(fileName, startChunkId, chunkPerKv*cfg.KvEntriesPerShard, 0, cfg.KvSize, es.ENCODE_BLOB_POSEIDON, cfg.Miner, cfg.ChunkSize)
		if err != nil {
			log.Crit("Creating data file", "error", err)
		}

		lg.Info("Data file created", "shard", shardIdx, "file", fileName, "datafile", fmt.Sprintf("%+v", df))
		files = append(files, fileName)
	}
	return files, nil
}

func getSourceData() ([]byte, error) {
	txt_4m := "https://www.gutenberg.org/cache/epub/10/pg10.txt"
	resp, err := http.Get(txt_4m)
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

func getArg(paramName string) string {
	for _, arg := range os.Args {
		pair := strings.Split(arg, "=")
		if len(pair) == 2 && pair[0] == paramName {
			paramValue := pair[1]
			fmt.Printf("%s=%s\n", paramName, paramValue)
			return paramValue
		}
	}
	return ""
}
