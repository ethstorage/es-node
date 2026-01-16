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

const (
	modeDisabled = iota
	// Compare local meta hashes with those in L1 contract
	modeCheckMeta
	// Compute meta hashes from local blobs and compare with those in L1 contract
	modeCheckBlob
	// Scan updated KVs from recent blocks and run "check-blob" on them
	modeCheckBlock
)

// scanMode is an internal per-loop mode used by scan workers (meta/blob/block).
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
	default:
		return fmt.Sprintf("unknown(%d)", int(m))
	}
}

const (
	modeSetMeta  scanModeSet = 1 << iota // 1
	modeSetBlob                          // 2
	modeSetBlock                         // 4
)

const scanModeSetMask = modeSetMeta | modeSetBlob | modeSetBlock // 7

// scanModeSet is a combination of scanMode values used for configuration purposes.
type scanModeSet uint8

func (m scanModeSet) String() string {
	if m == 0 {
		return "disabled"
	}

	out := ""
	if m&modeSetMeta != 0 {
		out = "check-meta"
	}
	if m&modeSetBlob != 0 {
		if out != "" {
			out += "+"
		}
		out += "check-blob"
	}
	if m&modeSetBlock != 0 {
		if out != "" {
			out += "+"
		}
		out += "check-block"
	}
	return out
}

type Config struct {
	Mode          scanModeSet
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
			Usage:  "Data scan mode (bitmask) : 0=disabled, 1=meta, 2=blob, 4=block; combinations via sum/OR: 3=meta+blob, 5=meta+block, 6=blob+block, 7=all",
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
	mode := scanModeSet(ctx.GlobalInt(ModeFlagName)) & scanModeSetMask

	if mode == 0 {
		return nil
	}
	return &Config{
		Mode:          mode,
		BatchSize:     ctx.GlobalInt(BatchSizeFlagName),
		L1SlotTime:    time.Second * time.Duration(slot),
		IntervalMeta:  time.Minute * time.Duration(ctx.GlobalInt(IntervalMetaFlagName)),
		IntervalBlob:  time.Minute * time.Duration(ctx.GlobalInt(IntervalBlobFlagName)),
		IntervalBlock: time.Minute * time.Duration(ctx.GlobalInt(IntervalBlockFlagName)),
	}
}
