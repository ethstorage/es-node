// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE
package miner

import (
	"context"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/term"
)

var gwei = new(big.Int).Exp(big.NewInt(10), big.NewInt(9), nil)

func Test_l1MiningAPI_checkProfit(t *testing.T) {
	ctx := context.Background()
	unsignedTx := &types.Transaction{}
	rst := result{startShardId: 0, blockNumber: big.NewInt(100)}
	minProfit := new(big.Int)
	tip := gwei
	gasFeeCap := new(big.Int).Mul(big.NewInt(5), gwei)
	l1Fee := new(big.Int).Mul(big.NewInt(25), gwei)
	// rewardL1 := new(big.Int).Mul(big.NewInt(5000000), gwei) // 0.05 eth
	rewardL2 := new(big.Int).Mul(big.NewInt(25), ether) // 25 qkc
	estimatedGas := uint64(500000)
	useL2 := true
	useConfig := false
	lgr := esLog.NewLogger(esLog.CLIConfig{
		Level:  "debug",
		Format: "text",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	})
	mockAPI := &MockMiningAPI{l1MiningAPI: l1MiningAPI{lg: lgr}}

	// Test Case 1: L2 | Using a flexible gas price until only minimal profit remains
	targetGasFeeCap := new(big.Int).SetInt64(49999999950000)
	mockAPI.On("GetL1Fee", ctx, unsignedTx).Return(l1Fee, nil)
	mockAPI.On("GetMiningReward", rst.startShardId, rst.blockNumber.Int64()).Return(rewardL2, nil)
	resultGasFeeCap, err := checkProfit(ctx, mockAPI, unsignedTx, rst, minProfit, gasFeeCap, tip, estimatedGas, useL2, useConfig, mockAPI.lg)
	assert.NoError(t, err)
	assert.Equal(t, targetGasFeeCap, resultGasFeeCap)

}

type MockMiningAPI struct {
	l1MiningAPI
	mock.Mock
}

func (m *MockMiningAPI) GetL1Fee(ctx context.Context, tx *types.Transaction) (*big.Int, error) {
	args := m.Called(ctx, tx)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockMiningAPI) GetMiningReward(shardId uint64, blockNumber int64) (*big.Int, error) {
	args := m.Called(shardId, blockNumber)
	return args.Get(0).(*big.Int), args.Error(1)
}
