// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"fmt"

	"github.com/ethstorage/go-ethstorage/ethstorage/flags/utils"
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
)

func scannerEnv(name string) string {
	return utils.PrefixEnvVar("SCANNER_" + name)
}

type Config struct {
	Mode      int
	BatchSize int
	Interval  int
}

func CLIFlags() []cli.Flag {
	flags := []cli.Flag{
		cli.IntFlag{
			Name:   ModeFlagName,
			Usage:  "Data scan mode, 0: disabled, 1: check meta, 2: check blob",
			EnvVar: scannerEnv("MODE"),
			Value:  1,
		},
		cli.IntFlag{
			Name:   BatchSizeFlagName,
			Usage:  "Data scan batch size",
			EnvVar: scannerEnv("BATCH_SIZE"),
			Value:  8192,
		},
		cli.IntFlag{
			Name:   IntervalFlagName,
			Usage:  "Data scan interval in minutes",
			EnvVar: scannerEnv("INTERVAL"),
			Value:  3,
		},
	}
	return flags
}

func NewConfig(ctx *cli.Context) *Config {
	mode := ctx.GlobalInt(ModeFlagName)
	if mode == modeDisabled {
		return nil
	}
	if mode != modeCheckMeta && mode != modeCheckBlob {
		panic(fmt.Sprintf("invalid scanner mode: %d", mode))
	}
	return &Config{
		Mode:      mode,
		BatchSize: ctx.GlobalInt(BatchSizeFlagName),
		Interval:  ctx.GlobalInt(IntervalFlagName),
	}
}
