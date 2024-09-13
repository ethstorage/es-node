// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"fmt"
	"math/big"
	"testing"
)

func Test_expectedDiff(t *testing.T) {
	type args struct {
		interval       uint64
		difficulty     *big.Int
		cutoff         *big.Int
		diffAdjDivisor *big.Int
		minDiff        *big.Int
	}
	tests := []struct {
		name string
		args args
		want *big.Int
	}{
		{
			"decrease: diff not match case 1",
			args{
				1724219196 - 1724218692,
				big.NewInt(16933920),
				big.NewInt(7200),
				big.NewInt(32),
				big.NewInt(9437184),
			},
			big.NewInt(17463105),
		},
		{
			"decrease: diff not match case 2",
			args{
				1724243184 - 1724242128,
				big.NewInt(71922272),
				big.NewInt(7200),
				big.NewInt(32),
				big.NewInt(9437184),
			},
			big.NewInt(74169843),
		},
		{
			"no change",
			args{
				1723277844 - 1723267236,
				big.NewInt(25034417035),
				big.NewInt(7200),
				big.NewInt(32),
				big.NewInt(4718592000),
			},
			big.NewInt(25034417035),
		},
		{
			"increase",
			args{
				1724507676 - 1724507412,
				big.NewInt(18788404245),
				big.NewInt(7200),
				big.NewInt(32),
				big.NewInt(4718592000),
			},
			big.NewInt(19375541877),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Println("tt.args.interval", tt.args.interval)
			if got := expectedDiff(tt.args.interval, tt.args.difficulty, tt.args.cutoff, tt.args.diffAdjDivisor, tt.args.minDiff); tt.want.Cmp(got) != 0 {
				t.Errorf("got  = %v, want %v", got, tt.want)
			}
		})
	}
}
