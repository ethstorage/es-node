// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package node

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"path/filepath"
	"time"

	oppprof "github.com/ethereum-optimism/optimism/op-service/pprof"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/archiver"
	"github.com/ethstorage/go-ethstorage/ethstorage/db"
	"github.com/ethstorage/go-ethstorage/ethstorage/downloader"
	"github.com/ethstorage/go-ethstorage/ethstorage/email"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/flags"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p"
	p2pcli "github.com/ethstorage/go-ethstorage/ethstorage/p2p/cli"
	"github.com/ethstorage/go-ethstorage/ethstorage/scanner"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
	"github.com/urfave/cli"
)

type Config struct {
	L1         eth.L1EndpointConfig
	Downloader downloader.Config
	// RPC used to query randao from a layer 1 blockchain when the storage contract is deployed on a layer 2
	RandaoSourceURL string
	// L2     L2EndpointSetup
	// L2Sync L2SyncEndpointSetup

	DataDir        string
	StateUploadURL string
	DBConfig       *db.Config
	ChainID        *big.Int
	// Driver driver.Config

	// // P2PSigner will be used for signing off on published content
	// // if the node is sequencing and if the p2p stack is enabled
	// P2PSigner p2p.SignerSetup

	RPC RPCConfig

	P2P p2p.SetupP2P

	Storage storage.StorageConfig

	Metrics MetricsConfig

	Pprof oppprof.CLIConfig

	// Used to poll the L1 for new finalized or safe blocks
	L1EpochPollInterval time.Duration

	// // Optional
	// Tracer    Tracer
	// Heartbeat HeartbeatConfig
	Mining *miner.Config

	Archiver *archiver.Config

	Scanner *scanner.Config
}

type MetricsConfig struct {
	Enabled    bool
	ListenAddr string
	ListenPort int
}

func (m MetricsConfig) Check() error {
	if !m.Enabled {
		return nil
	}

	if m.ListenPort < 0 || m.ListenPort > math.MaxUint16 {
		return errors.New("invalid metrics port")
	}

	return nil
}

type RPCConfig struct {
	ListenAddr string
	ListenPort int
	ESCallURL  string
}

// Check verifies that the given configuration makes sense
func (cfg *Config) Check() error {
	// if err := cfg.L2.Check(); err != nil {
	// 	return fmt.Errorf("l2 endpoint config error: %w", err)
	// }
	// if err := cfg.L2Sync.Check(); err != nil {
	// 	return fmt.Errorf("sync config error: %w", err)
	// }
	// if err := cfg.Rollup.Check(); err != nil {
	// 	return fmt.Errorf("rollup config error: %w", err)
	// }
	if err := cfg.Metrics.Check(); err != nil {
		return fmt.Errorf("metrics config error: %w", err)
	}
	if err := cfg.Pprof.Check(); err != nil {
		return fmt.Errorf("pprof config error: %w", err)
	}
	if cfg.P2P != nil {
		if err := cfg.P2P.Check(); err != nil {
			return fmt.Errorf("p2p config error: %w", err)
		}
	}
	return nil
}

// ResolvePath resolves path in the instance directory.
func (c *Config) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if c.DataDir == "" {
		return ""
	}
	return filepath.Join(c.DataDir, path)
}

func (c *Config) ResolveAncient(name string, ancient string) string {
	switch {
	case ancient == "":
		ancient = filepath.Join(c.ResolvePath(name), "ancient")
	case !filepath.IsAbs(ancient):
		ancient = c.ResolvePath(ancient)
	}
	return ancient
}

