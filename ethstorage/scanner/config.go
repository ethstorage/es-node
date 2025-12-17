// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"fmt"
	"time"

	"github.com/ethstorage/go-ethstorage/ethstorage/flags/utils"
	"github.com/urfave/cli"
)

const (
	modeDisabled = iota
	modeCheckMeta
	modeCheckBlob
)

const (
	ModeFlagName         = "scanner.mode"
	BatchSizeFlagName    = "scanner.batch-size"
	IntervalMetaFlagName = "scanner.interval.meta"
	IntervalBlobFlagName = "scanner.interval.blob"
)

// intervals in minutes
const defaultIntervalMeta = 3
const defaultIntervalBlob = 24 * 60
const minIntervalMeta = 1
const minIntervalBlob = 5

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
	Mode         scanMode
	BatchSize    int
	IntervalMeta time.Duration
	IntervalBlob time.Duration
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
			Name:   IntervalMetaFlagName,
			Usage:  fmt.Sprintf("Data scan interval of check-meta mode in minutes, minimum %d (default %d)", minIntervalMeta, defaultIntervalMeta),
			EnvVar: scannerEnv("INTERVAL_META"),
			Value:  defaultIntervalMeta,
		},
		cli.IntFlag{
			Name:   IntervalBlobFlagName,
			Usage:  fmt.Sprintf("Data scan interval of check-blob mode in minutes, minimum %d (default %d)", minIntervalBlob, defaultIntervalBlob),
			EnvVar: scannerEnv("INTERVAL_BLOB"),
			Value:  defaultIntervalBlob,
		},
	}
	return flags
}

func NewConfig(ctx *cli.Context) *Config {
	mode := ctx.GlobalInt(ModeFlagName)
	if mode == modeDisabled {
		return nil
	}
	if interval := ctx.GlobalInt(IntervalMetaFlagName); interval < minIntervalMeta {
		panic(fmt.Sprintf("scanner interval of check-meta mode must be at least %d minutes", minIntervalMeta))
	}
	if interval := ctx.GlobalInt(IntervalBlobFlagName); interval < minIntervalBlob {
		panic(fmt.Sprintf("scanner interval of check-blob mode must be at least %d minutes", minIntervalBlob))
	}
	return &Config{
		Mode:         scanMode(mode),
		BatchSize:    ctx.GlobalInt(BatchSizeFlagName),
		IntervalMeta: time.Minute * time.Duration(ctx.GlobalInt(IntervalMetaFlagName)),
		IntervalBlob: time.Minute * time.Duration(ctx.GlobalInt(IntervalBlobFlagName)),
	}
}
