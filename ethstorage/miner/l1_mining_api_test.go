// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE
package miner

import (
	"context"
	"fmt"
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

func Test_l1MiningAPI_checkGasPrice(t *testing.T) {
	unsignedTx := &types.Transaction{}
	mockResult := result{
		startShardId: 0,
		timestamp:    0,
	}
	lgr := esLog.NewLogger(esLog.CLIConfig{
		Level:  "debug",
		Format: "text",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	})

	estimatedGas := uint64(500000)
	safeGas := uint64(600000)

	testCases := []struct {
		name string

		// params
		minProfit *big.Int
		gasFeeCap *big.Int
		tip       *big.Int
		useL2     bool
		useConfig bool

		// mocked results
		l1Fee     *big.Int
		l1FeeErr  error
		reward    *big.Int
		rewardErr error

		// test results
		wantDropped   bool
		wantGasFeeCap *big.Int
	}{
		{
			name:          "SWC Beta: relax the gas price until minimal profit remains",
			minProfit:     big.NewInt(0),
			gasFeeCap:     big.NewInt(1000251),
			tip:           big.NewInt(1000000),
			useL2:         true,
			useConfig:     false,
			l1Fee:         new(big.Int).Mul(big.NewInt(25), gwei),
			l1FeeErr:      nil,
			reward:        new(big.Int).Mul(big.NewInt(25), ether),
			rewardErr:     nil,
			wantDropped:   false,
			wantGasFeeCap: new(big.Int).SetInt64(49999999950000),
		},
		{
			name:          "SWC Beta: relax the gas price until minimal profit remains, but cut by tx fee cap",
			minProfit:     big.NewInt(0),
			gasFeeCap:     big.NewInt(1000251),
			tip:           big.NewInt(1000000),
			useL2:         true,
			useConfig:     false,
			l1Fee:         new(big.Int).Mul(big.NewInt(25), gwei),
			l1FeeErr:      nil,
			reward:        new(big.Int).Mul(big.NewInt(1000), ether),
			rewardErr:     nil,
			wantDropped:   false,
			wantGasFeeCap: new(big.Int).SetInt64(1666666666666666),
		},
		{
			name:          "SWC Beta: tx dropped due to low reward",
			minProfit:     big.NewInt(0),
			gasFeeCap:     big.NewInt(1000251),
			tip:           big.NewInt(1000000),
			useL2:         true,
			useConfig:     false,
			l1Fee:         new(big.Int).Mul(big.NewInt(25), gwei),
			l1FeeErr:      nil,
			reward:        new(big.Int).Mul(big.NewInt(250), gwei),
			rewardErr:     nil,
			wantDropped:   true,
			wantGasFeeCap: nil,
		},
		{
			name:          "SWC Beta: tx dropped due to high minimum profit expected",
			minProfit:     new(big.Int).Mul(big.NewInt(25), ether),
			gasFeeCap:     big.NewInt(1000251),
			tip:           big.NewInt(1000000),
			useL2:         true,
			useConfig:     false,
			l1Fee:         new(big.Int).Mul(big.NewInt(25), gwei),
			l1FeeErr:      nil,
			reward:        new(big.Int).Mul(big.NewInt(25), ether),
			rewardErr:     nil,
			wantDropped:   true,
			wantGasFeeCap: nil,
		},
		{
			name:          "SWC Beta: tx dropped due to high gas price",
			minProfit:     big.NewInt(0),
			gasFeeCap:     new(big.Int).Mul(big.NewInt(250000), gwei),
			tip:           big.NewInt(1000000),
			useL2:         true,
			useConfig:     false,
			l1Fee:         new(big.Int).Mul(big.NewInt(25), gwei),
			l1FeeErr:      nil,
			reward:        new(big.Int).Mul(big.NewInt(25), ether),
			rewardErr:     nil,
			wantDropped:   true,
			wantGasFeeCap: nil,
		},
		{
			name:          "SWC Beta: tx dropped due to high l1 data fee",
			minProfit:     big.NewInt(0),
			gasFeeCap:     big.NewInt(1000251),
			tip:           big.NewInt(1000000),
			useL2:         true,
			useConfig:     false,
			l1Fee:         new(big.Int).Mul(big.NewInt(25), ether),
			l1FeeErr:      nil,
			reward:        new(big.Int).Mul(big.NewInt(25), ether),
			rewardErr:     nil,
			wantDropped:   true,
			wantGasFeeCap: nil,
		},
		{
			name:          "SWC Beta: failed to get l1 data fee",
			minProfit:     big.NewInt(0),
			gasFeeCap:     big.NewInt(1000251),
			tip:           big.NewInt(1000000),
			useL2:         true,
			useConfig:     false,
			l1Fee:         nil,
			l1FeeErr:      fmt.Errorf("l1fee error"),
			reward:        new(big.Int).Mul(big.NewInt(25), ether),
			rewardErr:     nil,
			wantDropped:   false,
			wantGasFeeCap: big.NewInt(50000000000000),
		},

		{
			name:          "SWC Beta: unable to get reward; use marketable gas price",
			minProfit:     big.NewInt(0),
			gasFeeCap:     big.NewInt(1000251),
			tip:           big.NewInt(1000000),
			useL2:         true,
			useConfig:     false,
			l1Fee:         new(big.Int).Mul(big.NewInt(25), gwei),
			l1FeeErr:      nil,
			reward:        nil,
			rewardErr:     fmt.Errorf("reward error"),
			wantDropped:   false,
			wantGasFeeCap: big.NewInt(1000502),
		},
		{
			name:          "SWC Beta: use configured gas price",
			minProfit:     big.NewInt(0),
			gasFeeCap:     big.NewInt(10002510),
			tip:           big.NewInt(1000000),
			useL2:         true,
			useConfig:     true,
			l1Fee:         new(big.Int).Mul(big.NewInt(25), gwei),
			l1FeeErr:      nil,
			reward:        new(big.Int).Mul(big.NewInt(25), ether),
			rewardErr:     nil,
			wantDropped:   false,
			wantGasFeeCap: new(big.Int).SetInt64(10002510),
		},
		{
			name:          "SWC Beta: use configured gas price, but dropped",
			minProfit:     big.NewInt(0),
			gasFeeCap:     new(big.Int).Mul(big.NewInt(1), ether),
			tip:           big.NewInt(1000000),
			useL2:         true,
			useConfig:     true,
			l1Fee:         new(big.Int).Mul(big.NewInt(25), gwei),
			l1FeeErr:      nil,
			reward:        new(big.Int).Mul(big.NewInt(25), ether),
			rewardErr:     nil,
			wantDropped:   true,
			wantGasFeeCap: nil,
		},
		{
			name:          "Sepolia: relax the gas price until minimal profit remains",
			minProfit:     big.NewInt(0),
			gasFeeCap:     new(big.Int).Mul(big.NewInt(5), gwei),
			tip:           new(big.Int).Mul(big.NewInt(1), gwei),
			useL2:         false,
			useConfig:     false,
			l1Fee:         nil,
			l1FeeErr:      nil,
			reward:        new(big.Int).Exp(big.NewInt(10), big.NewInt(17), nil), // 0.1 eth
			rewardErr:     nil,
			wantDropped:   false,
			wantGasFeeCap: new(big.Int).Mul(big.NewInt(200), gwei),
		},
		{
			name:          "Sepolia: relax the gas price until minimal profit remains, but cut by tx fee cap",
			minProfit:     big.NewInt(0),
			gasFeeCap:     new(big.Int).Mul(big.NewInt(5), gwei),
			tip:           new(big.Int).Mul(big.NewInt(1), gwei),
			useL2:         false,
			useConfig:     false,
			l1Fee:         nil,
			l1FeeErr:      nil,
			reward:        new(big.Int).Mul(big.NewInt(1), ether),
			rewardErr:     nil,
			wantDropped:   false,
			wantGasFeeCap: new(big.Int).SetInt64(1666666666666),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mockAPI := &MockMiningAPI{l1MiningAPI: l1MiningAPI{lg: lgr}}
			mockAPI.On("GetL1Fee", ctx, unsignedTx).Return(tc.l1Fee, tc.l1FeeErr)
			mockAPI.On("GetMiningReward", mockResult.startShardId, mockResult.timestamp).Return(tc.reward, tc.rewardErr)

			gotGasFeeCap, gotErr := checkGasPrice(
				ctx,
				mockAPI,
				unsignedTx,
				mockResult,
				tc.minProfit,
				tc.gasFeeCap,
				tc.tip,
				estimatedGas,
				safeGas,
				tc.useL2,
				tc.useConfig,
				lgr,
			)
			if tc.wantDropped {
				assert.ErrorIs(t, gotErr, errDropped, "expected an dropped error, but got none")
			} else {
				assert.NoError(t, gotErr, "unexpected error")
			}
			assert.Equal(t, tc.wantGasFeeCap, gotGasFeeCap, "unexpected final gasFeeCap")
		})
	}
}

type MockMiningAPI struct {
	l1MiningAPI
	mock.Mock
}

func (m *MockMiningAPI) GetL1Fee(ctx context.Context, tx *types.Transaction) (*big.Int, error) {
	args := m.Called(ctx, tx)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockMiningAPI) GetMiningReward(shardId uint64, timestamp uint64) (*big.Int, error) {
	args := m.Called(shardId, timestamp)
	return args.Get(0).(*big.Int), args.Error(1)
}
