// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/urfave/cli"
)

const (
	EnabledFlagName   = "scanner.enabled"
	BatchSizeFlagName = "scanner.batch-size"
	IntervalFlagName  = "scanner.interval"
	EsRpcFlagName     = "scanner.es-rpc"
)

type Config struct {
	Enabled   bool
	BatchSize int
	Interval  int
	EsRpc     string
}

func CLIFlags(envPrefix string) []cli.Flag {
	envPrefix += "_SCANNER"
	flags := []cli.Flag{
		cli.BoolFlag{
			Name:   EnabledFlagName,
			Usage:  "Data scan enabled",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "ENABLED"),
		},
		cli.IntFlag{
			Name:   BatchSizeFlagName,
			Usage:  "Data scan batch size",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "BATCH_SIZE"),
			Value:  4096,
		},
		cli.IntFlag{
			Name:   IntervalFlagName,
			Usage:  "Data scan interval in minutes",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "INTERVAL"),
			Value:  4,
		},
		cli.StringFlag{
			Name:   EsRpcFlagName,
			Usage:  "EthStorage RPC endpoint to query blobs",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "ES_RPC"),
		},
	}
	return flags
}

func NewConfig(ctx *cli.Context) *Config {
	// scan unless specifically disabled
	if ctx.GlobalIsSet(EnabledFlagName) {
		enabled := ctx.GlobalBool(EnabledFlagName)
		if !enabled {
			return nil
		}
	}
	return &Config{
		Enabled:   true,
		BatchSize: ctx.GlobalInt(BatchSizeFlagName),
		Interval:  ctx.GlobalInt(IntervalFlagName),
		EsRpc:     ctx.GlobalString(EsRpcFlagName),
	}
}
