package blobmgr

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
	openrpc "github.com/rollkit/celestia-openrpc"
	"github.com/rollkit/celestia-openrpc/types/share"
)

const (
	gasBufferRatio = 1.2
	waitTxTimeout  = 25 // seconds
)

type Config struct {
	*DaConfig
	// NetworkTimeout is the allowed duration for a single network request.
	NetworkTimeout time.Duration

	// ETHBackend is the set of methods to submit transactions to L1.
	Backend ETHBackend

	// ChainID is the chain ID of the L1 chain.
	ChainID *big.Int

	// Signer is a function used to sign transactions.
	Signer signer.SignerFn

	// From returns the sending address of the transaction.
	From common.Address

	// L1Contract is the address of the L1 storage contract.
	L1Contract common.Address
}

func NewConfig(l log.Logger, ccfg CLIConfig) (*Config, error) {

	daCfg, err := NewDaConfig(ccfg.DaRpc, ccfg.AuthToken, ccfg.NamespaceId)
	if err != nil {
		return nil, fmt.Errorf("failed to load da config: %w", err)
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
		DaConfig:       daCfg,
		Backend:        l1,
		ChainID:        chainID,
		Signer:         signerFactory(chainID),
		From:           from,
		L1Contract:     common.HexToAddress(ccfg.L1Contract),
		NetworkTimeout: ccfg.NetworkTimeout,
	}, nil
}

type DaConfig struct {
	DaRpc     string
	Namespace share.Namespace
	DaClient  *openrpc.Client
	AuthToken string
}

func NewDaConfig(rpc, token, ns string) (*DaConfig, error) {
	nsBytes, err := hex.DecodeString(ns)
	if err != nil {
		return &DaConfig{}, err
	}

	namespace, err := share.NewBlobNamespaceV0(nsBytes)
	if err != nil {
		return nil, err
	}

	client, err := openrpc.NewClient(context.Background(), rpc, token)
	if err != nil {
		return &DaConfig{}, err
	}

	return &DaConfig{
		Namespace: namespace,
		DaRpc:     rpc,
		DaClient:  client,
	}, nil
}

// ETHBackend is the set of methods that the transaction manager uses to resubmit gas & determine
// when transactions are included on L1.
type ETHBackend interface {
	BlockNumber(ctx context.Context) (uint64, error)
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	TransactionByHash(ctx context.Context, txHash common.Hash) (tx *types.Transaction, isPending bool, err error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
}