// NewConfig creates a Config from the provided flags or environment variables.
func NewConfig(ctx *cli.Context, lg log.Logger) (*Config, error) {
	if err := flags.CheckRequired(ctx); err != nil {
		return nil, err
	}

	datadir := ctx.GlobalString(flags.DataDir.Name)

	// TODO: blocktime is set to zero, need to update the value
	p2pConfig, err := p2pcli.NewConfig(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to load p2p config: %w", err)
	}

	l1Endpoint, client, err := NewL1EndpointConfig(ctx, lg)
	if err != nil {
		return nil, err
	}
	lg.Info("Read L1 config", flags.L1NodeAddr.Name, l1Endpoint.L1NodeAddr)
	lg.Info("Read L1 config", flags.L1BeaconAddr.Name, l1Endpoint.L1BeaconURL)
	defer client.Close()

	l1Contract := common.HexToAddress(ctx.GlobalString(flags.StorageL1Contract.Name))
	pClient := eth.NewClient(context.Background(), client, true, l1Contract, 0, nil, lg)
	storageConfig, err := NewStorageConfig(ctx, pClient, lg)
	if err != nil {
		return nil, fmt.Errorf("failed to load storage config: %w", err)
	}

	dlConfig := NewDownloaderConfig(ctx)
	minerConfig, err := NewMinerConfig(ctx, pClient, storageConfig.Miner, lg)
	if err != nil {
		return nil, fmt.Errorf("failed to load miner config: %w", err)
	}
	chainId := new(big.Int).SetUint64(ctx.GlobalUint64(flags.ChainId.Name))
	lg.Info("Read chain ID of EthStorage network", "chainID", chainId)
	if minerConfig != nil {
		minerConfig.ChainID = chainId
	}
	archiverConfig := archiver.NewConfig(ctx)
	// l2Endpoint, err := NewL2EndpointConfig(ctx, lg)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to load l2 endpoints info: %w", err)
	// }

	// l2SyncEndpoint := NewL2SyncEndpointConfig(ctx)
	cfg := &Config{
		L1:         *l1Endpoint,
		ChainID:    chainId,
		Downloader: *dlConfig,

		DataDir:        datadir,
		StateUploadURL: ctx.GlobalString(flags.StateUploadURL.Name),
		DBConfig:       db.DefaultDBConfig(),
		// rpc url to get randao from
		RandaoSourceURL: ctx.GlobalString(flags.RandaoURL.Name),
		// 	Driver: *driverConfig,
		RPC: RPCConfig{
			ListenAddr: ctx.GlobalString(flags.RPCListenAddr.Name),
			ListenPort: ctx.GlobalInt(flags.RPCListenPort.Name),
			ESCallURL:  ctx.GlobalString(flags.RPCESCallURL.Name),
		},
		Metrics: MetricsConfig{
			Enabled:    ctx.GlobalBool(flags.MetricsEnabledFlag.Name),
			ListenAddr: ctx.GlobalString(flags.MetricsAddrFlag.Name),
			ListenPort: ctx.GlobalInt(flags.MetricsPortFlag.Name),
		},
		Pprof: oppprof.CLIConfig{
			Enabled:    ctx.GlobalBool(flags.PprofEnabledFlag.Name),
			ListenAddr: ctx.GlobalString(flags.PprofAddrFlag.Name),
			ListenPort: ctx.GlobalInt(flags.PprofPortFlag.Name),
		},
		P2P: p2pConfig,

		L1EpochPollInterval: ctx.GlobalDuration(flags.L1EpochPollIntervalFlag.Name),
		// 	Heartbeat: node.HeartbeatConfig{
		// 		Enabled: ctx.GlobalBool(flags.HeartbeatEnabledFlag.Name),
		// 		Moniker: ctx.GlobalString(flags.HeartbeatMonikerFlag.Name),
		// 		URL:     ctx.GlobalString(flags.HeartbeatURLFlag.Name),
		// 	},
		Storage:  *storageConfig,
		Mining:   minerConfig,
		Archiver: archiverConfig,
		Scanner:  scanner.NewConfig(ctx),
	}
	if err := cfg.Check(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func NewMinerConfig(ctx *cli.Context, pClient *eth.PollingClient, minerAddr common.Address, lg log.Logger) (*miner.Config, error) {
	cliConfig := miner.ReadCLIConfig(ctx)
	if !cliConfig.Enabled {
		lg.Info("Miner is not enabled.")
		return nil, nil
	}
	if minerAddr == (common.Address{}) {
		return nil, fmt.Errorf("miner address cannot be empty")
	}
	lg.Debug("Read mining config from cli", "config", fmt.Sprintf("%+v", cliConfig))
	err := cliConfig.Check()
	if err != nil {
		return nil, fmt.Errorf("invalid miner flags: %w", err)
	}
	minerConfig, err := cliConfig.ToMinerConfig()
	if err != nil {
		return nil, err
	}
	if minerConfig.EmailEnabled {
		emailConfig, err := email.GetEmailConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get email config: %w", err)
		}
		minerConfig.EmailConfig = *emailConfig
	}

	randomChecks, err := pClient.ReadContractUint64Field("randomChecks", nil)
	if err != nil {
		return nil, err
	}
	minerConfig.RandomChecks = randomChecks
	nonceLimit, err := pClient.ReadContractUint64Field("nonceLimit", nil)
	if err != nil {
		return nil, err
	}
	minerConfig.NonceLimit = nonceLimit
	minimumDiff, err := pClient.ReadContractBigIntField("minimumDiff", nil)
	if err != nil {
		return nil, err
	}
	minerConfig.MinimumDiff = minimumDiff
	cutoff, err := pClient.ReadContractBigIntField("cutoff", nil)
	if err != nil {
		return nil, err
	}
	minerConfig.Cutoff = cutoff
	diffAdjDivisor, err := pClient.ReadContractBigIntField("diffAdjDivisor", nil)
	if err != nil {
		return nil, err
	}
	minerConfig.DiffAdjDivisor = diffAdjDivisor
	dcf, err := pClient.ReadContractBigIntField("dcfFactor", nil)
	if err != nil {
		return nil, err
	}
	minerConfig.DcfFactor = dcf

	startTime, err := pClient.ReadContractUint64Field("startTime", nil)
	if err != nil {
		return nil, err
	}
	minerConfig.StartTime = startTime
	shardEntryBits, err := pClient.ReadContractUint64Field("shardEntryBits", nil)
	if err != nil {
		return nil, err
	}
	minerConfig.ShardEntry = 1 << shardEntryBits
	treasuryShare, err := pClient.ReadContractUint64Field("treasuryShare", nil)
	if err != nil {
		return nil, err
	}
	minerConfig.TreasuryShare = treasuryShare
	storageCost, err := pClient.ReadContractBigIntField("storageCost", nil)
	if err != nil {
		return nil, err
	}
	minerConfig.StorageCost = storageCost
	prepaidAmount, err := pClient.ReadContractBigIntField("prepaidAmount", nil)
	if err != nil {
		return nil, err
	}
	minerConfig.PrepaidAmount = prepaidAmount
	signerFnFactory, signerAddr, err := NewSignerConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get signer: %w", err)
	}
	minerConfig.SignerFnFactory = signerFnFactory
	minerConfig.SignerAddr = signerAddr
	return &minerConfig, nil
}

