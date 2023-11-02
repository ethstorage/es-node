package blobmgr

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
	openrpc "github.com/rollkit/celestia-openrpc"
	"github.com/rollkit/celestia-openrpc/types/blob"

	"github.com/rollkit/celestia-openrpc/types/share"
)

const (
	gasBufferRatio = 1.2
	waitTxTimeout  = 25 // seconds
)

type Config struct {

	// NetworkTimeout is the allowed duration for a single network request.
	NetworkTimeout time.Duration

	//
	DAclient *openrpc.Client

	// Namespace is the id of the namespace of the Data Availability node.
	Namespace share.Namespace

	// AuthToken is the authentication token for the Data Availability node.
	AuthToken string

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

type BlobManager struct {
	*Config
	l log.Logger
}

func NewBlobManager(l log.Logger, ccfg CLIConfig) (*BlobManager, error) {
	conf, err := NewConfig(l, ccfg)
	if err != nil {
		return nil, err
	}
	return &BlobManager{
		Config: conf,
		l:      l,
	}, nil
}

func (m *BlobManager) SendBlob(ctx context.Context, data []byte) ([]byte, uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, m.NetworkTimeout)
	defer cancel()

	dataBlob, err := blob.NewBlobV0(m.Namespace.ToAppNamespace().Bytes(), data)
	com, err := blob.CreateCommitment(dataBlob)
	if err != nil {
		m.l.Warn("Unable to create blob commitment to Celestia", "err", err)
		return nil, 0, err
	}
	m.l.Debug("Create commitment", "commitment", hex.EncodeToString(com))
	err = m.DAclient.Header.SyncWait(ctx)
	if err != nil {
		m.l.Warn("Unable to wait for Celestia header sync", "err", err)
		return nil, 0, err
	}
	m.l.Debug("Start sending blob", "size", len(data))
	height, err := m.DAclient.Blob.Submit(ctx, []*blob.Blob{dataBlob}, nil)
	if err != nil {
		m.l.Warn("Unable to publish tx to Celestia", "err", err)
		return nil, 0, err
	}
	if height == 0 {
		m.l.Warn("unexpected response from Celestia got", "height", height)
		return nil, 0, errors.New("Unexpected response code")
	}
	m.l.Debug("Submit blob to Celestia successfully!", "height", height)
	return com, height, nil
}

func (m *BlobManager) PublishBlob(ctx context.Context, key, commit []byte, size, height uint64, ethValue *big.Int) error {
	m.l.Debug("Create tx for Ethereum", "commitment", hex.EncodeToString(commit), "height", height, "size", size, "value", ethValue)
	bytes32Type, _ := abi.NewType("bytes32", "", nil)
	uint256Type, _ := abi.NewType("uint256", "", nil)
	dataField, _ := abi.Arguments{
		{Type: bytes32Type},
		{Type: bytes32Type},
		{Type: uint256Type},
		{Type: uint256Type},
	}.Pack(
		key,
		commit,
		new(big.Int).SetUint64(size),
		new(big.Int).SetUint64(height),
	)
	h := crypto.Keccak256Hash([]byte(`putBlob(bytes32,bytes32,uint256,uint256)`))
	calldata := append(h[0:4], dataField...)

	nonce, err := m.Backend.NonceAt(ctx, m.From, big.NewInt(rpc.LatestBlockNumber.Int64()))
	if err != nil {
		m.l.Error("Query nonce failed", "error", err.Error())
		return err
	}
	m.l.Debug("Query nonce done", "nonce", nonce)

	gasTipCap, basefee, err := m.suggestGasPriceCaps(ctx)
	if err != nil {
		return fmt.Errorf("failed to get gas price info: %w", err)
	}
	gasFeeCap := calcGasFeeCap(basefee, gasTipCap)
	m.l.Debug("Query gas price done", "tip", gasTipCap, "feeCap", gasFeeCap)

	estimatedGas, err := m.Backend.EstimateGas(ctx, ethereum.CallMsg{
		From:      m.From,
		To:        &m.L1Contract,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		Data:      calldata,
	})
	if err != nil {
		m.l.Error("Estimate gas failed", "error", err.Error())
		return fmt.Errorf("failed to estimate gas: %w", err)
	}
	m.l.Debug("Estimated gas done", "gas", estimatedGas)
	gas := uint64(float64(estimatedGas) * gasBufferRatio)

	rawTx := &types.DynamicFeeTx{
		ChainID:   m.ChainID,
		Nonce:     nonce,
		To:        &m.L1Contract,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Gas:       gas,
		Data:      calldata,
		Value:     ethValue,
	}

	signedTx, err := m.Signer(ctx, m.From, types.NewTx(rawTx))
	if err != nil {
		m.l.Error("Sign tx error", "error", err)
		return err
	}
	txHash := signedTx.Hash()
	fmt.Println("txHash:", txHash.Hex())
	m.l.Debug("Signed tx done", "hash", txHash)
	err = m.Backend.SendTransaction(ctx, signedTx)
	if err != nil {
		m.l.Error("Send tx failed", "hash", txHash, "error", err)
		return err
	}
	m.l.Info("Send tx successfully", "hash", txHash)

	// waiting for tx confirmation or timeout
	ticker := time.NewTicker(1 * time.Second)
	checked := 0
	for range ticker.C {
		if checked > waitTxTimeout {
			log.Warn("Waiting for tx confirm timed out", "hash", txHash)
			break
		}
		_, isPending, err := m.Backend.TransactionByHash(context.Background(), txHash)
		if err == nil && !isPending {
			log.Debug("Tx confirmed", "hash", txHash)
			m.checkTxStatus(txHash)
			break
		}
		checked++
	}
	ticker.Stop()
	return nil
}

// suggestGasPriceCaps suggests what the new tip & new basefee should be based on the current L1 conditions
func (m *BlobManager) suggestGasPriceCaps(ctx context.Context) (*big.Int, *big.Int, error) {
	cCtx, cancel := context.WithTimeout(ctx, m.NetworkTimeout)
	defer cancel()
	tip, err := m.Backend.SuggestGasTipCap(cCtx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch the suggested gas tip cap: %w", err)
	} else if tip == nil {
		return nil, nil, errors.New("the suggested tip was nil")
	}
	cCtx, cancel = context.WithTimeout(ctx, m.NetworkTimeout)
	defer cancel()
	head, err := m.Backend.HeaderByNumber(cCtx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch the suggested basefee: %w", err)
	} else if head.BaseFee == nil {
		return nil, nil, errors.New("txmgr does not support pre-london blocks that do not have a basefee")
	}
	return tip, head.BaseFee, nil
}

func (m *BlobManager) checkTxStatus(txHash common.Hash) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	receipt, err := m.Backend.TransactionReceipt(ctx, txHash)
	if err != nil || receipt == nil {
		log.Warn("Tx not found!", "err", err, "hash", txHash)
	} else if receipt.Status == 1 {
		log.Info("Tx success!", "hash", txHash)
	} else if receipt.Status == 0 {
		log.Error("Tx failed!", "hash", txHash)
	}
}

// calcGasFeeCap deterministically computes the recommended gas fee cap given
// the base fee and gasTipCap. The resulting gasFeeCap is equal to:
//
//	gasTipCap + 2*baseFee.
func calcGasFeeCap(baseFee, gasTipCap *big.Int) *big.Int {
	return new(big.Int).Add(
		gasTipCap,
		new(big.Int).Mul(baseFee, big.NewInt(2)),
	)
}
