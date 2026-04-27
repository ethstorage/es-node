// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE
package email

import (
	"fmt"

	"github.com/ethstorage/go-ethstorage/ethstorage/flags/utils"
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

func emailEnv(name string) string {
	return utils.PrefixEnvVar("EMAIL_" + name)
}

func CLIFlags() []cli.Flag {
	flag := []cli.Flag{
		cli.StringFlag{
			Name:   EmailUsernameFlagName,
			Usage:  "Email username for notifications",
			EnvVar: emailEnv("USERNAME"),
		},
		cli.StringFlag{
			Name:   EmailPasswordFlagName,
			Usage:  "Email password for notifications",
			EnvVar: emailEnv("PASSWORD"),
		},
		cli.StringFlag{
			Name:   EmailHostFlagName,
			Usage:  "Email host for notifications",
			Value:  "smtp.gmail.com",
			EnvVar: emailEnv("HOST"),
		},
		cli.Uint64Flag{
			Name:   EmailPortFlagName,
			Usage:  "Email port for notifications",
			Value:  587,
			EnvVar: emailEnv("PORT"),
		},
		cli.StringSliceFlag{
			Name:   EmailToFlagName,
			Usage:  "Email addresses to send notifications to",
			EnvVar: emailEnv("TO"),
		},
		cli.StringFlag{
			Name:   EmailFromFlagName,
			Usage:  "Email address that will appear as the sender of the notifications",
			EnvVar: emailEnv("FROM"),
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
