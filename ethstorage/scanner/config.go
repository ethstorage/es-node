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
	modeCheckBlock
	modeHybrid
)

const (
	ModeFlagName          = "scanner.mode"
	BatchSizeFlagName     = "scanner.batch-size"
	IntervalMetaFlagName  = "scanner.interval.meta"
	IntervalBlobFlagName  = "scanner.interval.blob"
	IntervalBlockFlagName = "scanner.interval.block"
)

// intervals in minutes
const defaultIntervalMeta = 3
const defaultIntervalBlob = 60
const defaultIntervalBlock = 24 * 60

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
	case modeCheckBlock:
		return "check-block"
	case modeHybrid:
		return "hybrid"
	default:
		panic(fmt.Sprintf("invalid scanner mode: %d", m))
	}
}

type Config struct {
	Mode          scanMode
	BatchSize     int
	L1SlotTime    time.Duration
	IntervalMeta  time.Duration
	IntervalBlob  time.Duration
	IntervalBlock time.Duration
}

func CLIFlags() []cli.Flag {
	flags := []cli.Flag{
		cli.IntFlag{
			Name:   ModeFlagName,
			Usage:  "Data scan mode, 0: disabled, 1: check meta, 2: check blob, 3: check block, 4: hybrid",
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
			Usage:  fmt.Sprintf("Data scan interval of check-meta mode in minutes (default %d)", defaultIntervalMeta),
			EnvVar: scannerEnv("INTERVAL_META"),
			Value:  defaultIntervalMeta,
		},
		cli.IntFlag{
			Name:   IntervalBlobFlagName,
			Usage:  fmt.Sprintf("Data scan interval of check-blob mode in minutes (default %d)", defaultIntervalBlob),
			EnvVar: scannerEnv("INTERVAL_BLOB"),
			Value:  defaultIntervalBlob,
		},
		cli.IntFlag{
			Name:   IntervalBlockFlagName,
			Usage:  fmt.Sprintf("Data scan interval of check-block mode in minutes (default %d)", defaultIntervalBlock),
			EnvVar: scannerEnv("INTERVAL_BLOCK"),
			Value:  defaultIntervalBlock,
		},
	}
	return flags
}

func NewConfig(ctx *cli.Context, slot uint64) *Config {
	mode := ctx.GlobalInt(ModeFlagName)
	if mode == modeDisabled {
		return nil
	}
	return &Config{
		Mode:          scanMode(mode),
		BatchSize:     ctx.GlobalInt(BatchSizeFlagName),
		L1SlotTime:    time.Second * time.Duration(slot),
		IntervalMeta:  time.Minute * time.Duration(ctx.GlobalInt(IntervalMetaFlagName)),
		IntervalBlob:  time.Minute * time.Duration(ctx.GlobalInt(IntervalBlobFlagName)),
		IntervalBlock: time.Minute * time.Duration(ctx.GlobalInt(IntervalBlockFlagName)),
	}
}
