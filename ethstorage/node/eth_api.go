// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package node

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)

type ethAPI struct {
	rpcCfg *RPCConfig
	log    log.Logger
}

const (
	ESChainID = 333
	defaultCallTimeout = 2 * time.Second
)

var (
	rpcCli *rpc.Client
)


func NewETHAPI(config *RPCConfig, log log.Logger) *ethAPI {
	return &ethAPI{
		rpcCfg: config,
		log:    log,
	}
}

func (api *ethAPI) ChainId() hexutil.Uint64 {
	return hexutil.Uint64(ESChainID)
}

type TransactionArgs struct {
	From                 *common.Address `json:"from"`
	To                   *common.Address `json:"to"`
	Gas                  *hexutil.Uint64 `json:"gas"`
	GasPrice             *hexutil.Big    `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big    `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big    `json:"maxPriorityFeePerGas"`
	Value                *hexutil.Big    `json:"value"`
	Nonce                *hexutil.Uint64 `json:"nonce"`

	// We accept "data" and "input" for backwards-compatibility reasons.
	// "input" is the newer name and should be preferred by clients.
	// Issue detail: https://github.com/ethereum/go-ethereum/issues/15628
	Data  *hexutil.Bytes `json:"data"`
	Input *hexutil.Bytes `json:"input"`

	// Introduced by AccessListTxType transaction.
	AccessList *types.AccessList `json:"accessList,omitempty"`
	ChainID    *hexutil.Big      `json:"chainId,omitempty"`
}

type OverrideAccount struct {
	Nonce     *hexutil.Uint64              `json:"nonce"`
	Code      *hexutil.Bytes               `json:"code"`
	Balance   **hexutil.Big                `json:"balance"`
	State     *map[common.Hash]common.Hash `json:"state"`
	StateDiff *map[common.Hash]common.Hash `json:"stateDiff"`
}

// StateOverride is the collection of overridden accounts.
type StateOverride map[common.Address]OverrideAccount

// BlockOverrides is a set of header fields to override.
type BlockOverrides struct {
	Number     *hexutil.Big
	Difficulty *hexutil.Big
	Time       *hexutil.Uint64
	GasLimit   *hexutil.Uint64
	Coinbase   *common.Address
	Random     *common.Hash
	BaseFee    *hexutil.Big
}

func (api *ethAPI) Call(ctx context.Context, args TransactionArgs, blockNrOrHash rpc.BlockNumberOrHash, overrides *StateOverride, blockOverrides *BlockOverrides) (hexutil.Bytes, error) {
	var err error
	if rpcCli == nil {
		dialCtx, cancel := context.WithTimeout(ctx, defaultCallTimeout)
		rpcCli, err = rpc.DialContext(dialCtx, api.rpcCfg.ESCallURL)
		cancel()
		if err != nil {
			return []byte{}, err
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, defaultCallTimeout)
	defer cancel()
	var hex hexutil.Bytes
	err = rpcCli.CallContext(
		callCtx, 
		&hex, 
		"eth_esCall", 
		args, blockNrOrHash)
	return hex, err
}
