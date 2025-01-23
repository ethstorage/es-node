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

func Test_l1MiningAPI_checkGasPrice(t *testing.T) {
	unsignedTx := &types.Transaction{}
	mockResult := result{
		startShardId: 0,
		blockNumber:  big.NewInt(100),
	}
	lgr := esLog.NewLogger(esLog.CLIConfig{
		Level:  "debug",
		Format: "text",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	})

	testCases := []struct {
		name          string
		l1Fee         *big.Int
		l1FeeErr      error
		reward        *big.Int
		rewardErr     error
		minProfit     *big.Int
		gasFeeCap     *big.Int
		tip           *big.Int
		estimatedGas  uint64
		safeGas       uint64
		useL2         bool
		useConfig     bool
		wantError     bool
		wantGasFeeCap *big.Int
	}{
		{
			name:          "Case1 - SWC Beta: relax the gas price until minimal profit remains",
			l1Fee:         new(big.Int).Mul(big.NewInt(25), gwei),
			l1FeeErr:      nil,
			reward:        new(big.Int).Mul(big.NewInt(25), ether),
			rewardErr:     nil,
			minProfit:     big.NewInt(0),
			gasFeeCap:     big.NewInt(1000251),
			tip:           big.NewInt(1000000),
			estimatedGas:  uint64(500000),
			safeGas:       uint64(600000),
			useL2:         true,
			useConfig:     false,
			wantError:     false,
			wantGasFeeCap: new(big.Int).SetInt64(49999999950000),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mockAPI := &MockMiningAPI{l1MiningAPI: l1MiningAPI{lg: lgr}}
			mockAPI.On("GetL1Fee", ctx, unsignedTx).Return(tc.l1Fee, tc.l1FeeErr)
			mockAPI.On("GetMiningReward", mockResult.startShardId, mockResult.blockNumber.Int64()).Return(tc.reward, tc.rewardErr)

			gotGasFeeCap, gotErr := checkGasPrice(
				ctx,
				mockAPI,
				unsignedTx,
				mockResult,
				tc.minProfit,
				tc.gasFeeCap,
				tc.tip,
				tc.estimatedGas,
				tc.safeGas,
				tc.useL2,
				tc.useConfig,
				lgr,
			)

			if tc.wantError {
				assert.Error(t, gotErr, "expected an error but did not get one")
				assert.Nil(t, gotGasFeeCap, "gasFeeCap should be nil if error occurs")
			} else {
				assert.NoError(t, gotErr, "did not expect an error, but got one")
				assert.Equal(t, tc.wantGasFeeCap, gotGasFeeCap, "unexpected final gasFeeCap")
			}
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

func (m *MockMiningAPI) GetMiningReward(shardId uint64, blockNumber int64) (*big.Int, error) {
	args := m.Called(shardId, blockNumber)
	return args.Get(0).(*big.Int), args.Error(1)
}
