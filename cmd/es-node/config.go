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
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/flags"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/node"
	p2pcli "github.com/ethstorage/go-ethstorage/ethstorage/p2p/cli"
	"github.com/ethstorage/go-ethstorage/ethstorage/scanner"
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

	l1Contract := common.HexToAddress(ctx.GlobalString(flags.StorageL1Contract.Name))
	pClient := eth.NewClient(context.Background(), client, true, l1Contract, 0, nil, lg)
	storageConfig, err := NewStorageConfig(ctx, pClient, lg)
	if err != nil {
		return nil, fmt.Errorf("failed to load storage config: %w", err)
	}

	minerConfig, err := miner.NewMinerConfig(ctx, pClient, storageConfig.Miner, lg)
	if err != nil {
		return nil, fmt.Errorf("failed to load miner config: %w", err)
	}
	chainId := new(big.Int).SetUint64(ctx.GlobalUint64(flags.ChainId.Name))
	lg.Info("Read chain ID of EthStorage network", "chainID", chainId)
	if minerConfig != nil {
		minerConfig.ChainID = chainId
	}
	archiverConfig := archiver.NewConfig(ctx)

	cfg := &node.Config{
		L1:      *l1Endpoint,
		ChainID: chainId,
		Downloader: downloader.Config{
			DownloadStart:     ctx.GlobalInt64(flags.DownloadStart.Name),
			DownloadDump:      ctx.GlobalString(flags.DownloadDump.Name),
			DownloadThreadNum: ctx.GlobalInt(flags.DownloadThreadNum.Name),
		},

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

func NewStorageConfig(ctx *cli.Context, pClient *eth.PollingClient, lg log.Logger) (*storage.StorageConfig, error) {
	miner := common.HexToAddress(ctx.GlobalString(flags.StorageMiner.Name))
	lg.Info("Loaded storage config", "l1Contract", pClient.ContractAddress(), "miner", miner)
	storageCfg, err := pClient.LoadStorageConfigFromContract(miner)
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
