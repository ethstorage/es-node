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
	"github.com/rollkit/celestia-openrpc/types/blob"
)

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

func NewBlobDownloader(l log.Logger, ccfg CLIConfig) (*BlobManager, error) {
	daCfg, err := NewDaConfig(ccfg.DaRpc, ccfg.AuthToken, ccfg.NamespaceId)
	if err != nil {
		return nil, fmt.Errorf("failed to load da config: %w", err)
	}
	return &BlobManager{
		Config: &Config{DaConfig: daCfg},
		l:      l,
	}, nil
}

func (m *BlobManager) GetBlob(ctx context.Context, commitment []byte, height uint64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, m.NetworkTimeout)
	defer cancel()

	m.l.Info("requesting data from celestia", "namespace", hex.EncodeToString(m.Namespace), "height", height)
	blob, err := m.DaClient.Blob.Get(context.Background(), height, m.Namespace, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve frame from celestia: %w", err)
	}
	return blob.Data, nil
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
	err = m.DaClient.Header.SyncWait(ctx)
	if err != nil {
		m.l.Warn("Unable to wait for Celestia header sync", "err", err)
		return nil, 0, err
	}
	m.l.Debug("Start sending blob", "size", len(data))
	height, err := m.DaClient.Blob.Submit(ctx, []*blob.Blob{dataBlob}, nil)
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
		m.l.Debug("Waiting for tx being confirmed...", "hash", txHash)
		if checked > waitTxTimeout {
			m.l.Warn("Waiting for tx confirm timed out", "hash", txHash)
			break
		}
		_, isPending, err := m.Backend.TransactionByHash(context.Background(), txHash)
		if err == nil && !isPending {
			m.l.Debug("Tx confirmed", "hash", txHash)
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
		m.l.Warn("Tx not found!", "err", err, "hash", txHash)
	} else if receipt.Status == 1 {
		m.l.Info("Tx success!", "hash", txHash)
	} else if receipt.Status == 0 {
		m.l.Error("Tx failed!", "hash", txHash)
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
