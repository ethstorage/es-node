package blobmgr

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
	openrpc "github.com/rollkit/celestia-openrpc"
	"github.com/rollkit/celestia-openrpc/types/share"
	"github.com/urfave/cli"
)

const (
	BlobFileFlagName       = "blob-file"
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
			Name:  BlobFileFlagName,
			Usage: "File path to read blob data",
		},
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
	BlobFilePath   string
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
		BlobFilePath:   ctx.String(BlobFileFlagName),
		DaRpc:          ctx.String(DaRpcFlagName),
		NamespaceId:    ctx.String(NamespaceIdFlagName),
		AuthToken:      ctx.String(AuthTokenFlagName),
		L1RPCURL:       ctx.String(L1RPCFlagName),
		L1Contract:     ctx.String(L1ContractFlagName),
		PrivateKey:     ctx.String(PrivateKeyFlagName),
		NetworkTimeout: ctx.Duration(NetworkTimeoutFlagName),
	}
}

func NewConfig(l log.Logger, ccfg CLIConfig) (*Config, error) {
	daClient, err := openrpc.NewClient(context.Background(), ccfg.DaRpc, ccfg.AuthToken)
	if err != nil {
		l.Error("Init DA client failed", "rpc", ccfg.DaRpc, "err", err)
		return nil, err
	}
	nsBytes, err := hex.DecodeString(ccfg.NamespaceId)
	if err != nil {
		return nil, err
	}
	namespace, err := share.NewBlobNamespaceV0(nsBytes)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), ccfg.NetworkTimeout)
	defer cancel()
	l1, err := ethclient.DialContext(ctx, ccfg.L1RPCURL)
	if err != nil {
		return nil, fmt.Errorf("could not dial eth client: %w", err)
	}
	chainID, err := l1.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not dial fetch L1 chain ID: %w", err)
	}
	signerFactory, from, err := signer.SignerFactoryFromConfig(
		signer.CLIConfig{PrivateKey: ccfg.PrivateKey},
	)
	if err != nil {
		return nil, fmt.Errorf("could not init signer %w", err)
	}
	return &Config{
		DAclient:       daClient,
		Namespace:      namespace,
		Backend:        l1,
		ChainID:        chainID,
		Signer:         signerFactory(chainID),
		From:           from,
		L1Contract:     common.HexToAddress(ccfg.L1Contract),
		NetworkTimeout: ccfg.NetworkTimeout,
	}, nil
}
