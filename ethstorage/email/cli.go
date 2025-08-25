// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE
package email

import (
	"fmt"

	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/urfave/cli"
)

const (
	EmailUsernameFlagName = "email.username"
	EmailPasswordFlagName = "email.password"
	EmailHostFlagName     = "email.host"
	EmailPortFlagName     = "email.port"
	EmailToFlagName       = "email.to"
	EmailFromFlagName     = "email.from"
)

func CLIFlags(envPrefix string) []cli.Flag {
	envPrefix += "_EMAIL"
	flag := []cli.Flag{
		cli.StringFlag{
			Name:   EmailUsernameFlagName,
			Usage:  "Email username for notifications",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "USERNAME"),
		},
		cli.StringFlag{
			Name:   EmailPasswordFlagName,
			Usage:  "Email password for notifications",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "PASSWORD"),
		},
		cli.StringFlag{
			Name:   EmailHostFlagName,
			Usage:  "Email host for notifications",
			Value:  "smtp.gmail.com",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "HOST"),
		},
		cli.Uint64Flag{
			Name:   EmailPortFlagName,
			Usage:  "Email port for notifications",
			Value:  587,
			EnvVar: rollup.PrefixEnvVar(envPrefix, "PORT"),
		},
		cli.StringSliceFlag{
			Name:   EmailToFlagName,
			Usage:  "Email addresses to send notifications to",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "TO"),
		},
		cli.StringFlag{
			Name:   EmailFromFlagName,
			Usage:  "Email address that will appear as the sender of the notifications",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "FROM"),
		},
	}
	return flag
}

func GetEmailConfig(ctx *cli.Context) (*EmailConfig, error) {
	cfg := &EmailConfig{
		Username: ctx.String(EmailUsernameFlagName),
		Password: ctx.String(EmailPasswordFlagName),
		Host:     ctx.String(EmailHostFlagName),
		Port:     ctx.Uint64(EmailPortFlagName),
		To:       ctx.String(EmailToFlagName),
		From:     ctx.String(EmailFromFlagName),
	}
	if err := cfg.Check(); err != nil {
		return nil, fmt.Errorf("email config check error: %w", err)
	}
	return cfg, nil
}
