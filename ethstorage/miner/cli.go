// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/ethstorage/go-ethstorage/ethstorage/flags/types"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/urfave/cli"
)

const (
	EnabledFlagName          = "miner.enabled"
	GasPriceFlagName         = "miner.gas-price"
	PriorityGasPriceFlagName = "miner.priority-gas-price"
	ZKeyFileName             = "miner.zkey"
	ZKWorkingDir             = "miner.zk-working-dir"
	ThreadsPerShard          = "miner.threads-per-shard"
)

func CLIFlags(envPrefix string) []cli.Flag {
	envPrefix += "_MINER"
	flag := []cli.Flag{
		cli.BoolFlag{
			Name:   EnabledFlagName,
			Usage:  "Storage mining enabled",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "ENABLED"),
		},
		&types.BigFlag{
			Name:   GasPriceFlagName,
			Usage:  "Gas price for mining transactions",
			Value:  DefaultConfig.GasPrice,
			EnvVar: rollup.PrefixEnvVar(envPrefix, "GAS_PRICE"),
		},
		&types.BigFlag{
			Name:   PriorityGasPriceFlagName,
			Usage:  "Priority gas price for mining transactions",
			Value:  DefaultConfig.PriorityGasPrice,
			EnvVar: rollup.PrefixEnvVar(envPrefix, "PRIORITY_GAS_PRICE"),
		},
		cli.StringFlag{
			Name:   ZKeyFileName,
			Usage:  "zkey file name which should be put in the snarkjs folder",
			Value:  DefaultConfig.ZKeyFileName,
			EnvVar: rollup.PrefixEnvVar(envPrefix, "ZKEY_FILE"),
		},
		cli.StringFlag{
			Name:   ZKWorkingDir,
			Usage:  "Path to the snarkjs folder",
			Value:  DefaultConfig.ZKWorkingDir,
			EnvVar: rollup.PrefixEnvVar(envPrefix, "ZK_WORKING_DIR"),
		},
		cli.Uint64Flag{
			Name:   ThreadsPerShard,
			Usage:  "Number of threads per shard",
			Value:  DefaultConfig.ThreadsPerShard,
			EnvVar: rollup.PrefixEnvVar(envPrefix, "THREADS_PER_SHARD"),
		},
	}
	return flag
}

type CLIConfig struct {
	Enabled          bool
	GasPrice         *big.Int
	PriorityGasPrice *big.Int
	ZKeyFileName     string
	ZKWorkingDir     string
	ThreadsPerShard  uint64
}

func (c CLIConfig) Check() error {
	info, err := os.Stat(filepath.Join(c.ZKWorkingDir, "snarkjs"))
	if err != nil {
		if os.IsNotExist(err) || !info.IsDir() {
			return fmt.Errorf("snarkjs folder not found in ZKWorkingDir: %v\n", err)
		}
	}
	return nil
}

func (c CLIConfig) ToMinerConfig() (Config, error) {
	zkWorkingDir := c.ZKWorkingDir
	if !filepath.IsAbs(zkWorkingDir) {
		dir, err := filepath.Abs(zkWorkingDir)
		if err != nil {
			return Config{}, fmt.Errorf("check ZKWorkingDir error: %v\n", err)
		}
		zkWorkingDir = dir
	}
	cfg := DefaultConfig
	cfg.ZKWorkingDir = zkWorkingDir
	cfg.GasPrice = c.GasPrice
	cfg.PriorityGasPrice = c.PriorityGasPrice
	cfg.ZKeyFileName = c.ZKeyFileName
	cfg.ThreadsPerShard = c.ThreadsPerShard
	return cfg, nil
}

func ReadCLIConfig(ctx *cli.Context) CLIConfig {
	cfg := CLIConfig{
		Enabled:          ctx.GlobalBool(EnabledFlagName),
		GasPrice:         types.GlobalBig(ctx, GasPriceFlagName),
		PriorityGasPrice: types.GlobalBig(ctx, PriorityGasPriceFlagName),
		ZKeyFileName:     ctx.GlobalString(ZKeyFileName),
		ZKWorkingDir:     ctx.GlobalString(ZKWorkingDir),
		ThreadsPerShard:  ctx.GlobalUint64(ThreadsPerShard),
	}
	return cfg
}
