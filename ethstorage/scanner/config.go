// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/urfave/cli"
)

const (
	EnabledFlagName = "scanner.enabled"
	IntervalName    = "scanner.interval"
)

type Config struct {
	Enabled  bool
	Interval int
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
			Name:   IntervalName,
			Usage:  "Data scan interval in seconds",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "INTERVAL"),
			Value:  180,
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
		Enabled:  true,
		Interval: ctx.GlobalInt(IntervalName),
	}
}
