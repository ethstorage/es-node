// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"context"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
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
	l1Rpc       string
	contract    string
	privateKey  string
	miner       string
	datadir     string
	shardLength int
	chainId     int

	kvIdx     uint64
	firstBlob = true
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
	//fileSize := uint64(5 * 4096 * 31)
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

func generateDataAndWrite(cli *ethclient.Client, files []string, storageCfg *storage.StorageConfig) error {

	for shardIdx, file := range files {
		ds := initDataShard(uint64(shardIdx), file, storageCfg)

		// generate blob and write
		tranLen := [...]uint64{580, 580, 580, 192} // 2664+2664+2664+192=8192 blobs
		for _, length := range tranLen {
			var hashes []common.Hash
			// generate data
			var data []byte
			if length == 200 {
				data = make([]byte, length*4096*31)
			} else {
				data = randomData(length * 4096 * 31)
			}
			blobs := utils.EncodeBlobs(data)

			// write
			for _, blob := range blobs {
				versionedHash := writeBlob(kvIdx, blob, ds)
				hash := common.Hash{}
				copy(hash[0:], versionedHash[0:HashSizeInContract])
				hashes = append(hashes, hash)
				kvIdx += 1
			}

			// update to contract
			err := UploadHashes(cli, hashes)
			if err != nil {
				return err
			}
			log.Info("Upload Success", "hashes", hashes)
		}
		log.Info("Write File Success \n")
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

	// create files
	files, err := initFiles(storageCfg)
	if err != nil {
		log.Error("Failed to create data file", "error", err)
		return err
	} else {
		log.Info("File create success \n")
	}

	// generate data
	return generateDataAndWrite(client, files, storageCfg)
}
