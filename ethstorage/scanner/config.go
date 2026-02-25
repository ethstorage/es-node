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
	// Default Kvs per scan batch; bigger than this may cause run out of gas
	defaultBatchSize = 8192
	// Default intervals in minutes
	defaultIntervalMeta = 3
	defaultIntervalBlob = 60
	// Minutes between fixing loops
	fixingInterval = 12
	// Minutes to wait before trying to fix pending mismatches
	pendingWaitTime = 3
)

const (
	modeDisabled = iota
	// Compare local meta hashes with those in L1 contract
	modeCheckMeta
	// Compute meta hashes from local blobs and compare with those in L1 contract
	modeCheckBlob
)

const (
	modeSetMeta scanModeSet = 1 << iota // 1
	modeSetBlob                         // 2
)

const (
	ModeFlagName         = "scanner.mode"
	BatchSizeFlagName    = "scanner.batch-size"
	IntervalMetaFlagName = "scanner.interval.meta"
	IntervalBlobFlagName = "scanner.interval.blob"
)

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
	return out
}

func scannerEnv(name string) string {
	return utils.PrefixEnvVar("SCANNER_" + name)
}

type Config struct {
	Mode         scanModeSet
	BatchSize    int
	IntervalMeta time.Duration
	IntervalBlob time.Duration
}

func CLIFlags() []cli.Flag {
	flags := []cli.Flag{
		cli.IntFlag{
			Name:   ModeFlagName,
			Usage:  "Data scan mode (bitmask) : 0=disabled, 1=meta, 2=blob; combinations via sum/OR: 3=meta+blob",
			EnvVar: scannerEnv("MODE"),
			Value:  1,
		},
		cli.IntFlag{
			Name:   BatchSizeFlagName,
			Usage:  "Data scan batch size",
			EnvVar: scannerEnv("BATCH_SIZE"),
			Value:  defaultBatchSize,
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
	}
	return flags
}

const scanModeSetMask = modeSetMeta | modeSetBlob

func NewConfig(ctx *cli.Context) (*Config, error) {
	rawMode := scanModeSet(ctx.GlobalInt(ModeFlagName))
	if rawMode == 0 {
		return nil, nil
	}
	// Check for invalid bits outside the valid mask
	if rawMode&^scanModeSetMask != 0 {
		return nil, fmt.Errorf("invalid scanner mode: %d, valid values are 0 (disabled), 1 (meta), 2 (blob), or 3 (meta+blob)", rawMode)
	}
	mode := rawMode & scanModeSetMask
	return &Config{
		Mode:         mode,
		BatchSize:    ctx.GlobalInt(BatchSizeFlagName),
		IntervalMeta: time.Minute * time.Duration(ctx.GlobalInt(IntervalMetaFlagName)),
		IntervalBlob: time.Minute * time.Duration(ctx.GlobalInt(IntervalBlobFlagName)),
	}, nil
}
