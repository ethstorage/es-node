// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package prover

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
)

var (
	rpc          = "http://65.108.236.27:8545"
	chainID      = "7011893058"
	prvk         = "95eb6ffd2ae0b115db4d1f0d58388216f9d026896696a5211d77b5f14eb5badf"
	contractAddr = common.HexToAddress("0xb55D964a4a9d73460393dbB0EB9b6565DB5c1362")
	value        = hexutil.EncodeUint64(10000000000000)
)

func TestKZGProver_GenerateKZGProof(t *testing.T) {
	dataRaw, err := readFile()
	if err != nil {
		t.Fatalf("read raw data error = %v", err)
	}
	dataHash := uploadBlob(dataRaw)
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
	p := NewKZGProver(esLog.NewLogger(esLog.DefaultCLIConfig()))
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

func uploadBlob(data []byte) common.Hash {
	// key: "0x0000000000000000000000000000000000000000000000000000000000000001"
	// blobIdx: 0
	// length: 128*1024
	calldata := "0x4581a920000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000020000"

	tx := utils.SendBlobTx(
		rpc,
		contractAddr,
		prvk,
		data,
		true,
		-1,
		value,
		510000,
		"30000000000",
		"20000000000",
		"10000000000",
		chainID,
		calldata,
	)
	log.Printf("Blob transaction submitted %v", tx.Hash())
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, rpc)
	if err != nil {
		log.Fatal("Failed to connect to the Ethereum client", err)
	}
	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		log.Fatal("Get transaction receipt err:", err)
	}
	if receipt.Status == 0 {
		log.Fatal("Blob transaction failed")
	}
	log.Printf("Blob transaction success! Gas used %v", receipt.GasUsed)
	eventTopics := receipt.Logs[0].Topics
	kvIndex := new(big.Int).SetBytes(eventTopics[1][:])
	kvSize := new(big.Int).SetBytes(eventTopics[2][:])
	dataHash := eventTopics[3]
	log.Printf("Put Blob with kvIndex=%v kvSize=%v, dataHash=%x", kvIndex, kvSize, dataHash)
	return dataHash
}

func verifyInclusive(trunkIdx uint64, peInput []byte) error {
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, rpc)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	defer client.Close()

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
	return callVerify(calldata)
}

func callVerify(calldata []byte) error {
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, rpc)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	defer client.Close()
	msg := ethereum.CallMsg{
		To:   &contractAddr,
		Data: calldata,
	}
	bs, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return err
	}
	if bs[len(bs)-1] != 1 {
		return fmt.Errorf("false")
	}
	return nil
}

func readFile() ([]byte, error) {
	txt_77k := "https://www.gutenberg.org/files/7266/7266-0.txt"
	resp, err := http.Get(txt_77k)
	if err != nil {
		return nil, fmt.Errorf("error reading blob txtUrl: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading blob txtUrl: %v", err)
	}
	return data, nil
}
