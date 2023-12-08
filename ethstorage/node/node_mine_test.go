// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package node

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
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
	"golang.org/x/term"
)

const (
	rpcUrl            = "http://65.108.236.27:8545"
	chainID           = "7011893058"
	kvEntriesPerShard = 16
	kvSize            = 128 * 1024
	chunkSize         = 128 * 1024
	maxBlobsPerTx     = 4
	dataFileName      = "shard-%d.dat"
)

var (
	minerAddr    = common.HexToAddress("0x04580493117292ba13361D8e9e28609ec112264D")
	contractAddr = common.HexToAddress("0x188aac000e21ec314C5694bB82035b72210315A8")
	private      = "95eb6ffd2ae0b115db4d1f0d58388216f9d026896696a5211d77b5f14eb5badf"
	shardIds     = []uint64{0, 1}
	l            = esLog.NewLogger(esLog.CLIConfig{
		Level:  "info",
		Format: "text",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	})
)

// to use a new contract and override data files:
// go test -run ^TestMining$ github.com/ethstorage/go-ethstorage/ethstorage/node -v count=1 -contract=0x9BE8dEbb712A3c0dD163Da85D3aF867793Aef3E6 -init=true
func TestMining(t *testing.T) {
	contract := contractAddr
	flagContract := getArg("-contract")
	if flagContract != "" {
		contract = common.HexToAddress(flagContract)
		l.Info("Use the contract address from flag", "contract", flagContract)
	}
	init := false
	flagInitStorage := getArg("-init")
	if flagInitStorage == "true" {
		l.Info("Data files will be removed")
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
		l.Info("Data files already exist, will start mining")
	}
	if existFile == 1 {
		l.Crit("One of the data files is missing, please check")
	}
	if existFile == 0 {
		l.Info("Will initialize the data files, please make sure you use a new contract")
	}
	storConfig := storage.StorageConfig{
		KvSize:            kvSize,
		ChunkSize:         chunkSize,
		KvEntriesPerShard: kvEntriesPerShard,
		L1Contract:        contract,
		Miner:             minerAddr,
	}
	var files []string
	if !intialized {
		f, err := CreateDataFiles(&storConfig)
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

	signerCfg := signer.CLIConfig{
		// Endpoint: "http://65.108.236.27:8550",
		// Address:  "0x13259366de990b0431e2c97cea949362bb68df12",
		PrivateKey: private,
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
	l1 := &eth.L1EndpointConfig{
		L1NodeAddr: rpcUrl,
	}
	cfg := &Config{
		Storage:             storConfig,
		L1:                  *l1,
		L1EpochPollInterval: time.Second * 10,
		Mining:              miningConfig,
	}
	n, err := New(context.Background(), cfg, l, "")
	if err != nil {
		t.Fatalf("Create new node failed %v", err)
	}

	if !intialized {
		// prepareData(t, n, contract)
		fillEmpty(t, n, contract)
	}
	n.startL1(cfg)
	n.miner.Start()
	n.feed.Send(protocol.EthStorageSyncDone{
		DoneType: protocol.SingleShardDone,
		ShardId:  0,
	})
	time.Sleep(360 * time.Second)
	n.feed.Send(protocol.EthStorageSyncDone{
		DoneType: protocol.SingleShardDone,
		ShardId:  1,
	})

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	n.miner.Close()
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

func fillEmpty(t *testing.T, n *EsNode, contract common.Address) {
	empty := make([]byte, 0)
	block, err := n.l1Source.BlockNumber(context.Background())
	if err != nil {
		t.Fatalf("Failed to get block number %v", err)
	}
	n.storageManager.Reset(int64(block))
	lastBlobIdx := n.storageManager.LastKvIndex()
	limit := n.storageManager.KvEntries() * uint64(len(shardIds))
	for idx := lastBlobIdx; idx < limit; idx++ {
		err = n.storageManager.CommitBlob(idx, empty, common.Hash{})
		if err != nil {
			t.Fatalf("write empty to kv file fail, index: %d; error: %s", idx, err.Error())
		}
	}
}

func prepareData(t *testing.T, n *EsNode, contract common.Address) {
	data, err := getSourceData()
	if err != nil {
		t.Fatalf("Get source data failed %v", err)
	}
	blobs := utils.EncodeBlobs(data)
	t.Logf("Blobs len %d \n", len(blobs))
	var hashs []common.Hash
	var ids []uint64

	value := hexutil.EncodeUint64(10000000000000)
	txs := len(blobs) / maxBlobsPerTx
	last := len(blobs) % maxBlobsPerTx
	if last > 0 {
		txs = txs + 1
	}
	t.Logf("tx len %d \n", txs)
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
		kvIdxes, dataHashes, err := utils.UploadBlobs(n.l1Source, rpcUrl, private, chainID, contract, blobData, false, value)
		if err != nil {
			t.Fatalf("Upload blobs failed %v", err)
		}
		hashs = append(hashs, dataHashes...)
		ids = append(ids, kvIdxes...)
		t.Logf("ids=%v \n", ids)
	}
	block, err := n.l1Source.BlockNumber(context.Background())
	if err != nil {
		t.Fatalf("Failed to get block number %v", err)
	}
	n.storageManager.Reset(int64(block))
	for i := 0; i < len(shardIds)*kvEntriesPerShard; i++ {
		err := n.storageManager.CommitBlob(ids[i], blobs[i][:], hashs[i])
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
			l.Crit("Creating data file", "error", "file already exists, will not overwrite", "file", fileName)
		}
		if cfg.ChunkSize == 0 {
			l.Crit("Creating data file", "error", "chunk size should not be 0")
		}
		if cfg.KvSize%cfg.ChunkSize != 0 {
			l.Crit("Creating data file", "error", "max kv size %% chunk size should be 0")
		}
		chunkPerKv := cfg.KvSize / cfg.ChunkSize
		startChunkId := shardIdx * cfg.KvEntriesPerShard * chunkPerKv
		chunkIdxLen := chunkPerKv * cfg.KvEntriesPerShard
		log.Info("Creating data file", "chunkIdxStart", startChunkId, "chunkIdxLen", chunkIdxLen, "chunkSize", cfg.ChunkSize, "miner", cfg.Miner, "encodeType", es.ENCODE_BLOB_POSEIDON)

		df, err := es.Create(fileName, startChunkId, chunkPerKv*cfg.KvEntriesPerShard, 0, cfg.KvSize, es.ENCODE_BLOB_POSEIDON, cfg.Miner, cfg.ChunkSize)
		if err != nil {
			log.Crit("Creating data file", "error", err)
		}

		l.Info("Data file created", "shard", shardIdx, "file", fileName, "datafile", fmt.Sprintf("%+v", df))
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
