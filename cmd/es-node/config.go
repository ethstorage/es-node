// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"context"
	"fmt"
	"math/big"

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
	"github.com/ethstorage/go-ethstorage/ethstorage/node"
	p2pcli "github.com/ethstorage/go-ethstorage/ethstorage/p2p/cli"
	"github.com/ethstorage/go-ethstorage/ethstorage/scanner"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
	"github.com/urfave/cli"
)

// NewConfig creates a Config from the provided flags or environment variables.
func NewConfig(ctx *cli.Context, lg log.Logger) (*node.Config, error) {
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

	storageConfig, err := NewStorageConfig(ctx, client, lg)
	if err != nil {
		return nil, fmt.Errorf("failed to load storage config: %w", err)
	}

	dlConfig := NewDownloaderConfig(ctx)
	minerConfig, err := NewMinerConfig(ctx, client, storageConfig.L1Contract, storageConfig.Miner, lg)
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
	cfg := &node.Config{
		L1:         *l1Endpoint,
		ChainID:    chainId,
		Downloader: *dlConfig,

		DataDir:        datadir,
		StateUploadURL: ctx.GlobalString(flags.StateUploadURL.Name),
		DBConfig:       db.DefaultDBConfig(),
		// rpc url to get randao from
		RandaoSourceURL: ctx.GlobalString(flags.RandaoURL.Name),
		// 	Driver: *driverConfig,
		RPC: node.RPCConfig{
			ListenAddr: ctx.GlobalString(flags.RPCListenAddr.Name),
			ListenPort: ctx.GlobalInt(flags.RPCListenPort.Name),
			ESCallURL:  ctx.GlobalString(flags.RPCESCallURL.Name),
		},
		Metrics: node.MetricsConfig{
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

func NewMinerConfig(ctx *cli.Context, client *ethclient.Client, l1Contract, minerAddr common.Address, lg log.Logger) (*miner.Config, error) {
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

	cctx := context.Background()
	cr := newContractReader(cctx, client, l1Contract, lg)

	randomChecks, err := cr.readUint("randomChecks")
	if err != nil {
		return nil, err
	}
	minerConfig.RandomChecks = randomChecks
	nonceLimit, err := cr.readUint("nonceLimit")
	if err != nil {
		return nil, err
	}
	minerConfig.NonceLimit = nonceLimit
	minimumDiff, err := cr.readBig("minimumDiff")
	if err != nil {
		return nil, err
	}
	minerConfig.MinimumDiff = minimumDiff
	cutoff, err := cr.readBig("cutoff")
	if err != nil {
		return nil, err
	}
	minerConfig.Cutoff = cutoff
	diffAdjDivisor, err := cr.readBig("diffAdjDivisor")
	if err != nil {
		return nil, err
	}
	minerConfig.DiffAdjDivisor = diffAdjDivisor
	dcf, err := cr.readBig("dcfFactor")
	if err != nil {
		return nil, err
	}
	minerConfig.DcfFactor = dcf

	startTime, err := cr.readUint("startTime")
	if err != nil {
		return nil, err
	}
	minerConfig.StartTime = startTime
	shardEntryBits, err := cr.readUint("shardEntryBits")
	if err != nil {
		return nil, err
	}
	minerConfig.ShardEntry = 1 << shardEntryBits
	treasuryShare, err := cr.readUint("treasuryShare")
	if err != nil {
		return nil, err
	}
	minerConfig.TreasuryShare = treasuryShare
	storageCost, err := cr.readBig("storageCost")
	if err != nil {
		return nil, err
	}
	minerConfig.StorageCost = storageCost
	prepaidAmount, err := cr.readBig("prepaidAmount")
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

func NewStorageConfig(ctx *cli.Context, client *ethclient.Client, lg log.Logger) (*storage.StorageConfig, error) {
	l1Contract := common.HexToAddress(ctx.GlobalString(flags.StorageL1Contract.Name))
	miner := common.HexToAddress(ctx.GlobalString(flags.StorageMiner.Name))
	lg.Info("Loaded storage config", "l1Contract", l1Contract, "miner", miner)
	storageCfg, err := initStorageConfig(context.Background(), client, l1Contract, miner, lg)
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
