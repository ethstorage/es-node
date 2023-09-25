// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"math/big"

	"github.com/ethstorage/go-ethstorage/ethstorage/flags/types"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/urfave/cli"
)

const (
	EnabledFlagName          = "miner.enabled"
	GasPriceFlagName         = "miner.gas-price"
	PriorityGasPriceFlagName = "miner.priority-gas-price"
	ZKeyFileName             = "miner.zkey"
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
			Usage:  "Path to snarkjs zkey file",
			Value:  DefaultConfig.ZKeyFileName,
			EnvVar: rollup.PrefixEnvVar(envPrefix, "ZKEY_FILE"),
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
	ThreadsPerShard  uint64
}

func (c CLIConfig) Check() error {
	return nil
}

func (c CLIConfig) ToMinerConfig() Config {
	return Config{
		GasPrice:         c.GasPrice,
		PriorityGasPrice: c.PriorityGasPrice,
		ZKeyFileName:     c.ZKeyFileName,
		ThreadsPerShard:  c.ThreadsPerShard,
	}
}

func ReadCLIConfig(ctx *cli.Context) CLIConfig {
	cfg := CLIConfig{
		Enabled:          ctx.GlobalBool(EnabledFlagName),
		GasPrice:         types.GlobalBig(ctx, GasPriceFlagName),
		PriorityGasPrice: types.GlobalBig(ctx, PriorityGasPriceFlagName),
		ZKeyFileName:     ctx.GlobalString(ZKeyFileName),
		ThreadsPerShard:  ctx.GlobalUint64(ThreadsPerShard),
	}
	return cfg
}
