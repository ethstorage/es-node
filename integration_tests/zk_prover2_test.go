// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ethstorage/go-ethstorage/ethstorage/prover"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	zkey2Name = "blob_poseidon2.zkey"
)

func TestZKProver_GenerateZKProof(t *testing.T) {
	proverPath, _ := filepath.Abs(prPath)
	zkeyFull := filepath.Join(proverPath, prover.SnarkLib, zkey2Name)
	if _, err := os.Stat(zkeyFull); os.IsNotExist(err) {
		t.Fatalf("%s not found", zkeyFull)
	}

	encodingKeys := []common.Hash{
		common.HexToHash("0x1"),
		common.HexToHash("0x1e88fb83944b20562a100533d0521b90bf7df7cc6e0aaa1c46482b67c7b370ab"),
	}
	sampleIdxs := []uint64{
		0,
		4095,
	}
	xIns := []string{
		"1",
		"12199007973319674300030596965685270475268514105269206407619072166392043015767",
	}
	libDir := filepath.Join(proverPath, prover.SnarkLib)
	p, err := prover.NewZKProver(libDir, zkey2Name, prover.Wasm2Name, lg)
	if err != nil {
		t.Errorf("NewZKProver() error = %v", err)
		return
	}
	t.Run("zk test", func(t *testing.T) {
		inputsBytes, err := p.GenerateInputs(encodingKeys, sampleIdxs)
		if err != nil {
			t.Errorf("ZKProver.GenerateInputs() error = %v", err)
			return
		}
		var inputs map[string]interface{}
		err = json.Unmarshal(inputsBytes, &inputs)
		if err != nil {
			t.Errorf("ZKProver.GenerateInputs() error = %v", err)
			return
		}
		vxIn, ok := inputs["xIn"].([]interface{})
		if !ok {
			t.Errorf("ZKProver.GenerateInputs() type: %v, want []interface{}", reflect.TypeOf(inputs["xIn"]))
			return
		}
		for i, xIn := range xIns {
			if vxIn[i] != xIn {
				t.Errorf("ZKProver.GenerateInputs() xIn = %v, want %v", inputs["xIn"], xIns)
				return
			}
		}
		proof, masks, err := p.GenerateZKProof(encodingKeys, sampleIdxs)
		if err != nil {
			t.Errorf("ZKProver.GenerateZKProof() error = %v ", err)
			return
		}
		for i, encodingKey := range encodingKeys {
			maskGo, err := GenerateMask(encodingKey, sampleIdxs[i])
			if err != nil {
				t.Errorf("GenerateMask() error = %v", err)
				return
			}
			if maskGo.Cmp(masks[i]) != 0 {
				t.Errorf("ZKProver.GenerateZKProof() mask = %v, GeneratedMask %v", masks[i], maskGo)
				return
			}
		}
		err = verifyProof(t, masks, proof)
		if err != nil {
			t.Errorf("ZKProver.GenerateZKProof() verifyProof err: %v", err)
		} else {
			t.Log("verifyProof success!")
		}
		err = verifyDecodeSample2(t, masks, proof)
		if err != nil {
			t.Errorf("ZKProver.GenerateZKProof() verifyDecodeSample2 err: %v", err)
			return
		}
		t.Log("verifyDecodeSample2 success!")
	})
}

func verifyDecodeSample2(t *testing.T, masks []*big.Int, proof []byte) error {
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, l1Endpoint)
	if err != nil {
		t.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	defer client.Close()
	h := crypto.Keccak256Hash([]byte("decodeSample(uint256[],bytes)"))
	uintArrayType, _ := abi.NewType("uint256[]", "", nil)
	bytesType, _ := abi.NewType("bytes", "", nil)
	args := abi.Arguments{
		{Type: uintArrayType},
		{Type: bytesType},
	}
	values := []interface{}{masks, proof}
	dataField, err := args.Pack(values...)
	if err != nil {
		return fmt.Errorf("%v, values: %v", err, values)
	}
	calldata := append(h[0:4], dataField...)
	return callVerify(calldata, l1Contract)
}

// call Decoder.sol
func verifyProof(t *testing.T, masks []*big.Int, proof []byte) error {
	u2, _ := abi.NewType("uint256[2]", "", nil)
	u22, _ := abi.NewType("uint256[2][2]", "", nil)
	unpacked, err := abi.Arguments{
		{Type: u2},
		{Type: u22},
		{Type: u2},
	}.UnpackValues(proof)
	if err != nil {
		t.Errorf("ZKProver.GenerateZKProof() unpackValues err: %v", err)
		return err
	}
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, l1Endpoint)
	if err != nil {
		t.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	defer client.Close()
	h := crypto.Keccak256Hash([]byte("verifyProof(uint256[2],uint256[2][2],uint256[2],uint256[2])"))
	args := abi.Arguments{
		{Type: u2},
		{Type: u22},
		{Type: u2},
		{Type: u2},
	}
	values := append(unpacked, masks)
	dataField, err := args.Pack(values...)
	if err != nil {
		return fmt.Errorf("%v, values: %v", err, values)
	}
	t.Logf("values: %x", values)
	calldata := append(h[0:4], dataField...)
	return callVerify(calldata, l1Contract)
}
