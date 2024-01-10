// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package integration

import (
	"context"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
)

var (
	clefContract = common.HexToAddress(os.Getenv("ES_NODE_STORAGE_L1CONTRACT_CLEF"))
	clefEndpoint = os.Getenv("ES_NODE_CLEF_RPC")
)

func TestSignerFactoryFromConfig(t *testing.T) {
	t.SkipNow()
	tests := []struct {
		name         string
		signerConfig signer.CLIConfig
		addrFrom     common.Address
		addrTo       common.Address
		wantErr      bool
	}{
		{
			"",
			signer.CLIConfig{
				PrivateKey: "0x7fb8f46cff75dd565c23e83f3c5aa2693f39cb6a5ede0120666af139cded39af",
				Mnemonic:   "",
				HDPath:     "",
			},
			common.HexToAddress("0xd7cc258C5438a392cA1D7873020d5B9971568c00"),
			clefContract,
			false,
		},
		{
			"",
			signer.CLIConfig{
				PrivateKey: "",
				Mnemonic:   "candy maple cake sugar pudding cream honey rich smooth crumble sweet treat",
				HDPath:     "m/44'/60'/0'/0/0",
			},
			common.HexToAddress("0x627306090abab3a6e1400e9345bc60c78a8bef57"),
			clefContract,
			false,
		},
		{
			"",
			signer.CLIConfig{
				PrivateKey: "",
				Mnemonic:   "",
				HDPath:     "",
				Endpoint:   clefEndpoint,
				Address:    "0x13259366de990b0431e2c97cea949362bb68df12",
			},
			common.HexToAddress("0x13259366DE990B0431E2C97CEa949362BB68df12"),
			clefContract,
			false,
		},
		{
			"",
			signer.CLIConfig{
				PrivateKey: "",
				Mnemonic:   "",
				HDPath:     "",
				Endpoint:   clefEndpoint,
				Address:    "0x13259366de990b0431e2c97cea949362bb68df12",
			},
			common.HexToAddress("0x13259366de990b0431e2c97cea949362bb68df12"),
			common.HexToAddress("0x0000000000000000000000000000000000001234"), // only allowed to send tx to contractAddrDevnet1
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, addrFrom, err := signer.SignerFactoryFromConfig(tt.signerConfig)
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
