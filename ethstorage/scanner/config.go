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

const defaultInterval = 3 // in minutes

func scannerEnv(name string) string {
	return utils.PrefixEnvVar("SCANNER_" + name)
}

type scanMode int

func (m scanMode) String() string {
	switch m {
	case modeDisabled:
		return "disabled"
	case modeCheckMeta:
		return "check-meta"
	case modeCheckBlob:
		return "check-blob"
	case modeCheckBlob + modeCheckMeta:
		return "hybrid"
	default:
		panic(fmt.Sprintf("invalid scanner mode: %d", m))
	}
}

type Config struct {
	Mode      scanMode
	BatchSize int
	Interval  int
}

func CLIFlags() []cli.Flag {
	flags := []cli.Flag{
		cli.IntFlag{
			Name:   ModeFlagName,
			Usage:  "Data scan mode, 0: disabled, 1: check meta, 2: check blob, 3: hybrid",
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
			Usage:  fmt.Sprintf("Data scan interval in minutes, minimum %d (default). In hybrid mode, the interval applies to meta mode in minutes, blob mode in days", defaultInterval),
			EnvVar: scannerEnv("INTERVAL"),
			Value:  defaultInterval,
		},
	}
	return flags
}

func NewConfig(ctx *cli.Context) *Config {
	mode := ctx.GlobalInt(ModeFlagName)
	if mode == modeDisabled {
		return nil
	}
	if mode != modeCheckMeta && mode != modeCheckBlob && mode != modeCheckBlob+modeCheckMeta {
		panic(fmt.Sprintf("invalid scanner mode: %d", mode))
	}
	if interval := ctx.GlobalInt(IntervalFlagName); interval < defaultInterval {
		panic(fmt.Sprintf("scanner interval must be at least %d minutes", defaultInterval))
	}
	return &Config{
		Mode:      scanMode(mode),
		BatchSize: ctx.GlobalInt(BatchSizeFlagName),
		Interval:  ctx.GlobalInt(IntervalFlagName),
	}
}
