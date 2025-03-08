package main

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

// Receipt represents the results of a transaction.
type L2Receipt struct {
	types.Receipt
	// Optimism: extend receipts with L1 fee info
	L1Fee *big.Int `json:"l1Fee,omitempty"` // Present from pre-bedrock
}

// UnmarshalJSON unmarshals from JSON.
func (r *L2Receipt) UnmarshalJSON(input []byte) error {
	type Receipt struct {
		Type                  *hexutil.Uint64 `json:"type,omitempty"`
		PostState             *hexutil.Bytes  `json:"root"`
		Status                *hexutil.Uint64 `json:"status"`
		CumulativeGasUsed     *hexutil.Uint64 `json:"cumulativeGasUsed" gencodec:"required"`
		Bloom                 *types.Bloom    `json:"logsBloom"         gencodec:"required"`
		Logs                  []*types.Log    `json:"logs"              gencodec:"required"`
		TxHash                *common.Hash    `json:"transactionHash" gencodec:"required"`
		ContractAddress       *common.Address `json:"contractAddress"`
		GasUsed               *hexutil.Uint64 `json:"gasUsed" gencodec:"required"`
		EffectiveGasPrice     *hexutil.Big    `json:"effectiveGasPrice"`
		BlobGasUsed           *hexutil.Uint64 `json:"blobGasUsed,omitempty"`
		BlobGasPrice          *hexutil.Big    `json:"blobGasPrice,omitempty"`
		DepositNonce          *hexutil.Uint64 `json:"depositNonce,omitempty"`
		DepositReceiptVersion *hexutil.Uint64 `json:"depositReceiptVersion,omitempty"`
		BlockHash             *common.Hash    `json:"blockHash,omitempty"`
		BlockNumber           *hexutil.Big    `json:"blockNumber,omitempty"`
		TransactionIndex      *hexutil.Uint   `json:"transactionIndex"`
		L1GasPrice            *hexutil.Big    `json:"l1GasPrice,omitempty"`
		L1BlobBaseFee         *hexutil.Big    `json:"l1BlobBaseFee,omitempty"`
		L1GasUsed             *hexutil.Big    `json:"l1GasUsed,omitempty"`
		L1Fee                 *hexutil.Big    `json:"l1Fee,omitempty"`
		FeeScalar             *big.Float      `json:"l1FeeScalar,omitempty"`
		L1BaseFeeScalar       *hexutil.Uint64 `json:"l1BaseFeeScalar,omitempty"`
		L1BlobBaseFeeScalar   *hexutil.Uint64 `json:"l1BlobBaseFeeScalar,omitempty"`
	}
	var dec Receipt
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Type != nil {
		r.Type = uint8(*dec.Type)
	}
	if dec.PostState != nil {
		r.PostState = *dec.PostState
	}
	if dec.Status != nil {
		r.Status = uint64(*dec.Status)
	}
	if dec.CumulativeGasUsed == nil {
		return errors.New("missing required field 'cumulativeGasUsed' for Receipt")
	}
	r.CumulativeGasUsed = uint64(*dec.CumulativeGasUsed)
	if dec.Bloom == nil {
		return errors.New("missing required field 'logsBloom' for Receipt")
	}
	r.Bloom = *dec.Bloom
	if dec.Logs == nil {
		return errors.New("missing required field 'logs' for Receipt")
	}
	r.Logs = dec.Logs
	if dec.TxHash == nil {
		return errors.New("missing required field 'transactionHash' for Receipt")
	}
	r.TxHash = *dec.TxHash
	if dec.ContractAddress != nil {
		r.ContractAddress = *dec.ContractAddress
	}
	if dec.GasUsed == nil {
		return errors.New("missing required field 'gasUsed' for Receipt")
	}
	r.GasUsed = uint64(*dec.GasUsed)
	if dec.EffectiveGasPrice != nil {
		r.EffectiveGasPrice = (*big.Int)(dec.EffectiveGasPrice)
	}
	if dec.BlobGasUsed != nil {
		r.BlobGasUsed = uint64(*dec.BlobGasUsed)
	}
	if dec.BlobGasPrice != nil {
		r.BlobGasPrice = (*big.Int)(dec.BlobGasPrice)
	}
	if dec.BlockHash != nil {
		r.BlockHash = *dec.BlockHash
	}
	if dec.BlockNumber != nil {
		r.BlockNumber = (*big.Int)(dec.BlockNumber)
	}
	if dec.TransactionIndex != nil {
		r.TransactionIndex = uint(*dec.TransactionIndex)
	}
	if dec.L1Fee != nil {
		r.L1Fee = (*big.Int)(dec.L1Fee)
	}
	return nil
}
