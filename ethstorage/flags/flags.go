// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package flags

import (
	"fmt"
	"time"

	eslog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
	"github.com/urfave/cli"
)

const envVarPrefix = "ES_NODE"

func prefixEnvVar(name string) string {
	return envVarPrefix + "_" + name
}

var (
	/* Required Flags */
	/* Optional Flags */
	Network = cli.StringFlag{
		Name:   "network",
		Usage:  fmt.Sprintf("Predefined L1 network selection. Available networks: %s", "devnet"),
		EnvVar: prefixEnvVar("NETWORK"),
	}
	DataDir = cli.StringFlag{
		Name:   "datadir",
		Usage:  "Data directory for the databases and keystore",
		EnvVar: prefixEnvVar("DATADIR"),
	}
	RollupConfig = cli.StringFlag{
		Name:   "rollup.config",
		Usage:  "Rollup chain parameters",
		EnvVar: prefixEnvVar("ROLLUP_CONFIG"),
	}
	// TODO: move storage flag to storage folder
	StorageFiles = cli.StringSliceFlag{
		Name:   "storage.files",
		Usage:  "Storage files parameter",
		EnvVar: prefixEnvVar("STORAGE_FILES"),
	}
	L1ChainId = cli.Uint64Flag{
		Name:   "l1.chain_id",
		Usage:  "Chain id of L1 chain endpoint to use",
		Value:  1,
		EnvVar: prefixEnvVar("L1_CHAIN_ID"),
	}
	L1NodeAddr = cli.StringFlag{
		Name:   "l1.rpc",
		Usage:  "Address of L1 User JSON-RPC endpoint to use (eth namespace required)",
		Value:  "http://127.0.0.1:8545",
		EnvVar: prefixEnvVar("L1_ETH_RPC"),
	}
	L1BeaconAddr = cli.StringFlag{
		Name:   "l1.beacon",
		Usage:  "Address of L1 beacon chain endpoint to use",
		Value:  "http://127.0.0.1:5052",
		EnvVar: prefixEnvVar("L1_BEACON_URL"),
	}
	// TODO: @Qiang everytime devnet changed, we may need to change it
	L1BeaconBasedTime = cli.Uint64Flag{
		Name:   "l1.beacon-based-time",
		Usage:  "A pair of timestamp and slot number in the past time",
		Value:  1688443932,
		EnvVar: prefixEnvVar("L1_BEACON_BASED_TIME"),
	}
	// TODO: @Qiang everytime devnet changed, we may need to change it
	L1BeaconBasedSlot = cli.Uint64Flag{
		Name:   "l1.beacon-based-slot",
		Usage:  "A pair of timestamp and slot number in the past time",
		Value:  26456,
		EnvVar: prefixEnvVar("L1_BEACON_BASED_SLOT"),
	}
	// TODO: @Qiang everytime devnet changed, we may need to change it if the slot time changed
	L1BeaconSlotTime = cli.Uint64Flag{
		Name:   "l1.beacon-slot-time",
		Usage:  "Address of L1 beacon chain endpoint to use",
		Value:  12,
		EnvVar: prefixEnvVar("L1_BEACON_SLOT_TIME"),
	}
	L1MinDurationForBlobsRequest = cli.Uint64Flag{
		Name:   "l1.min-duration-blobs-request",
		Usage:  "Min duration for blobs sidecars request",
		Value:  4096 * 32 * 12, // ~18 days, define in CL p2p spec: https://github.com/ethereum/consensus-specs/pull/3047
		EnvVar: prefixEnvVar("L1_BEACON_MIN_DURATION_BLOBS_REQUEST"),
	}
	L2ChainId = cli.Uint64Flag{
		Name:   "l2.chain_id",
		Usage:  "Chain id of L2 chain endpoint to use",
		Value:  3333,
		EnvVar: prefixEnvVar("L2_CHAIN_ID"),
	}
	MetricsEnable = cli.BoolFlag{
		Name:     "metrics.enable",
		Usage:    "Enable metrics",
		Required: false,
		EnvVar:   prefixEnvVar("METRICS_ENABLE"),
	}
	DownloadStart = cli.Int64Flag{
		Name:   "download.start",
		Usage:  "Block number which the downloader download blobs from",
		Value:  0,
		EnvVar: prefixEnvVar("DOWNLOAD_START"),
	}
	DownloadDump = cli.StringFlag{
		Name:   "download.dump",
		Usage:  "Where to dump the downloaded blobs",
		Value:  "",
		EnvVar: prefixEnvVar("DOWNLOAD_DUMP"),
	}
	StorageMiner = cli.StringFlag{
		Name:     "storage.miner",
		Usage:    "Storage miner address parameter",
		Required: false,
		Value:    "0x0000000000000000000000000000000000000000",
		EnvVar:   prefixEnvVar("STORAGE_MINER"),
	}
	StorageL1Contract = cli.StringFlag{
		Name:     "storage.l1contract",
		Usage:    "Storage contract on l1 address parameter",
		Required: false,
		Value:    "0x0000000000000000000000000000000000000000",
		EnvVar:   prefixEnvVar("STORAGE_L1CONTRACT"),
	}
	StorageKvSize = cli.Uint64Flag{
		Name:     "storage.kv-size",
		Usage:    "Storage kv size paramaeter",
		Required: false,
		Hidden:   true,
		Value:    0,
		EnvVar:   prefixEnvVar("STORAGE_KV_SIZE"),
	}
	StorageChunkSize = cli.Uint64Flag{
		Name:     "storage.chunk-size",
		Usage:    "Storage chunk (encoding) size parameter",
		Required: false,
		Hidden:   true,
		Value:    0,
		EnvVar:   prefixEnvVar("STORAGE_CHUNK_SIZE"),
	}
	StorageKvEntries = cli.Uint64Flag{
		Name:     "storage.kv-entries",
		Usage:    "Storage kv entries per shard parameter",
		Required: false,
		Hidden:   true,
		Value:    0,
		EnvVar:   prefixEnvVar("STORAGE_KV_ENTRIES"),
	}
	L1EpochPollIntervalFlag = cli.DurationFlag{
		Name:     "l1.epoch-poll-interval",
		Usage:    "Poll interval for retrieving new L1 epoch updates such as safe and finalized block changes. Disabled if 0 or negative.",
		EnvVar:   prefixEnvVar("L1_EPOCH_POLL_INTERVAL"),
		Required: false,
		Value:    time.Second * 12 * 32,
	}
	RPCListenAddr = cli.StringFlag{
		Name:     "rpc.addr",
		Usage:    "RPC listening address",
		Required: false,
		EnvVar:   prefixEnvVar("RPC_ADDR"),
		Value:    "127.0.0.1",
	}
	RPCListenPort = &cli.IntFlag{
		Name:     "rpc.port",
		Usage:    "RPC listening port",
		Required: false,
		EnvVar:   prefixEnvVar("RPC_PORT"),
		Value:    9545,
	}
	RPCESCallURL = cli.StringFlag{
		Name:     "rpc.escall-url",
		Usage:    "RPC EsCall URL",
		Required: false,
		EnvVar:   prefixEnvVar("RPC_ESCALL_URL"),
		Value:    "http://127.0.0.1:8545",
	}
)

var requiredFlags = []cli.Flag{}

var optionalFlags = []cli.Flag{
	Network,
	DataDir,
	RollupConfig,
	L1ChainId,
	L1NodeAddr,
	L1BeaconAddr,
	L1BeaconBasedTime,
	L1BeaconBasedSlot,
	L1BeaconSlotTime,
	L1MinDurationForBlobsRequest,
	L2ChainId,
	MetricsEnable,
	DownloadStart,
	DownloadDump,
	L1EpochPollIntervalFlag,
	StorageFiles,
	StorageMiner,
	StorageL1Contract,
	StorageKvSize,
	StorageChunkSize,
	StorageKvEntries,
	RPCListenAddr,
	RPCListenPort,
	RPCESCallURL,
}

// Flags contains the list of configuration options available to the binary.
var Flags []cli.Flag

func init() {
	optionalFlags = append(optionalFlags, p2pFlags...)
	optionalFlags = append(optionalFlags, eslog.CLIFlags(envVarPrefix)...)
	optionalFlags = append(optionalFlags, signer.CLIFlags(envVarPrefix)...)
	optionalFlags = append(optionalFlags, miner.CLIFlags(envVarPrefix)...)
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
