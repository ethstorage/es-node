// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package flags

import (
	"fmt"
	"time"

	"github.com/ethstorage/go-ethstorage/ethstorage/archiver"
	"github.com/ethstorage/go-ethstorage/ethstorage/email"
	"github.com/ethstorage/go-ethstorage/ethstorage/flags/utils"
	eslog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/scanner"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
	"github.com/urfave/cli"
)

var (
	//Network = cli.StringFlag{
	//	Name:   "network",
	//	Usage:  fmt.Sprintf("Predefined L1 network selection. Available networks: %s", "devnet"),
	//	EnvVar: prefixEnvVar("NETWORK"),
	//}
	DataDir = cli.StringFlag{
		Name:   "datadir",
		Usage:  "(Required) Data directory for the storage files, databases and keystore",
		EnvVar: utils.PrefixEnvVar("DATADIR"),
	}
	L1NodeAddr = cli.StringFlag{
		Name:   "l1.rpc",
		Usage:  "(Required) Address of L1 User JSON-RPC endpoint to use (eth namespace required)",
		EnvVar: utils.PrefixEnvVar("L1_ETH_RPC"),
	}
	L1BeaconAddr = cli.StringFlag{
		Name:   "l1.beacon",
		Usage:  "(Required) Address of L1 beacon chain endpoint to use",
		EnvVar: utils.PrefixEnvVar("L1_BEACON_URL"),
	}
	L1BlockTime = cli.Uint64Flag{
		Name:   "l1.block_time",
		Usage:  "Block time of L1 chain",
		Value:  12,
		EnvVar: utils.PrefixEnvVar("L1_BLOCK_TIME"),
	}
	DAURL = cli.StringFlag{
		Name:   "da.url",
		Usage:  "URL of the custom data availability service",
		EnvVar: utils.PrefixEnvVar("DA_URL"),
	}
	RandaoURL = cli.StringFlag{
		Name:   "randao.url",
		Usage:  "URL of JSON-RPC endpoint to query randao",
		EnvVar: utils.PrefixEnvVar("RANDAO_URL"),
	}
	// TODO: @Qiang everytime devnet changed, we may need to change it if the slot time changed
	L1BeaconSlotTime = cli.Uint64Flag{
		Name:   "l1.beacon-slot-time",
		Usage:  "Slot time of the L1 beacon chain",
		Value:  12,
		EnvVar: utils.PrefixEnvVar("L1_BEACON_SLOT_TIME"),
	}
	L1MinDurationForBlobsRequest = cli.Uint64Flag{
		Name:   "l1.min-duration-blobs-request",
		Usage:  "Min duration for blobs sidecars request",
		Value:  4096 * 32 * 12, // ~18 days, define in CL p2p spec: https://github.com/ethereum/consensus-specs/pull/3047
		EnvVar: utils.PrefixEnvVar("L1_BEACON_MIN_DURATION_BLOBS_REQUEST"),
	}
	ChainId = cli.Uint64Flag{
		Name:   "chain_id",
		Usage:  "Chain id of L2 chain endpoint to use",
		Value:  3333,
		EnvVar: utils.PrefixEnvVar("CHAIN_ID"),
	}
	MetricsEnabledFlag = cli.BoolFlag{
		Name:   "metrics.enabled",
		Usage:  "Enable the metrics server",
		EnvVar: utils.PrefixEnvVar("METRICS_ENABLED"),
	}
	MetricsAddrFlag = cli.StringFlag{
		Name:   "metrics.addr",
		Usage:  "Metrics listening address",
		Value:  "0.0.0.0",
		EnvVar: utils.PrefixEnvVar("METRICS_ADDR"),
	}
	MetricsPortFlag = cli.IntFlag{
		Name:   "metrics.port",
		Usage:  "Metrics listening port",
		Value:  7300,
		EnvVar: utils.PrefixEnvVar("METRICS_PORT"),
	}
	PprofEnabledFlag = cli.BoolFlag{
		Name:   "pprof.enabled",
		Usage:  "Enable the pprof server",
		EnvVar: utils.PrefixEnvVar("PPROF_ENABLED"),
	}
	PprofAddrFlag = cli.StringFlag{
		Name:   "pprof.addr",
		Usage:  "pprof listening address",
		Value:  "0.0.0.0",
		EnvVar: utils.PrefixEnvVar("PPROF_ADDR"),
	}
	PprofPortFlag = cli.IntFlag{
		Name:   "pprof.port",
		Usage:  "pprof listening port",
		Value:  6060,
		EnvVar: utils.PrefixEnvVar("PPROF_PORT"),
	}
	DownloadStart = cli.Int64Flag{
		Name:   "download.start",
		Usage:  "Block number which the downloader download blobs from",
		Value:  0,
		EnvVar: utils.PrefixEnvVar("DOWNLOAD_START"),
	}
	DownloadThreadNum = cli.IntFlag{
		Name:   "download.thread",
		Usage:  "Threads number that will be used to download the blobs",
		Value:  1,
		EnvVar: utils.PrefixEnvVar("DOWNLOAD_THREAD"),
	}
	DownloadDump = cli.StringFlag{
		Name:   "download.dump",
		Usage:  "Where to dump the downloaded blobs",
		Value:  "",
		EnvVar: utils.PrefixEnvVar("DOWNLOAD_DUMP"),
	}
	// TODO: move storage flag to storage folder
	StorageFiles = cli.StringSliceFlag{
		Name:   "storage.files",
		Usage:  "(Required) File paths where the data are stored",
		EnvVar: utils.PrefixEnvVar("STORAGE_FILES"),
	}
	StorageMiner = cli.StringFlag{
		Name:   "storage.miner",
		Usage:  "Miner's address to encode data and receive mining rewards",
		EnvVar: utils.PrefixEnvVar("STORAGE_MINER"),
	}
	StorageL1Contract = cli.StringFlag{
		Name:   "storage.l1contract",
		Usage:  "(Required) Storage contract address on l1",
		EnvVar: utils.PrefixEnvVar("STORAGE_L1CONTRACT"),
	}
	StorageKvSize = cli.Uint64Flag{
		Name:   "storage.kv-size",
		Usage:  "Storage kv size parameter",
		Hidden: true,
		Value:  0,
		EnvVar: utils.PrefixEnvVar("STORAGE_KV_SIZE"),
	}
	StorageChunkSize = cli.Uint64Flag{
		Name:   "storage.chunk-size",
		Usage:  "Storage chunk (encoding) size parameter",
		Hidden: true,
		Value:  0,
		EnvVar: utils.PrefixEnvVar("STORAGE_CHUNK_SIZE"),
	}
	StorageKvEntries = cli.Uint64Flag{
		Name:   "storage.kv-entries",
		Usage:  "Storage kv entries per shard parameter",
		Hidden: true,
		Value:  0,
		EnvVar: utils.PrefixEnvVar("STORAGE_KV_ENTRIES"),
	}
	L1EpochPollIntervalFlag = cli.DurationFlag{
		Name:   "l1.epoch-poll-interval",
		Usage:  "Poll interval for retrieving new L1 epoch updates such as safe and finalized block changes. Disabled if 0 or negative.",
		EnvVar: utils.PrefixEnvVar("L1_EPOCH_POLL_INTERVAL"),
		Value:  time.Second * 12 * 32,
	}
	RPCListenAddr = cli.StringFlag{
		Name:   "rpc.addr",
		Usage:  "RPC listening address",
		EnvVar: utils.PrefixEnvVar("RPC_ADDR"),
		Value:  "127.0.0.1",
	}
	RPCListenPort = cli.IntFlag{
		Name:   "rpc.port",
		Usage:  "RPC listening port",
		EnvVar: utils.PrefixEnvVar("RPC_PORT"),
		Value:  9545,
	}
	RPCESCallURL = cli.StringFlag{
		Name:   "rpc.escall-url",
		Usage:  "RPC EsCall URL",
		EnvVar: utils.PrefixEnvVar("RPC_ESCALL_URL"),
		Value:  "http://127.0.0.1:8545",
	}
	StateUploadURL = cli.StringFlag{
		Name:   "state.upload.url",
		Usage:  "API that update es-node state to, the node will upload state to API for statistic if it has been set correctly.",
		EnvVar: utils.PrefixEnvVar("STATE_UPLOAD_URL"),
	}
)

