// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/ethstorage/go-ethstorage/ethstorage/flags/types"
	"github.com/ethstorage/go-ethstorage/ethstorage/flags/utils"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
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