func NewSignerConfig(ctx *cli.Context) (signer.SignerFactory, common.Address, error) {
	signerConfig := signer.ReadCLIConfig(ctx)
	if err := signerConfig.Check(); err != nil {
		return nil, common.Address{}, fmt.Errorf("invalid siger flags: %w", err)
	}
	return signer.SignerFactoryFromConfig(signerConfig)
}

func NewStorageConfig(ctx *cli.Context, pClient *eth.PollingClient, lg log.Logger) (*storage.StorageConfig, error) {
	miner := common.HexToAddress(ctx.GlobalString(flags.StorageMiner.Name))
	lg.Info("Loaded storage config", "l1Contract", pClient.ContractAddress(), "miner", miner)
	storageCfg, err := InitStorageConfig(pClient, miner, lg)
	if err != nil {
		lg.Error("Failed to load storage config from contract", "error", err)
		return nil, err
	}
	storageCfg.Filenames = ctx.GlobalStringSlice(flags.StorageFiles.Name)
	return storageCfg, nil
}

func NewL1EndpointConfig(ctx *cli.Context, lg log.Logger) (*eth.L1EndpointConfig, *ethclient.Client, error) {
	l1NodeAddr := ctx.GlobalString(flags.L1NodeAddr.Name)
	client, err := ethclient.DialContext(context.Background(), l1NodeAddr)
	if err != nil {
		lg.Error("Failed to connect to the L1 RPC", "error", err, "l1Rpc", l1NodeAddr)
		return nil, nil, err
	}
	chainid, err := client.ChainID(context.Background())
	if err != nil {
		lg.Error("Failed to fetch chain id from the L1 RPC", "error", err, "l1Rpc", l1NodeAddr)
		return nil, nil, err
	}
	return &eth.L1EndpointConfig{
		L1ChainID:                    chainid.Uint64(),
		L1NodeAddr:                   l1NodeAddr,
		L1BlockTime:                  ctx.GlobalUint64(flags.L1BlockTime.Name),
		L1BeaconURL:                  ctx.GlobalString(flags.L1BeaconAddr.Name),
		L1BeaconSlotTime:             ctx.GlobalUint64(flags.L1BeaconSlotTime.Name),
		DAURL:                        ctx.GlobalString(flags.DAURL.Name),
		L1MinDurationForBlobsRequest: ctx.GlobalUint64(flags.L1MinDurationForBlobsRequest.Name),
	}, client, nil
}

func NewDownloaderConfig(ctx *cli.Context) *downloader.Config {
	return &downloader.Config{
		DownloadStart:     ctx.GlobalInt64(flags.DownloadStart.Name),
		DownloadDump:      ctx.GlobalString(flags.DownloadDump.Name),
		DownloadThreadNum: ctx.GlobalInt(flags.DownloadThreadNum.Name),
	}
}

func InitStorageConfig(client *eth.PollingClient, miner common.Address, lg log.Logger) (*storage.StorageConfig, error) {
	exist, err := client.IsContractExist()
	if err != nil {
		return nil, fmt.Errorf("check contract exist fail: %s", err.Error())
	}
	if !exist {
		return nil, fmt.Errorf("contract does not exist")
	}
	result, err := client.ReadContractField("maxKvSizeBits", nil)
	if err != nil {
		return nil, fmt.Errorf("get maxKvSizeBits: %s", err.Error())
	}
	maxKvSizeBits := new(big.Int).SetBytes(result).Uint64()
	chunkSizeBits := maxKvSizeBits
	result, err = client.ReadContractField("shardEntryBits", nil)
	if err != nil {
		return nil, fmt.Errorf("get shardEntryBits: %s", err.Error())
	}
	shardEntryBits := new(big.Int).SetBytes(result).Uint64()

	return &storage.StorageConfig{
		L1Contract:        client.ContractAddress(),
		Miner:             miner,
		KvSize:            1 << maxKvSizeBits,
		ChunkSize:         1 << chunkSizeBits,
		KvEntriesPerShard: 1 << shardEntryBits,
	}, nil
}
