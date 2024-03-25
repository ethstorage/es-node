package log

import (
	"fmt"
	"io"
	"os"
	"strings"

	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/urfave/cli"
	"golang.org/x/exp/slog"
	"golang.org/x/term"
)

const (
	LevelFlagName  = "log.level"
	FormatFlagName = "log.format"
	ColorFlagName  = "log.color"
)

func CLIFlags(envPrefix string) []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:   LevelFlagName,
			Usage:  "The lowest log level that will be output",
			Value:  "info",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "LOG_LEVEL"),
		},
		cli.StringFlag{
			Name:   FormatFlagName,
			Usage:  "Format the log output. Supported formats: 'text', 'terminal', 'logfmt', 'json'",
			Value:  "text",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "LOG_FORMAT"),
		},
		cli.BoolFlag{
			Name:   ColorFlagName,
			Usage:  "Color the log output if in terminal mode",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "LOG_COLOR"),
		},
	}
}

type CLIConfig struct {
	Level  string // Log level: trace, debug, info, warn, error, crit. Capitals are accepted too.
	Color  bool   // Color the log output. Defaults to true if terminal is detected.
	Format string // Format the log output. Supported formats: 'text', 'terminal', 'logfmt', 'json''
}

func (cfg CLIConfig) Check() error {
	switch cfg.Format {
	case "json", "terminal", "text", "logfmt":
	default:
		return fmt.Errorf("unrecognized log format: %s", cfg.Format)
	}

	if _, err := LevelFromString(cfg.Level); err != nil {
		return fmt.Errorf("unrecognized log level: %w", err)
	}
	return nil
}

func NewLogger(cfg CLIConfig) log.Logger {
	handler := FormatHandler(cfg.Format, cfg.Color)(os.Stdout)
	handler = oplog.NewDynamicLogHandler(Level(cfg.Level), handler)
	log.SetDefault(log.NewLogger(handler))
	logger := log.New()
	return logger
}

func DefaultCLIConfig() CLIConfig {
	return CLIConfig{
		Level:  "info",
		Format: "text",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	}
}

func ReadLocalCLIConfig(ctx *cli.Context) CLIConfig {
	cfg := DefaultCLIConfig()
	cfg.Level = ctx.String(LevelFlagName)
	cfg.Format = ctx.String(FormatFlagName)
	if ctx.IsSet(ColorFlagName) {
		cfg.Color = ctx.Bool(ColorFlagName)
	}
	return cfg
}

func ReadCLIConfig(ctx *cli.Context) CLIConfig {
	cfg := DefaultCLIConfig()
	cfg.Level = ctx.GlobalString(LevelFlagName)
	cfg.Format = ctx.GlobalString(FormatFlagName)
	if ctx.IsSet(ColorFlagName) {
		cfg.Color = ctx.GlobalBool(ColorFlagName)
	}
	return cfg
}

func FormatHandler(lf string, color bool) func(io.Writer) slog.Handler {
	termColorHandler := func(w io.Writer) slog.Handler {
		return log.NewTerminalHandler(w, color)
	}
	switch lf {
	case "json":
		return log.JSONHandler
	case "text":
		if term.IsTerminal(int(os.Stdout.Fd())) {
			return termColorHandler
		} else {
			return log.LogfmtHandler
		}
	case "terminal":
		return termColorHandler
	case "logfmt":
		return log.LogfmtHandler
	default:
		panic("Failed to create `log.Handler` from options")
	}
}

// Level parses the level string into an appropriate object
func Level(s string) slog.Level {
	s = strings.ToLower(s) // ignore case
	l, err := LevelFromString(s)
	if err != nil {
		panic(fmt.Sprintf("Could not parse log level: %v", err))
	}
	return l
}

func LevelFromString(lvlString string) (slog.Level, error) {
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

func SetupDefaults() {
	log.SetDefault(
		log.NewLogger(
			log.LogfmtHandlerWithLevel(os.Stdout, log.LevelInfo),
		),
	)
}
