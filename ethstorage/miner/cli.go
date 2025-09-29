// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/email"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/flags/types"
	"github.com/ethstorage/go-ethstorage/ethstorage/flags/utils"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
	"github.com/urfave/cli"
)

const (
	EnabledFlagName          = "miner.enabled"
	GasPriceFlagName         = "miner.gas-price"
	PriorityGasPriceFlagName = "miner.priority-gas-price"
	ZKeyFileNameFlagName     = "miner.zkey"
	ZKProverModeFlagName     = "miner.zk-prover-mode"
	ZKProverImplFlagName     = "miner.zk-prover-impl"
	ThreadsPerShardFlagName  = "miner.threads-per-shard"
	MinimumProfitFlagName    = "miner.min-profit"
	MaxGasPriceFlagName      = "miner.max-gas-price"

	// proof submission notify
	EmailEnabledFlagName = "miner.email-enabled"
)

func minerEnv(name string) string {
	return utils.PrefixEnvVar("MINER_" + name)
}

func CLIFlags() []cli.Flag {
	flag := []cli.Flag{
		cli.BoolFlag{
			Name:   EnabledFlagName,
			Usage:  "Storage mining enabled",
			EnvVar: minerEnv("ENABLED"),
		},
		&types.BigFlag{
			Name:   GasPriceFlagName,
			Usage:  "Gas price for mining transactions",
			Value:  DefaultConfig.GasPrice,
			EnvVar: minerEnv("GAS_PRICE"),
		},
		&types.BigFlag{
			Name:   PriorityGasPriceFlagName,
			Usage:  "Priority gas price for mining transactions",
			Value:  DefaultConfig.PriorityGasPrice,
			EnvVar: minerEnv("PRIORITY_GAS_PRICE"),
		},
		&types.BigFlag{
			Name:   MinimumProfitFlagName,
			Usage:  "Minimum profit for mining transactions in wei",
			Value:  DefaultConfig.MinimumProfit,
			EnvVar: minerEnv("MIN_PROFIT"),
		},
		cli.StringFlag{
			Name:   ZKeyFileNameFlagName,
			Usage:  "zkey file name with path",
			Value:  DefaultConfig.ZKeyFile,
			EnvVar: minerEnv("ZKEY_FILE"),
		},
		cli.Uint64Flag{
			Name:   ZKProverModeFlagName,
			Usage:  "ZK prover mode, 1: one proof per sample, 2: one proof for multiple samples",
			Value:  DefaultConfig.ZKProverMode,
			EnvVar: minerEnv("ZK_PROVER_MODE"),
		},
		cli.Uint64Flag{
			Name:   ZKProverImplFlagName,
			Usage:  "ZK prover implementation, 1: snarkjs, 2: go-rapidsnark",
			Value:  DefaultConfig.ZKProverImpl,
			EnvVar: minerEnv("ZK_PROVER_IMPL"),
		},
		cli.Uint64Flag{
			Name:   ThreadsPerShardFlagName,
			Usage:  "Number of threads per shard",
			Value:  DefaultConfig.ThreadsPerShard,
			EnvVar: minerEnv("THREADS_PER_SHARD"),
		},
		cli.BoolFlag{
			Name:   EmailEnabledFlagName,
			Usage:  "Enable proof submission notifications via email",
			EnvVar: minerEnv("EMAIL_ENABLED"),
		},
		&types.BigFlag{
			Name:   MaxGasPriceFlagName,
			Usage:  "Maximum gas price for mining transactions, set to 0 to disable the check",
			Value:  DefaultConfig.MaxGasPrice,
			EnvVar: minerEnv("MAX_GAS_PRICE"),
		},
	}
	return flag
}

type CLIConfig struct {
	Enabled          bool
	GasPrice         *big.Int
	PriorityGasPrice *big.Int
	MinimumProfit    *big.Int
	MaxGasPrice      *big.Int
	ZKeyFile         string
	ZKWorkingDir     string
	ZKProverMode     uint64
	ZKProverImpl     uint64
	ThreadsPerShard  uint64
	EmailEnabled     bool
}

func (c CLIConfig) Check() error {
	info, err := os.Stat(filepath.Join(c.ZKWorkingDir, prover.SnarkLib))
	if err != nil {
		if os.IsNotExist(err) || !info.IsDir() {
			return fmt.Errorf("%s folder not found in ZKWorkingDir: %v", prover.SnarkLib, err)
		}
	}
	return nil
}

func (c CLIConfig) ToMinerConfig() (Config, error) {
	zkWorkingDir := c.ZKWorkingDir
	if !filepath.IsAbs(zkWorkingDir) {
		dir, err := filepath.Abs(zkWorkingDir)
		if err != nil {
			return Config{}, fmt.Errorf("check ZKWorkingDir error: %v", err)
		}
		zkWorkingDir = dir
	}
	zkFile := c.ZKeyFile
	if !filepath.IsAbs(zkFile) {
		dir, err := filepath.Abs(zkFile)
		if err != nil {
			return Config{}, fmt.Errorf("check ZKeyFileName error: %v", err)
		}
		zkFile = dir
	}
	cfg := DefaultConfig
	cfg.ZKWorkingDir = zkWorkingDir
	cfg.ZKeyFile = zkFile
	cfg.ZKProverMode = c.ZKProverMode
	cfg.ZKProverImpl = c.ZKProverImpl
	cfg.GasPrice = c.GasPrice
	cfg.PriorityGasPrice = c.PriorityGasPrice
	cfg.MinimumProfit = c.MinimumProfit
	cfg.ThreadsPerShard = c.ThreadsPerShard
	cfg.EmailEnabled = c.EmailEnabled
	cfg.MaxGasPrice = c.MaxGasPrice
	return cfg, nil
}

func ReadCLIConfig(ctx *cli.Context) CLIConfig {
	cfg := CLIConfig{
		Enabled:          ctx.GlobalBool(EnabledFlagName),
		GasPrice:         types.GlobalBig(ctx, GasPriceFlagName),
		PriorityGasPrice: types.GlobalBig(ctx, PriorityGasPriceFlagName),
		MinimumProfit:    types.GlobalBig(ctx, MinimumProfitFlagName),
		ZKeyFile:         ctx.GlobalString(ZKeyFileNameFlagName),
		ZKWorkingDir:     DefaultConfig.ZKWorkingDir,
		ZKProverMode:     ctx.GlobalUint64(ZKProverModeFlagName),
		ZKProverImpl:     ctx.GlobalUint64(ZKProverImplFlagName),
		ThreadsPerShard:  ctx.GlobalUint64(ThreadsPerShardFlagName),
		EmailEnabled:     ctx.GlobalBool(EmailEnabledFlagName),
		MaxGasPrice:      types.GlobalBig(ctx, MaxGasPriceFlagName),
	}
	return cfg
}

func NewMinerConfig(ctx *cli.Context, pClient *eth.PollingClient, minerAddr common.Address, lg log.Logger) (*Config, error) {
	cliConfig := ReadCLIConfig(ctx)
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
