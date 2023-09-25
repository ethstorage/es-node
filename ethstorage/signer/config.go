package signer

import (
	"errors"

	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/urfave/cli"
)

const (
	EndpointFlagName   = "signer.endpoint"
	AddressFlagName    = "signer.address"
	MnemonicsFlagName  = "signer.mnemonic"
	HdpathFlagName     = "signer.hdpath"
	PrivateKeyFlagName = "signer.private-key"
)

func CLIFlags(envPrefix string) []cli.Flag {
	envPrefix += "_SIGNER"
	flags := []cli.Flag{
		cli.StringFlag{
			Name:   EndpointFlagName,
			Usage:  "Signer endpoint the client will connect to",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "ENDPOINT"),
		},
		cli.StringFlag{
			Name:   AddressFlagName,
			Usage:  "Address the signer is signing transactions for",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "ADDRESS"),
		},
		cli.StringFlag{
			Name:   MnemonicsFlagName,
			Usage:  "The HD seed used to derive the wallet private keys for mining. Must be used in conjunction with HDPath.",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "MNEMONIC"),
		},
		cli.StringFlag{
			Name:   HdpathFlagName,
			Usage:  "HDPath is the derivation path used to obtain the private key for mining transactions",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "ADDRESS"),
		},
		cli.StringFlag{
			Name:   PrivateKeyFlagName,
			Usage:  "The private key to sign a mining transaction",
			EnvVar: rollup.PrefixEnvVar(envPrefix, "PRIVATE_KEY"),
		},
	}
	return flags
}

type CLIConfig struct {

	// Endpoint is the remote signer url the miner will connect to.
	Endpoint string

	// Address is the address the from address of the mining transactions.
	Address string
	// PrivateKey is the private key used for mining transactions.
	PrivateKey string

	// Mnemonic is the HD seed used to derive the wallet private keys for both
	// the sequence and proposer. Must be used in conjunction with
	// SequencerHDPath and ProposerHDPath.
	Mnemonic string

	// HDPath is the derivation path used to obtain the private key for
	// the mining transactions.
	HDPath string
}

func (c CLIConfig) Check() error {
	if !((c.Endpoint == "" && c.Address == "") || (c.Endpoint != "" && c.Address != "")) {
		return errors.New("signer endpoint and address must both be set or not set")
	}
	if !((c.Mnemonic == "" && c.HDPath == "") || (c.Mnemonic != "" && c.HDPath != "")) {
		return errors.New("mnemonic and hdpath must both be set or not set")
	}
	if c.PrivateKey != "" && c.Mnemonic != "" {
		return errors.New("cannot specify both a private key and a mnemonic")
	}
	if (c.Endpoint == "" && c.Address == "") && c.PrivateKey == "" && c.Mnemonic == "" {
		return errors.New("must specify one of the 3 signer methods: 1) endpoint + address, 2) private key, or 3) mnemonic + hdpath")
	}
	return nil
}

func (c CLIConfig) RemoteEnabled() bool {
	if c.Endpoint != "" && c.Address != "" {
		return true
	}
	return false
}

func ReadCLIConfig(ctx *cli.Context) CLIConfig {
	cfg := CLIConfig{
		Endpoint:   ctx.String(EndpointFlagName),
		Address:    ctx.String(AddressFlagName),
		PrivateKey: ctx.String(PrivateKeyFlagName),
		Mnemonic:   ctx.String(MnemonicsFlagName),
		HDPath:     ctx.String(HdpathFlagName),
	}
	return cfg
}
