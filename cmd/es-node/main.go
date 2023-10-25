// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/flags"
	eslog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/node"
	"github.com/urfave/cli"
)

var (
	GitCommit = ""
	GitDate   = ""
	Version   = "v0.1.1"
	Meta      = "dev"
)

// VersionWithMeta holds the textual version string including the metadata.
var VersionWithMeta = func() string {
	v := Version
	if GitCommit != "" {
		v += "-" + GitCommit[:8]
	}
	if GitDate != "" {
		v += "-" + GitDate
	}
	if Meta != "" {
		v += "-" + Meta
	}
	return v
}()

func main() {
	// Set up logger with a default INFO level in case we fail to parse flags,
	// otherwise the final critical log won't show what the parsing error was.
	eslog.SetupDefaults()

	app := cli.NewApp()
	app.Version = VersionWithMeta
	app.Flags = flags.Flags
	app.Name = "es-node"
	app.Usage = "EthStorage Storage Node"
	app.Description = "The EthStorage Storage Node derives L2 datahashes of the values of KV store from L1 data and reconstructs the values via L1 DA and ES P2P network."
	app.Action = EsNodeMain
	app.Commands = []cli.Command{
		{
			Name:      "init",
			Aliases:   []string{"i"},
			Usage:     `Init storage node by creating a data file for each shard. Type 'es-node init --help' for more information.`,
			UsageText: `You can specify shard_len (the number of shards) or shard_index (the index of specified shard, and you can specify more than one) to mine. If both appears, shard_index takes precedence. `,
			Flags: []cli.Flag{
				cli.Uint64Flag{
					Name:  shardLenFlagName,
					Usage: "Number of shards to mine. Will create one data file per shard.",
				},
				cli.Uint64Flag{
					Name:  encodingTypeFlagName,
					Value: ethstorage.ENCODE_BLOB_POSEIDON,
					Usage: "Encoding type of the shards. 0: no encoding, 1: keccak256, 2: ethash, 3: blob poseidon. Default: 3",
				},
				cli.Int64SliceFlag{
					Name:  shardIndexFlagName,
					Usage: "Indexes of shards to mine. Will create one data file per shard.",
				},
				flags.DataDir,
				flags.L1NodeAddr,
				flags.StorageL1Contract,
				flags.StorageMiner,
			},
			Action: EsNodeInit,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Crit("Application failed", "message", err)
	}
}

func EsNodeMain(ctx *cli.Context) error {
	log.Info("Configuring EthStorage Node")
	logCfg := eslog.ReadCLIConfig(ctx)
	if err := logCfg.Check(); err != nil {
		log.Error("Unable to create the log config", "error", err)
		return err
	}
	log := eslog.NewLogger(logCfg)
	cfg, err := NewConfig(ctx, log)
	if err != nil {
		log.Error("Unable to create the rollup node config", "error", err)
		return err
	}

	n, err := node.New(context.Background(), cfg, log, VersionWithMeta)
	if err != nil {
		log.Error("Unable to create the storage node", "error", err)
		return err
	}
	log.Info("Starting storage node", "version", VersionWithMeta)

	if err := n.Start(context.Background(), cfg); err != nil {
		log.Error("Unable to start rollup node", "error", err)
		return err
	}
	defer n.Close()

	// TODO: heartbeat
	// TODO: pprof

	log.Info("Storage node started")

	// Run simple sync test
	start := ctx.GlobalUint64(flags.TestSimpleSyncStartFlag.Name)
	end := ctx.GlobalUint64(flags.TestSimpleSyncEndFlag.Name)
	if end > start {
		log.Info("Start force sync", "start", start, "end", end)
		n.RequestL2Range(context.Background(), start, end)
	}

	interruptChannel := make(chan os.Signal, 1)
	signal.Notify(interruptChannel, []os.Signal{
		os.Interrupt,
		os.Kill,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	}...)
	<-interruptChannel

	log.Info("Storage node exited")
	return nil
}

func EsNodeInit(ctx *cli.Context) error {
	logCfg := eslog.ReadCLIConfig(ctx)
	if err := logCfg.Check(); err != nil {
		log.Error("Unable to create the log config", "error", err)
		return err
	}
	log := eslog.NewLogger(logCfg)
	log.Info("Will create data files for storage node")
	l1Rpc := readRequiredFlag(ctx, flags.L1NodeAddr.Name)
	contract := readRequiredFlag(ctx, flags.StorageL1Contract.Name)
	miner := readRequiredFlag(ctx, flags.StorageMiner.Name)
	datadir := readRequiredFlag(ctx, flags.DataDir.Name)
	encodingType := ethstorage.ENCODE_BLOB_POSEIDON
	if ctx.IsSet(encodingTypeFlagName) {
		encodingType := ctx.Uint64(encodingTypeFlagName)
		log.Info("Read flag", "name", encodingTypeFlagName, "value", encodingType)
		if encodingType > 3 {
			return fmt.Errorf("encoding_type must be an integer between 0 and 3")
		}
	}
	shardIndexes := ctx.Int64Slice(shardIndexFlagName)
	log.Info("Read flag", "name", shardIndexFlagName, "value", shardIndexes)
	shardLen := 0
	if len(shardIndexes) == 0 {
		shards := ctx.Int(shardLenFlagName)
		log.Info("Read flag", "name", shardLenFlagName, "value", shards)
		if shards == 0 {
			return fmt.Errorf("shard_len or shard_index must be specified")
		}
		shardLen = shards
	}
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
	var shardIdxList []uint64
	if len(shardIndexes) > 0 {
		// check existense of shard indexes but add shard 0 anyway
		for i := 0; i < len(shardIndexes); i++ {
			shard := uint64(shardIndexes[i])
			if shard > 0 {
				diff, err := getDifficulty(cctx, client, l1Contract, shard)
				if err != nil {
					log.Error("Failed to get shard info from contract", "error", err)
					return err
				}
				if diff != nil && diff.Cmp(big.NewInt(0)) == 0 {
					return fmt.Errorf("Shard not exist: %d", shard)
				}
			}
			shardIdxList = append(shardIdxList, shard)
		}
	} else {
		// get shard indexes of length shardLen from contract
		shardList, err := getShardList(cctx, client, l1Contract, shardLen)
		if err != nil {
			log.Error("Failed to get shard indexes from contract", "error", err)
			return err
		}
		if len(shardList) == 0 {
			return fmt.Errorf("No shard indexes found")
		}
		shardIdxList = shardList
	}
	files, err := createDataFile(storageCfg, shardIdxList, datadir, encodingType)
	if err != nil {
		log.Error("Failed to create data file", "error", err)
		return err
	}
	if len(files) > 0 {
		log.Info("Data files created", "files", strings.Join(files, ","))
	} else {
		log.Warn("No data files created")
	}
	return nil
}
