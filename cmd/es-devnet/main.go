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

var (
	log = esLog.NewLogger(esLog.DefaultCLIConfig())
)

var (
	l1Rpc       string
	contract    string
	miner       string
	datadir     string
	shardLength int
)

var kvIdx uint64

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

func InitFiles(storageCfg *storage.StorageConfig) ([]string, error) {
	shardIdxList := make([]uint64, shardLength)
	return createDataFile(storageCfg, shardIdxList, datadir)
}

func GenerateData(files []string, storageCfg *storage.StorageConfig) error {
	for shardIdx, file := range files {
		ds := initDataShard(uint64(shardIdx), file, storageCfg)

		// generate data
		fileSize := uint64(5 * 4096 * 31)
		data := make([]byte, fileSize)
		for j := uint64(0); j < fileSize; j += 31 {
			scalar := genRandomCanonicalScalar()
			copy(data[j:(j+31)], scalar[:31])
		}

		// generate blob
		var hashes []common.Hash
		blobs := utils.EncodeBlobs(data)
		for _, blob := range blobs {
			hash := writeBlob(kvIdx, blob, ds)
			hashes = append(hashes, hash)
			kvIdx += 1
		}
		log.Info("", "hash", hashes)
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

	l1Contract := common.HexToAddress(contract)
	storageCfg, err := initStorageConfig(cctx, client, l1Contract, common.HexToAddress(miner))
	if err != nil {
		log.Error("Failed to load storage config", "error", err)
		return err
	}
	log.Info("Storage config loaded", "storageCfg", storageCfg)

	// create files
	files, err := InitFiles(storageCfg)
	if err != nil {
		log.Error("Failed to create data file", "error", err)
		return err
	}

	// create contract

	// generate data
	GenerateData(files, storageCfg)
	return nil
}
