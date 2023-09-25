package signer

import (
	"context"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
)

const l1Endpoint = "http://65.108.236.27:8545"

func TestSignerFactoryFromConfig(t *testing.T) {
	tests := []struct {
		name         string
		signerConfig CLIConfig
		addrFrom     common.Address
		addrTo       common.Address
		wantErr      bool
	}{
		{
			"",
			CLIConfig{
				PrivateKey: "0xaaa279031ebf27a046278a8ff5d1b8ab77362d19ace646653b76a7c1184516ef",
				Mnemonic:   "",
				HDPath:     "",
			},
			common.HexToAddress("0x9ce0b38e90cd2a0c82409486078af83c790dff20"),
			common.HexToAddress("0x188aac000e21ec314C5694bB82035b72210315A8"),
			false,
		},
		{
			"",
			CLIConfig{
				PrivateKey: "",
				Mnemonic:   "candy maple cake sugar pudding cream honey rich smooth crumble sweet treat",
				HDPath:     "m/44'/60'/0'/0/0",
			},
			common.HexToAddress("0x627306090abab3a6e1400e9345bc60c78a8bef57"),
			common.HexToAddress("0x188aac000e21ec314C5694bB82035b72210315A8"),
			false,
		},
		{
			"",
			CLIConfig{
				PrivateKey: "",
				Mnemonic:   "",
				HDPath:     "",
				Endpoint:   "http://65.108.236.27:8550",
				Address:    "0x13259366de990b0431e2c97cea949362bb68df12",
			},
			common.HexToAddress("0x13259366DE990B0431E2C97CEa949362BB68df12"),
			common.HexToAddress("0x188aac000e21ec314C5694bB82035b72210315A8"),
			false,
		},
		{
			"",
			CLIConfig{
				PrivateKey: "",
				Mnemonic:   "",
				HDPath:     "",
				Endpoint:   "http://65.108.236.27:8550",
				Address:    "0x13259366de990b0431e2c97cea949362bb68df12",
			},
			common.HexToAddress("0x13259366de990b0431e2c97cea949362bb68df12"),
			common.HexToAddress("0x0000000000000000000000000000000000001234"), // only allowed to send tx to 0x188aac000e21ec314C5694bB82035b72210315A8
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, addrFrom, err := SignerFactoryFromConfig(tt.signerConfig)
			if err != nil {
				t.Fatal("SignerFactoryFromConfig err", err)
			}
			if !reflect.DeepEqual(addrFrom, tt.addrFrom) {
				t.Errorf("SignerFactoryFromConfig() addrFrom = %x, want %x", addrFrom, tt.addrFrom)
				return
			}
			ctx := context.Background()
			client, err := ethclient.DialContext(ctx, l1Endpoint)
			if err != nil {
				t.Fatal("Failed to connect to the Ethereum client", err)
			}
			chainID, err := client.NetworkID(ctx)
			if err != nil {
				t.Fatal("Failed to get network ID: ", err)
			}
			sign := factory(chainID)
			nonce, err := client.PendingNonceAt(ctx, addrFrom)
			if err != nil {
				t.Fatal("Failed to get nonce", err)
			}
			tx := types.NewTx(&types.DynamicFeeTx{
				ChainID:   chainID,
				Gas:       21000,
				Nonce:     nonce,
				To:        &tt.addrTo,
				Value:     big.NewInt(1),
				GasFeeCap: new(big.Int).Mul(big.NewInt(5), big.NewInt(params.GWei)),
				GasTipCap: big.NewInt(2),
			})
			signedTx, err := sign(ctx, addrFrom, tx)
			if (err != nil) != tt.wantErr {
				t.Errorf("sign error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				err = client.SendTransaction(ctx, signedTx)
				if err != nil {
					t.Fatal("Failed to send tx", err, addrFrom)
				}
				t.Log("Submit tx", signedTx.Hash().Hex())
			}
		})
	}
}
