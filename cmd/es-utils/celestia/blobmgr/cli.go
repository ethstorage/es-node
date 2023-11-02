package blobmgr

import (
	"time"

	"github.com/urfave/cli"
)

const (
	DaRpcFlagName          = "da-rpc"
	NamespaceIdFlagName    = "namespace-id"
	AuthTokenFlagName      = "auth-token"
	L1RPCFlagName          = "l1-eth-rpc"
	L1ContractFlagName     = "l1-contract"
	PrivateKeyFlagName     = "private-key"
	NetworkTimeoutFlagName = "network-timeout"
)

func CLIFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  DaRpcFlagName,
			Usage: "RPC URL of the DA layer",
		},
		cli.StringFlag{
			Name:  NamespaceIdFlagName,
			Usage: "Namespace ID of the DA layer",
		},
		cli.StringFlag{
			Name:  AuthTokenFlagName,
			Usage: "Authentication Token of the DA layer",
		},
		cli.StringFlag{
			Name:  L1RPCFlagName,
			Usage: "HTTP provider URL for L1",
		},
		cli.StringFlag{
			Name:  L1ContractFlagName,
			Usage: "L1 storage contract address",
		},
		cli.StringFlag{
			Name:  PrivateKeyFlagName,
			Usage: "The private key to submit Ethereum transactions with.",
		},
		cli.DurationFlag{
			Name:  NetworkTimeoutFlagName,
			Usage: "Timeout for all network operations",
			Value: 20 * time.Second,
		},
	}
}

type CLIConfig struct {
	DaRpc          string
	NamespaceId    string
	AuthToken      string
	L1RPCURL       string
	L1Contract     string
	PrivateKey     string
	NetworkTimeout time.Duration
}

func ReadCLIConfig(ctx *cli.Context) CLIConfig {
	return CLIConfig{
		DaRpc:          ctx.String(DaRpcFlagName),
		NamespaceId:    ctx.String(NamespaceIdFlagName),
		AuthToken:      ctx.String(AuthTokenFlagName),
		L1RPCURL:       ctx.String(L1RPCFlagName),
		L1Contract:     ctx.String(L1ContractFlagName),
		PrivateKey:     ctx.String(PrivateKeyFlagName),
		NetworkTimeout: ctx.Duration(NetworkTimeoutFlagName),
	}
}
