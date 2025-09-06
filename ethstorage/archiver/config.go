// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"github.com/ethstorage/go-ethstorage/ethstorage/flags/utils"
	"github.com/urfave/cli"
)

const (
	EnabledFlagName      = "archiver.enabled"
	ListenAddrFlagName   = "archiver.addr"
	ListenPortFlagName   = "archiver.port"
	MaxBlobsPerBlockName = "archiver.maxBlobsPerBlock"
)

func archiverEnv(name string) string {
	return utils.PrefixEnvVar("ARCHIVER_" + name)
}

type Config struct {
	Enabled          bool
	ListenAddr       string
	ListenPort       int
	MaxBlobsPerBlock int
}

func CLIFlags() []cli.Flag {
	flags := []cli.Flag{
		cli.BoolFlag{
			Name:   EnabledFlagName,
			Usage:  "Blob archiver enabled",
			EnvVar: archiverEnv("ENABLED"),
		},
		cli.StringFlag{
			Name:   ListenAddrFlagName,
			Usage:  "Blob archiver listening address",
			EnvVar: archiverEnv("ADDRESS"),
			Value:  "0.0.0.0",
		},
		cli.IntFlag{
			Name:   ListenPortFlagName,
			Usage:  "Blob archiver listening port",
			EnvVar: archiverEnv("PORT"),
			Value:  9645,
		},
		cli.IntFlag{
			Name:   MaxBlobsPerBlockName,
			Usage:  "Max blobs per block",
			EnvVar: archiverEnv("MAX_BLOBS_PER_BLOCK"),
			Value:  6,
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
