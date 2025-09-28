// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	oppprof "github.com/ethereum-optimism/optimism/op-service/pprof"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/email"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/flags"
	"github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
	"github.com/ethstorage/go-ethstorage/ethstorage/node"

	"github.com/urfave/cli"
)

var (
	GitCommit     = ""
	GitDate       = ""
	Version       = "v0.2.6"
	Meta          = "dev"
	BuildTime     = ""
	systemVersion = fmt.Sprintf("%s/%s", runtime.GOARCH, runtime.GOOS)
	golangVersion = runtime.Version()
	lg            = log.DefaultLogger()
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

var BuildInfo = func() string {
	return fmt.Sprintf(
		"%s\nBuild date: %s\nSystem version: %s\nGolang version: %s",
		VersionWithMeta, BuildTime, systemVersion, golangVersion)
}()

func main() {
	log.SetupDefaults()
	app := cli.NewApp()
	app.Version = BuildInfo
	app.Flags = flags.Flags
	app.Name = "EthStorage node"
	app.Usage = "EthStorage Storage Node"
	app.Description = "The EthStorage Storage Node derives L2 datahashes of the values of KV store from L1 data and reconstructs the values via L1 DA and ES P2P network."
	app.Action = EsNodeMain
	app.Commands = []cli.Command{
		{
			Name:      "init",
			Aliases:   []string{"i"},
			Usage:     `Initialize shard data files (one per shard). Type 'es-node init --help' for more information.`,
			UsageText: `Use --shard_len to create N sequential shards starting at 0, or --shard_index to list one or more explicit shard indices (repeatable). If both are supplied, --shard_index wins.`,
			Flags: []cli.Flag{
				cli.Uint64Flag{
					Name:  shardLenFlagName,
					Usage: "Number of shards to mine. Will create one data file per shard.",
				},
				cli.IntFlag{
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
		{
			Name:      "sync",
			Aliases:   []string{"s"},
			Usage:     "Fetch a single blob (by KV index) from a remote EthStorage RPC and write it into local storage. Type 'es-node sync --help' for more information.",
			UsageText: fmt.Sprintf("Requires --%s (KV index) and --%s (RPC endpoint).", kvIndexFlagName, esRpcFlagName),
			Flags: []cli.Flag{
				cli.Uint64Flag{
					Name:  kvIndexFlagName,
					Usage: "KV index to update",
				},
				cli.StringFlag{
					Name:  esRpcFlagName,
					Usage: "EthStorage RPC endpoint",
				},
				flags.DataDir,
				flags.L1NodeAddr,
				flags.StorageL1Contract,
				flags.StorageMiner,
				flags.StorageFiles,
			},
			Action: EsNodeSync,
		},
		{
			Name:    "email",
			Aliases: []string{"m"},
			Usage:   "Send a test email using the configured SMTP settings.",
			Flags:   email.CLIFlags(),
			Action: func(ctx *cli.Context) error {
				emailConfig, err := email.GetEmailConfig(ctx)
				if err != nil {
					return err
				}
				lg.Info("Email configuration", "username", emailConfig.Username)
				lg.Info("Email configuration", "host", emailConfig.Host)
				lg.Info("Email configuration", "port", strconv.FormatUint(emailConfig.Port, 10))
				lg.Info("Email configuration", "to", emailConfig.To)
				lg.Info("Email configuration", "from", emailConfig.From)

				subject := "Test Email from EthStorage"
				body := "This is a test email sent from the EthStorage node CLI."
				email.SendEmail(subject, body, *emailConfig, lg)
				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		lg.Crit("Application failed", "message", err)
	}
}

func EsNodeMain(ctx *cli.Context) error {
	lg.Info("Configuring EthStorage Node")
	lgCfg := log.ReadCLIConfig(ctx)
	lg.Info("Loading log config", "config", lgCfg)
	clog := log.NewLogger(lgCfg)
	cfg, err := NewConfig(ctx, clog)
	if err != nil {
		lg.Error("Unable to create the rollup node config", "error", err)
		return err
	}

	var m metrics.Metricer = metrics.NoopMetrics
	if cfg.Metrics.Enabled {
		m = metrics.NewMetrics("default")
	}
	n, err := node.New(context.Background(), cfg, clog, VersionWithMeta, m)
	if err != nil {
		clog.Error("Unable to create the storage node", "error", err)
		return err
	}
	clog.Info("Starting storage node", "version", VersionWithMeta)

	if err := n.Start(context.Background(), cfg); err != nil {
		clog.Error("Unable to start rollup node", "error", err)
		return err
	}
	defer n.Close()

	m.RecordInfo(VersionWithMeta)
	m.RecordUp()
	// TODO: heartbeat
	if cfg.Pprof.Enabled {
		pprofCtx, pprofCancel := context.WithCancel(context.Background())
		go func() {
			clog.Info("pprof server started", "addr", net.JoinHostPort(cfg.Pprof.ListenAddr, strconv.Itoa(cfg.Pprof.ListenPort)))
			if err := oppprof.ListenAndServe(pprofCtx, cfg.Pprof.ListenAddr, cfg.Pprof.ListenPort); err != nil {
				clog.Error("error starting pprof", "err", err)
			}
		}()
		defer pprofCancel()
	}

	clog.Info("Storage node started")

	// Run simple sync test
	start := ctx.GlobalUint64(flags.TestSimpleSyncStartFlag.Name)
	end := ctx.GlobalUint64(flags.TestSimpleSyncEndFlag.Name)
	if end > start {
		lg.Info("Start force sync", "start", start, "end", end)
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

	clog.Info("Storage node exited")
	return nil
}

func EsNodeInit(ctx *cli.Context) error {
	lg.Info("Will create data files for storage node")
	l1Rpc := readRequiredFlag(ctx, flags.L1NodeAddr)
	contract := readRequiredFlag(ctx, flags.StorageL1Contract)
	if !common.IsHexAddress(contract) {
		return fmt.Errorf("invalid contract address %s", contract)
	}

	datadir := readRequiredFlag(ctx, flags.DataDir)
	encodingType := ethstorage.ENCODE_BLOB_POSEIDON
	miner := "0x"
	if ctx.IsSet(encodingTypeFlagName) {
		encodingType = ctx.Int(encodingTypeFlagName)
		lg.Info("Read flag", "name", encodingTypeFlagName, "value", encodingType)
		if encodingType > 3 || encodingType < 0 {
			return fmt.Errorf("encoding_type must be an integer between 0 and 3")
		}
	}
	if encodingType != ethstorage.NO_ENCODE {
		miner = readRequiredFlag(ctx, flags.StorageMiner)
		if !common.IsHexAddress(miner) {
			return fmt.Errorf("invalid miner address %s", miner)
		}
	}
	shardIndexes := ctx.Int64Slice(shardIndexFlagName)
	lg.Info("Read flag", "name", shardIndexFlagName, "value", shardIndexes)
	shardLen := 0
	if len(shardIndexes) == 0 {
		shards := ctx.Int(shardLenFlagName)
		lg.Info("Read flag", "name", shardLenFlagName, "value", shards)
		if shards == 0 {
			return fmt.Errorf("shard_len or shard_index must be specified")
		}
		shardLen = shards
	}
	cctx := context.Background()
	client, err := ethclient.DialContext(cctx, l1Rpc)
	if err != nil {
		lg.Error("Failed to connect to the Ethereum client", "error", err, "l1Rpc", l1Rpc)
		return err
	}
	defer client.Close()

	l1Contract := common.HexToAddress(contract)
	storageCfg, err := initStorageConfig(cctx, client, l1Contract, common.HexToAddress(miner), lg)
	if err != nil {
		lg.Error("Failed to load storage config", "error", err)
		return err
	}
	lg.Info("Storage config loaded", "storageCfg", storageCfg)
	var shardIdxList []uint64
	if len(shardIndexes) > 0 {
	out:
		for i := 0; i < len(shardIndexes); i++ {
			new := uint64(shardIndexes[i])
			// prevent duplicated
			for _, s := range shardIdxList {
				if s == new {
					continue out
				}
			}
			shardIdxList = append(shardIdxList, new)
		}
	} else {
		// get shard indexes of length shardLen from contract
		shardList, err := getShardList(cctx, client, l1Contract, shardLen)
		if err != nil {
			lg.Error("Failed to get shard indexes from contract", "error", err)
			return err
		}
		if len(shardList) == 0 {
			return fmt.Errorf("no shard indexes found")
		}
		shardIdxList = shardList
	}
	files, err := createDataFile(storageCfg, shardIdxList, datadir, encodingType)
	if err != nil {
		lg.Error("Failed to create data file", "error", err)
		return err
	}
	if len(files) > 0 {
		lg.Info("Data files created", "files", strings.Join(files, ","))
	} else {
		lg.Warn("No data files created")
	}
	return nil
}

func EsNodeSync(ctx *cli.Context) error {
	lg.Info("Sync data for specified kv")
	if !ctx.IsSet(kvIndexFlagName) {
		return fmt.Errorf("kv_index must be specified")
	}
	kvIndex := uint64(ctx.Int(kvIndexFlagName))
	lg.Info("Read flag", "name", kvIndexFlagName, "value", kvIndex)
	if !ctx.IsSet(esRpcFlagName) {
		return fmt.Errorf("es_rpc must be specified")
	}
	esRpc := ctx.String(esRpcFlagName)
	lg.Info("Read flag", "name", esRpcFlagName, "value", esRpc)
	// query meta
	contract := readRequiredFlag(ctx, flags.StorageL1Contract)
	if !common.IsHexAddress(contract) {
		return fmt.Errorf("invalid contract address %s", contract)
	}
	l1contract := common.HexToAddress(contract)
	l1Rpc := readRequiredFlag(ctx, flags.L1NodeAddr)
	pClient, err := eth.Dial(l1Rpc, l1contract, 2, lg)
	if err != nil {
		return fmt.Errorf("failed to dial eth client: %w", err)
	}
	meta, err := pClient.GetKvMetas([]uint64{kvIndex}, rpc.LatestBlockNumber.Int64())
	if err != nil {
		return fmt.Errorf("failed to get meta: %w", err)
	}
	lg.Info("Query meta from contract done", "kvIndex", kvIndex, "meta", common.Hash(meta[0]).Hex())
	// query blob
	var commit common.Hash
	copy(commit[:], meta[0][32-ethstorage.HashSizeInContract:32])
	blob, err := downloadBlobFromRPC(esRpc, kvIndex, commit)
	if err != nil {
		return fmt.Errorf("failed to download blob from RPC: %w", err)
	}
	lg.Info("Download blob from RPC done", "kvIndex", kvIndex, "commit", commit.Hex())
	// write blob and meta
	shardManager, err := initShardManager(ctx, l1Rpc, l1contract)
	if err != nil {
		return fmt.Errorf("failed to init shard manager: %w", err)
	}
	preparedCommit := ethstorage.PrepareCommit(commit)
	ok, err := shardManager.TryWrite(kvIndex, blob, preparedCommit)
	if err != nil {
		return fmt.Errorf("failed to write kv: %w", err)
	}
	if !ok {
		return fmt.Errorf("failed to write kv: kv index not in the shard")
	}
	lg.Info("Sync data finished", "kvIndex", kvIndex, "commit", commit.Hex())
	return nil
}
