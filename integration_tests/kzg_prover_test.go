// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package integration

import (
	"bytes"
	"context"
	"math/big"
	"os"
	"testing"

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

var kzgContract = common.HexToAddress(os.Getenv("ES_NODE_STORAGE_L1CONTRACT_KZG"))

func TestKZGProver_GenerateKZGProof(t *testing.T) {
	lg.Info("KZG prover test", "contract", kzgContract)
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
	client, err := ethclient.DialContext(context.Background(), l1Endpoint)
	if err != nil {
		t.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	defer client.Close()

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		t.Fatalf("Get chain id failed %v", err)
	}
	sig := crypto.Keccak256Hash([]byte("storageCost()"))
	bs, err := client.CallContract(context.Background(), ethereum.CallMsg{
		To:   &kzgContract,
		Data: sig[0:4],
	}, nil)
	if err != nil {
		t.Fatalf("failed to get storageCost from contract: %v", err)
	}
	storageCost := new(big.Int).SetBytes(bs)
	lg.Info("Get storage cost done", "storageCost", storageCost)

	key, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		t.Fatalf("Invalid private key: %s, err: %v", privateKey, err)
	}
	signer := crypto.PubkeyToAddress(key.PublicKey)
	lg.Info("Get signer address", "signer", signer.Hex())
	n, err := client.NonceAt(context.Background(), signer, big.NewInt(rpc.LatestBlockNumber.Int64()))
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
	estimatedGas, err := client.EstimateGas(context.Background(), ethereum.CallMsg{
		From:  signer,
		To:    &kzgContract,
		Value: storageCost,
		Data:  calldata,
	})
	if err != nil {
		lg.Crit("Estimate gas failed", "error", err.Error())
	}
	lg.Info("Estimated gas done", "gas", estimatedGas)

	tx := utils.SendBlobTx(
		l1Endpoint,
		kzgContract,
		privateKey,
		data,
		true,
		int64(n),
		storageCost.String(),
		510000,
		"",
		"",
		"100000000",
		chainID.String(),
		"0x"+common.Bytes2Hex(calldata),
	)
	lg.Info("Blob transaction submitted", "hash", tx.Hash())
	receipt, err := bind.WaitMined(context.Background(), client, tx)
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
	dataHash := common.Hash{}
	copy(dataHash[:], peInput[:24])
	index := new(big.Int).SetInt64(int64(trunkIdx))
	decodedData := new(big.Int).SetBytes(peInput[64:96])
	h := crypto.Keccak256Hash([]byte("checkInclusive(bytes32,uint256,uint256,bytes)"))
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
	return callVerify(calldata, kzgContract)
}
