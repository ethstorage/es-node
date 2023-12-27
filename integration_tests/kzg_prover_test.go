// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package integration

import (
	"bytes"
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
)

func TestKZGProver_GenerateKZGProof(t *testing.T) {
	dataRaw := generateRandomContent(128)
	dataHash := uploadBlob(t, dataRaw)
	blobs := utils.EncodeBlobs(dataRaw)
	blob := blobs[0][:]
	tests := []struct {
		name     string
		chunkIdx uint64
	}{
		{"check 0 th element",
			0,
		},
		{"check 235 th element",
			235,
		},
		{"check 3293 th element",
			3293,
		},
	}
	p := prover.NewKZGProver(lg)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peInput, err := p.GenerateKZGProof(blob, tt.chunkIdx)
			if err != nil {
				t.Errorf("KZGProver.GenerateKZGProof() error = %v", err)
				return
			}
			if !bytes.Equal(peInput[0:24], dataHash[:24]) {
				t.Errorf("dataHash not correct: off-chain %v, on-chain %v", peInput[0:24], dataHash[:24])
				return
			}
			err = verifyInclusive(tt.chunkIdx, peInput)
			if err != nil {
				t.Errorf("verifyInclusive() error = %v", err)
				return
			}
		})
	}
}

func uploadBlob(t *testing.T, data []byte) common.Hash {

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	client, err := ethclient.DialContext(ctx, l1Endpoint)
	if err != nil {
		t.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	defer client.Close()

	chainID, err := client.ChainID(ctx)
	if err != nil {
		t.Fatalf("Get chain id failed %v", err)
	}
	key, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		t.Fatalf("Invalid private key: %s, err: %v", privateKey, err)
	}
	signer := crypto.PubkeyToAddress(key.PublicKey)
	lg.Info("Get signer address", "signer", signer.Hex())
	n, err := client.NonceAt(ctx, signer, big.NewInt(rpc.LatestBlockNumber.Int64()))
	if err != nil {
		t.Fatalf("Error getting nonce: %v", err)
	}
	blbKey := "0x0000000000000000000000000000000000000000000000000000000000000001"
	blbIdx := common.Big0
	length := new(big.Int).SetInt64(128 * 1024)

	h := crypto.Keccak256Hash([]byte("putBlob(bytes32,uint256,uint256)"))
	mid := h[0:4]
	bytes32Type, _ := abi.NewType("bytes32", "", nil)
	uint256Type, _ := abi.NewType("uint256", "", nil)
	args := abi.Arguments{
		{Type: bytes32Type},
		{Type: uint256Type},
		{Type: uint256Type},
	}
	values := []interface{}{common.HexToHash(blbKey), blbIdx, length}
	dataField, err := args.Pack(values...)
	if err != nil {
		t.Fatalf("Error getting calldata: %v", err)
	}
	calldata := append(mid, dataField...)
	valBig, _ := new(big.Int).SetString(value, 10)
	estimatedGas, err := client.EstimateGas(ctx, ethereum.CallMsg{
		From:  signer,
		To:    &contractAddr16kv,
		Value: valBig,
		Data:  calldata,
	})
	if err != nil {
		lg.Crit("Estimate gas failed", "error", err.Error())
	}
	lg.Info("Estimated gas done", "gas", estimatedGas)

	tx := utils.SendBlobTx(
		l1Endpoint,
		contractAddr16kv,
		privateKey,
		data,
		true,
		int64(n),
		value,
		510000,
		"",
		"",
		"100000000",
		chainID.String(),
		"0x"+common.Bytes2Hex(calldata),
	)
	lg.Info("Blob transaction submitted", "hash", tx.Hash())
	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		lg.Crit("Get transaction receipt err", "error", err)
	}
	// if receipt.Status == 0 {
	// 	lg.Crit("Blob transaction failed")
	// }
	lg.Info("Blob transaction success!", "GasUsed", receipt.GasUsed)
	dataHash := receipt.Logs[0].Topics[3]
	lg.Info("Put Blob done", "datahash", dataHash)
	return dataHash
}

func verifyInclusive(trunkIdx uint64, peInput []byte) error {
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, l1Endpoint)
	if err != nil {
		lg.Crit("Failed to connect to the Ethereum client.", "error", err)
	}
	defer client.Close()

	dataHash := common.Hash{}
	copy(dataHash[:], peInput[:24])
	index := new(big.Int).SetInt64(int64(trunkIdx))
	decodedData := new(big.Int).SetBytes(peInput[64:96])
	h := crypto.Keccak256Hash([]byte("_checkInclusive(bytes32,uint256,uint256,bytes)"))
	mid := h[0:4]
	bytes32Type, _ := abi.NewType("bytes32", "", nil)
	uint256Type, _ := abi.NewType("uint256", "", nil)
	bytesType, _ := abi.NewType("bytes", "", nil)
	args := abi.Arguments{
		{Type: bytes32Type},
		{Type: uint256Type},
		{Type: uint256Type},
		{Type: bytesType},
	}
	values := []interface{}{dataHash, index, decodedData, peInput}
	dataField, err := args.Pack(values...)
	if err != nil {
		return err
	}
	calldata := append(mid, dataField...)
	return callVerify(calldata)
}
