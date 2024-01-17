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
	"testing"

	"github.com/ethstorage/go-ethstorage/ethstorage/prover"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	vKeyName2 = "blob_poseidon2_verification_key.json"
	zkeyFile2 = "blob_poseidon2.zkey"
)

func TestZKProver_GenerateZKProof(t *testing.T) {
	proverPath, _ := filepath.Abs(prPath)
	zkeyFull := filepath.Join(proverPath, snarkLibDir, zkeyFile2)
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
	libDir := filepath.Join(proverPath, snarkLibDir)
	p := prover.NewZKProverInternal(proverPath, zkeyFile2, lg)
	t.Run("zk test", func(t *testing.T) {
		proof, publics, err := p.GenerateZKProofRaw(encodingKeys, sampleIdxs)
		if err != nil {
			t.Errorf("ZKProver.GenerateZKProof() error = %v ", err)
			return
		}
		tempDir := crypto.Keccak256Hash([]byte(fmt.Sprint(encodingKeys, sampleIdxs)))
		buildDir := filepath.Join(proverPath, snarkBuildDir, common.Bytes2Hex(tempDir[:]))

		xIn, err := readXIn2(buildDir)
		if err != nil {
			t.Fatalf("get xIn failed %v", err)
		}
		for i, x := range xIn {
			if x != xIns[i] {
				t.Errorf("ZKProver.GenerateZKProof() xIn[%d] = %v, expected %v", i, x, xIns[i])
				return
			}
		}
		masks := publics[4:]
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
		err = localVerify(t, libDir, buildDir, vKeyName2)
		if err != nil {
			t.Errorf("ZKProver.GenerateZKProof() localVerify failed: %v", err)
			return
		}
		err = verifyProof(t, publics, proof)
		if err != nil {
			t.Errorf("ZKProver.GenerateZKProof() verifyProof err: %v", err)
			return
		}
		t.Log("verifyProof success!")
		os.RemoveAll(buildDir)
	})
}

func readXIn2(buildDir string) ([]string, error) {
	f, err := os.Open(filepath.Join(buildDir, inputName))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var input prover.InputPairV2
	var decoder = json.NewDecoder(f)
	err = decoder.Decode(&input)
	if err != nil {
		return nil, err
	}
	return input.XIn, nil
}

// call Decoder2.sol
func verifyProof(t *testing.T, pubs []*big.Int, proof []byte) error {
	u2, _ := abi.NewType("uint256[2]", "", nil)
	u6, _ := abi.NewType("uint256[6]", "", nil)
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
	h := crypto.Keccak256Hash([]byte("verifyProof(uint256[2],uint256[2][2],uint256[2],uint256[6])"))
	args := abi.Arguments{
		{Type: u2},
		{Type: u22},
		{Type: u2},
		{Type: u6},
	}
	values := append(unpacked, pubs)
	dataField, err := args.Pack(values...)
	if err != nil {
		return fmt.Errorf("%v, values: %v", err, values)
	}
	t.Logf("values: %x", values)
	calldata := append(h[0:4], dataField...)
	return callVerify(calldata, l1Contract)
}
