// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/urfave/cli"
)

const (
	modeDisabled = iota
	modeCheckMeta
	modeCheckBlob
)

const (
	ModeFlagName      = "scanner.mode"
	BatchSizeFlagName = "scanner.batch-size"
	IntervalFlagName  = "scanner.interval"
	EsRpcFlagName     = "scanner.es-rpc"
)

type Config struct {
	Mode      int
	BatchSize int
	Interval  int
	EsRpc     string
}

func CLIFlags(envPrefix string) []cli.Flag {
	envPrefix += "_SCANNER"
	flags := []cli.Flag{
		cli.IntFlag{
			Name:   ModeFlagName,
			Usage:  "Data scan mode, 0: disabled, 1: check meta, 2: check blob",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "MODE"),
			Value:  1,
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
	var mode int
	if ctx.GlobalIsSet(ModeFlagName) {
		mode = ctx.GlobalInt(ModeFlagName)
		if mode != modeCheckMeta && mode != modeCheckBlob {
			return nil
		}
	}
	return &Config{
		Mode:      mode,
		BatchSize: ctx.GlobalInt(BatchSizeFlagName),
		Interval:  ctx.GlobalInt(IntervalFlagName),
		EsRpc:     ctx.GlobalString(EsRpcFlagName),
	}
}
