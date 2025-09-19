// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/urfave/cli"
)

const (
	EnabledFlagName      = "archiver.enabled"
	ListenAddrFlagName   = "archiver.addr"
	ListenPortFlagName   = "archiver.port"
	MaxBlobsPerBlockName = "archiver.maxBlobsPerBlock"
)

type Config struct {
	Enabled          bool
	ListenAddr       string
	ListenPort       int
	MaxBlobsPerBlock int
}

func CLIFlags(envPrefix string) []cli.Flag {
	envPrefix += "_ARCHIVER"
	flags := []cli.Flag{
		cli.BoolFlag{
			Name:   EnabledFlagName,
			Usage:  "Blob archiver enabled",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "ENABLED"),
		},
		cli.StringFlag{
			Name:   ListenAddrFlagName,
			Usage:  "Blob archiver listening address",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "ADDRESS"),
			Value:  "0.0.0.0",
		},
		cli.IntFlag{
			Name:   ListenPortFlagName,
			Usage:  "Blob archiver listening port",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "PORT"),
			Value:  9645,
		},
		cli.IntFlag{
			Name:   MaxBlobsPerBlockName,
			Usage:  "Max blobs per block",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "MAX_BLOBS_PER_BLOCK"),
			Value:  0, // 0 means no limit
		},
	}
	return flags
}

func NewConfig(ctx *cli.Context) *Config {
	cfg := Config{
		Enabled:          ctx.GlobalBool(EnabledFlagName),
		ListenAddr:       ctx.GlobalString(ListenAddrFlagName),
		ListenPort:       ctx.GlobalInt(ListenPortFlagName),
		MaxBlobsPerBlock: ctx.GlobalInt(MaxBlobsPerBlockName),
	}
	if cfg.Enabled {
		return &cfg
	}
	return nil
}