// Not use 'Required' field in order to avoid unnecessary check when use 'init' subcommand
// Instead follow optimism's design to use `CheckRequired()`
var requiredFlags = []cli.Flag{
	DataDir,
	StorageFiles,
	L1NodeAddr,
	StorageL1Contract,
}

var optionalFlags = []cli.Flag{
	StorageMiner,
	//	Network,
	L1BlockTime,
	L1BeaconSlotTime,
	L1BeaconAddr,
	DAURL,
	RandaoURL,
	L1MinDurationForBlobsRequest,
	ChainId,
	MetricsEnabledFlag,
	MetricsAddrFlag,
	MetricsPortFlag,
	PprofEnabledFlag,
	PprofAddrFlag,
	PprofPortFlag,
	DownloadStart,
	DownloadThreadNum,
	DownloadDump,
	L1EpochPollIntervalFlag,
	StorageKvSize,
	StorageChunkSize,
	StorageKvEntries,
	RPCListenAddr,
	RPCListenPort,
	RPCESCallURL,
	StateUploadURL,
}

// Flags contains the list of configuration options available to the binary.
var Flags []cli.Flag

func init() {
	optionalFlags = append(optionalFlags, p2pFlags...)
	optionalFlags = append(optionalFlags, eslog.CLIFlags()...)
	optionalFlags = append(optionalFlags, signer.CLIFlags()...)
	optionalFlags = append(optionalFlags, miner.CLIFlags()...)
	optionalFlags = append(optionalFlags, archiver.CLIFlags()...)
	optionalFlags = append(optionalFlags, scanner.CLIFlags()...)
	optionalFlags = append(optionalFlags, email.CLIFlags()...)
	Flags = append(requiredFlags, optionalFlags...)
}

func CheckRequired(ctx *cli.Context) error {
	for _, f := range requiredFlags {
		if !ctx.GlobalIsSet(f.GetName()) {
			return fmt.Errorf("flag %s is required", f.GetName())
		}
	}
	return nil
}
