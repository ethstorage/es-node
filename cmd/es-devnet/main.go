// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/ethclient"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
	"github.com/urfave/cli"
)

const (
	HashSizeInContract = 24
)

var (
	log = esLog.NewLogger(esLog.DefaultCLIConfig())
)

var (
	l1Rpc        string
	contract     string
	privateKey   string
	miner        string
	datadir      string
	generateData string
	shardLength  int
	chainId      int

	fromAddress common.Address
	firstBlob   = true
)

var flags = []cli.Flag{
	cli.StringFlag{
		Name:        "l1.rpc",
		Usage:       "Address of L1 User JSON-RPC endpoint to use (eth namespace required)",
		Destination: &l1Rpc,
	},
	cli.StringFlag{
		Name:        "storage.l1contract",
		Usage:       "Storage contract address on l1",
		Destination: &contract,
	},
	cli.IntFlag{
		Name:        "l1.chainId",
		Usage:       "L1 network chain id",
		Destination: &chainId,
	},
	cli.StringFlag{
		Name:        "storage.privateKey",
		Usage:       "Storage private key",
		Destination: &privateKey,
	},
	cli.StringFlag{
		Name:        "storage.miner",
		Usage:       "Miner's address to encode data and receive mining rewards",
		Destination: &miner,
	},
	cli.StringFlag{
		Name:        "datadir",
		Value:       "./es-data",
		Usage:       "Data directory for the storage files, databases and keystore",
		Destination: &datadir,
	},
	cli.IntFlag{
		Name:        "shardLength",
		Value:       1,
		Usage:       "File counts",
		Destination: &shardLength,
	},
	cli.StringFlag{
		Name:        "generateData",
		Usage:       "need to Generate Data",
		Destination: &generateData,
	},
}

type HashInfo struct {
	index int
	hash  common.Hash
}

func main() {
	app := cli.NewApp()
	app.Version = "1.0.0"
	app.Name = "es-devnet"
	app.Usage = "Create EthStorage Test Data"
	app.Flags = flags
	app.Action = GenerateTestData

	// start
	err := app.Run(os.Args)
	if err != nil {
		log.Crit("Application failed", "message", err)
		return
	}
}

func initFiles(storageCfg *storage.StorageConfig) ([]string, error) {
	shardIdxList := make([]uint64, shardLength)
	return createDataFile(storageCfg, shardIdxList, datadir)
}

func randomData(dataSize uint64) []byte {
	data := make([]byte, dataSize)
	for j := uint64(0); j < dataSize; j += 32 {
		scalar := genRandomCanonicalScalar()
		max := j + 32
		if max > dataSize {
			max = dataSize
		}
		copy(data[j:max], scalar[:max-j])
	}
	return data
}

func generateDataAndWrite(files []string, storageCfg *storage.StorageConfig) error {
	log.Info("Start write files...")

	hashFile, err := createHashFile()
	if err != nil {
		log.Error("Create hash file failed", "error", err)
		return err
	}
	defer hashFile.Close()

	ds := initDataShard(0, files[0], storageCfg)
	// generate data
	data := randomData(4096 * 31)
	blob := utils.EncodeBlobs(data)[0]
	commit, err := kzg4844.BlobToCommitment(blob)
	if err != nil {
		log.Error("Compute commit failed", "error", err)
		return err
	}
	// data hash
	versionHash := sha256.Sum256(commit[:])
	versionHash[0] = blobCommitmentVersionKZG

	startTime := time.Now()
	// set blob size, set 192 empty blob
	//maxBlobSize := 1023*8192 + 8000
	maxBlobSize := 3200
	numGoroutines := 32
	goroutineBlobLength := maxBlobSize / numGoroutines

	// write data
	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			kvIdx := goroutineBlobLength * index
			for j := 0; j < goroutineBlobLength; j++ {
				err = ds.Write(uint64(kvIdx), blob[:], versionHash)
				if err != nil {
					log.Crit("Write failed", "error", err)
				}
				log.Info("Write file", "kvIdx", kvIdx)

				kvIdx += 1
			}
		}(i)
	}
	wg.Wait()
	log.Info("Write file time", "time", time.Now().Sub(startTime))

	// save hash
	writer := bufio.NewWriter(hashFile)
	content := hex.EncodeToString(versionHash[:])
	_, err = writer.WriteString(strconv.Itoa(maxBlobSize) + ":" + content)
	if err != nil {
		log.Crit("Write hash failed", "error", err)
	}
	err = writer.Flush()
	if err != nil {
		log.Error("Save file failed", "error", err)
		return err
	}

	// write 192 empty blob
	startTime = time.Now()
	blob = kzg4844.Blob{}
	versionHash = common.Hash{}
	kvIdx := maxBlobSize
	for j := 0; j < 4992; j++ {
		err = ds.Write(uint64(kvIdx), blob[:], versionHash)
		if err != nil {
			return err
		}
		kvIdx += 1
	}
	log.Info("Write empty file time", "time", time.Now().Sub(startTime))
	log.Info("Write Empty File Success \n")
	return nil
}

func uploadBlobHashes(cli *ethclient.Client, hashes []common.Hash) error {
	// Submitting 580 blob hashes costs 30 million gas
	submitCount := 580
	for i, length := 0, len(hashes); i < length; i += submitCount {
		max := i + submitCount
		if max > length {
			max = length
		}
		submitHashes := hashes[i:max]
		log.Info("Transaction submitted start", "from", i, "to", max)
		// update to contract
		err := UploadHashes(cli, submitHashes)
		if err != nil {
			return err
		}
		log.Info("Upload Success \n")
	}
	return nil
}

func GenerateTestData(ctx *cli.Context) error {
	// init
	cctx := context.Background()
	client, err := ethclient.DialContext(cctx, l1Rpc)
	if err != nil {
		log.Error("Failed to connect to the Ethereum client", "error", err, "l1Rpc", l1Rpc)
		return err
	}
	defer client.Close()
	// init config
	l1Contract := common.HexToAddress(contract)
	storageCfg, err := initStorageConfig(cctx, client, l1Contract, common.HexToAddress(miner))
	if err != nil {
		log.Error("Failed to load storage config", "error", err)
		return err
	}
	log.Info("Storage config loaded", "storageCfg", storageCfg)
	// generate from address
	key, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		log.Error("Invalid private key", "err", err)
		return err
	}
	fromAddress = crypto.PubkeyToAddress(key.PublicKey)

	// create files
	if generateData == "true" {
		files, err := initFiles(storageCfg)
		if err != nil {
			log.Error("Failed to create data file", "error", err)
			return err
		} else {
			log.Info("File Create Success \n")
		}

		// generate data
		err = generateDataAndWrite(files, storageCfg)
		if err != nil {
			log.Error("Write file failed", "error", err)
			return err
		}
	}

	// upload
	hashes := readHashFile()
	return uploadBlobHashes(client, hashes)
}
