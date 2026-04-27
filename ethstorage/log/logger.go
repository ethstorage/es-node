// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package log

import (
	"io"
	"log/slog"
	"os"

	"github.com/ethereum/go-ethereum/log"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
)

func NewLogger(cfg CLIConfig) log.Logger {
	l, err := levelFromString(cfg.Level)
	if err != nil {
		panic(err)
	}

	// Select output (enable Windows color support if needed)
	output := io.Writer(os.Stdout)
	useColor := cfg.Color && (isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())) && os.Getenv("TERM") != "dumb"

	// Enable cross-platform color support for Windows terminals using go-colorable wrapper
	if useColor {
		output = colorable.NewColorableStdout()
	}

	var h slog.Handler
	switch cfg.Format {
	case "terminal":
		// Terminal format with timestamp and optional colors
		h = log.NewTerminalHandlerWithLevel(output, l, useColor)
	case "text", "logfmt":
		// slog text handler is logfmt-like: key=value pairs per line
		h = slog.NewTextHandler(output, &slog.HandlerOptions{Level: l})
	case "json", "json-pretty":
		// Pretty JSON is not natively supported; fall back to compact JSON
		h = slog.NewJSONHandler(output, &slog.HandlerOptions{Level: l})
	default:
		h = log.NewTerminalHandlerWithLevel(output, l, useColor)
	}

	return log.NewLogger(h)
}

func DefaultLogger() log.Logger {
	return NewLogger(defaultCLIConfig)
}

func SetupDefaults() {
	log.SetDefault(DefaultLogger())
}
