// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"context"
	"fmt"
	"math/big"
	"time"

	opcrypto "github.com/ethereum-optimism/optimism/op-service/crypto"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
)

func newTxMgrConfig(l1Addr string, signerFactory opcrypto.SignerFactory) (txmgr.Config, error) {
	cfg := txmgr.CLIConfig{
		L1RPCURL: l1Addr,
		// Number of confirmations which we will wait after sending a transaction
		NumConfirmations: uint64(3),
		// Number of ErrNonceTooLow observations required to give up on a tx at a particular nonce without receiving confirmation
		SafeAbortNonceTooLowCount: uint64(3),
		// The multiplier applied to fee suggestions to put a hard limit on fee increases
		FeeLimitMultiplier: uint64(5),
		// Minimum threshold (in Wei) at which the FeeLimitMultiplier takes effect.
		FeeLimitThresholdGwei: 100.0,
		// Duration we will wait before resubmitting a transaction to L1
		ResubmissionTimeout: 48 * time.Second,
		// NetworkTimeout is the allowed duration for a single network request.
		NetworkTimeout: 10 * time.Second,
		// Timeout for aborting a tx send if the tx does not make it to the mempool.
		TxNotInMempoolTimeout: 2 * time.Minute,
		// Frequency to poll for receipts
		ReceiptQueryInterval: 12 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.NetworkTimeout)
	defer cancel()
	l1, err := ethclient.DialContext(ctx, cfg.L1RPCURL)
	if err != nil {
		return txmgr.Config{}, fmt.Errorf("could not dial eth client: %w", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), cfg.NetworkTimeout)
	defer cancel()
	chainID, err := l1.ChainID(ctx)
	if err != nil {
		return txmgr.Config{}, fmt.Errorf("could not dial fetch L1 chain ID: %w", err)
	}

	// convert float GWei value into integer Wei value
	feeLimitThreshold, _ := new(big.Float).Mul(
		big.NewFloat(cfg.FeeLimitThresholdGwei),
		big.NewFloat(params.GWei)).
		Int(nil)

	return txmgr.Config{
		Backend:                   l1,
		ResubmissionTimeout:       cfg.ResubmissionTimeout,
		FeeLimitMultiplier:        cfg.FeeLimitMultiplier,
		FeeLimitThreshold:         feeLimitThreshold,
		ChainID:                   chainID,
		TxSendTimeout:             cfg.TxSendTimeout,
		TxNotInMempoolTimeout:     cfg.TxNotInMempoolTimeout,
		NetworkTimeout:            cfg.NetworkTimeout,
		ReceiptQueryInterval:      cfg.ReceiptQueryInterval,
		NumConfirmations:          cfg.NumConfirmations,
		SafeAbortNonceTooLowCount: cfg.SafeAbortNonceTooLowCount,
		Signer:                    signerFactory(chainID),
	}, nil
}

// https://github.com/ethereum/go-ethereum/issues/21221#issuecomment-805852059
func weiToEther(wei *big.Int) *big.Float {
	f := new(big.Float)
	f.SetPrec(236) //  IEEE 754 octuple-precision binary floating-point format: binary256
	f.SetMode(big.ToNearestEven)
	if wei == nil {
		return f.SetInt64(0)
	}
	fWei := new(big.Float)
	fWei.SetPrec(236) //  IEEE 754 octuple-precision binary floating-point format: binary256
	fWei.SetMode(big.ToNearestEven)
	return f.Quo(fWei.SetInt(wei), big.NewFloat(params.Ether))
}
