// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package log

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/urfave/cli"
)

const (
	LevelFlagName  = "log.level"
	FormatFlagName = "log.format"
	ColorFlagName  = "log.color"
)

var (
	defaultCLIConfig = CLIConfig{
		Level:  "info",
		Format: "terminal",
		Color:  true,
	}
)

func CLIFlags(envPrefix string) []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:   LevelFlagName,
			Usage:  "The lowest log level that will be output",
			Value:  defaultCLIConfig.Level,
			EnvVar: rollup.PrefixEnvVar(envPrefix, "LOG_LEVEL"),
		},
		cli.StringFlag{
			Name:   FormatFlagName,
			Usage:  "Format the log output. Supported formats: 'text', 'terminal', 'logfmt', 'json', 'json-pretty',",
			Value:  defaultCLIConfig.Format,
			EnvVar: rollup.PrefixEnvVar(envPrefix, "LOG_FORMAT"),
		},
		cli.BoolTFlag{
			Name:  ColorFlagName,
			Usage: "Color the log output if in terminal mode (defaults to true if terminal is detected)",
			// BoolTFlag is true by default
			EnvVar: rollup.PrefixEnvVar(envPrefix, "LOG_COLOR"),
		},
	}
}

type CLIConfig struct {
	Level  string // Log level: trace, debug, info, warn, error, crit. Capitals are accepted too.
	Color  bool   // Color the log output. Defaults to true if terminal is detected.
	Format string // Format the log output. Supported formats: 'text', 'terminal', 'logfmt', 'json', 'json-pretty'
}

func ReadCLIConfig(ctx *cli.Context) CLIConfig {
	cfg := CLIConfig{
		Level:  ctx.GlobalString(LevelFlagName),
		Format: ctx.GlobalString(FormatFlagName),
		Color:  ctx.GlobalBool(ColorFlagName),
	}
	if err := cfg.check(); err != nil {
		panic(err)
	}
	return cfg
}

func levelFromString(lvlString string) (slog.Level, error) {
	lvlString = strings.ToLower(lvlString) // ignore case
	switch lvlString {
	case "trace", "trce":
		return log.LevelTrace, nil
	case "debug", "dbug":
		return log.LevelDebug, nil
	case "info":
		return log.LevelInfo, nil
	case "warn":
		return log.LevelWarn, nil
	case "error", "eror":
		return log.LevelError, nil
	case "crit":
		return log.LevelCrit, nil
	default:
		return log.LevelDebug, fmt.Errorf("unknown level: %v", lvlString)
	}
}

func (cfg CLIConfig) check() error {
	switch cfg.Format {
	case "json", "json-pretty", "terminal", "text", "logfmt":
	default:
		return fmt.Errorf("unrecognized log format: %s", cfg.Format)
	}

	level := strings.ToLower(cfg.Level)
	_, err := levelFromString(level)
	if err != nil {
		return fmt.Errorf("unrecognized log level: %w", err)
	}
	return nil
}
