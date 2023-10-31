package txmgr

import (
	"context"
	"fmt"
	"math/big"
	"time"

	opcrypto "github.com/ethereum-optimism/optimism/op-service/crypto"
	"github.com/ethereum-optimism/optimism/op-service/signer"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli"
)

const (
	DaRpcFlagName          = "da-rpc"
	NamespaceIdFlagName    = "namespace-id"
	AuthTokenFlagName      = "auth-token"
	L1RPCFlagName          = "l1-eth-rpc"
	PrivateKeyFlagName     = "private-key"
	NetworkTimeoutFlagName = "network-timeout"
)

func CLIFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  DaRpcFlagName,
			Usage: "RPC URL of the DA layer",
			Value: "",
		},
		cli.StringFlag{
			Name:  NamespaceIdFlagName,
			Usage: "Namespace ID of the DA layer",
		},
		cli.StringFlag{
			Name:  AuthTokenFlagName,
			Usage: "Authentication Token of the DA layer",
			Value: "",
		},
		cli.StringFlag{
			Name:  "l1-eth-rpc",
			Usage: "HTTP provider URL for L1",
			Value: "http://localhost:8545",
		},
		cli.StringFlag{
			Name:  "private-key",
			Usage: "The private key to use with the service. Must not be used with mnemonic.",
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
	PrivateKey     string
	NetworkTimeout time.Duration
}

func ReadCLIConfig(ctx *cli.Context) CLIConfig {
	return CLIConfig{
		DaRpc:       ctx.GlobalString(DaRpcFlagName),
		NamespaceId: ctx.GlobalString(NamespaceIdFlagName),
		AuthToken:   ctx.GlobalString(AuthTokenFlagName),
		L1RPCURL:    ctx.GlobalString(L1RPCFlagName),
		PrivateKey:  ctx.GlobalString(PrivateKeyFlagName),
	}
}

func NewConfig(l log.Logger, cfg CLIConfig) (Config, error) {

	ctx, cancel := context.WithTimeout(context.Background(), cfg.NetworkTimeout)
	defer cancel()
	l1, err := ethclient.DialContext(ctx, cfg.L1RPCURL)
	if err != nil {
		return Config{}, fmt.Errorf("could not dial eth client: %w", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), cfg.NetworkTimeout)
	defer cancel()
	chainID, err := l1.ChainID(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("could not dial fetch L1 chain ID: %w", err)
	}

	signerFactory, from, err := opcrypto.SignerFactoryFromConfig(l,
		cfg.PrivateKey, "", "", signer.CLIConfig{},
	)
	if err != nil {
		return Config{}, fmt.Errorf("SignerFactoryFromConfig %w", err)
	}

	return Config{
		Backend:        l1,
		ChainID:        chainID,
		Signer:         signerFactory(chainID),
		From:           from,
		NetworkTimeout: cfg.NetworkTimeout,
	}, nil
}

// Config houses parameters for altering the behavior of a SimpleTxManager.
type Config struct {
	Backend ETHBackend
	// ChainID is the chain ID of the L1 chain.
	ChainID *big.Int

	// NetworkTimeout is the allowed duration for a single network request.
	// This is intended to be used for network requests that can be replayed.

	NetworkTimeout time.Duration
	// DaRpc is the HTTP provider URL for the Data Availability node.
	DaRpc string

	// NamespaceId is the id of the namespace of the Data Availability node.
	NamespaceId string

	// AuthToken is the authentication token for the Data Availability node.
	AuthToken string

	// Signer is used to sign transactions when the gas price is increased.
	Signer opcrypto.SignerFn
	From   common.Address
}
