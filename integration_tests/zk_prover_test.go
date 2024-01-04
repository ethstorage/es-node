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
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ethstorage/go-ethstorage/ethstorage/prover"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/crate-crypto/go-proto-danksharding-crypto/eth"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethstorage/go-ethstorage/ethstorage/encoder"
)

const (
	snarkLibDir   = "snarkjs"
	snarkBuildDir = "snarkbuild"
	proofName     = "proof_blob_poseidon.json"
	publicName    = "public_blob_poseidon.json"
	prPath        = "../ethstorage/prover"
	zkeyFile      = "blob_poseidon.zkey"
)

func TestZKProver_GenerateZKProof(t *testing.T) {
	proverPath, _ := filepath.Abs(prPath)
	zkeyFull := filepath.Join(proverPath, snarkLibDir, zkeyFile)
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
	p := prover.NewZKProverInternal(proverPath, zkeyFile, lg)
	t.Run("zk test", func(t *testing.T) {
		proof, masks, err := p.GenerateZKProof(encodingKeys, sampleIdxs)
		if err != nil {
			t.Errorf("ZKProver.GenerateZKProof() error = %v ", err)
			return
		}
		tempDir := crypto.Keccak256Hash([]byte(fmt.Sprint(encodingKeys, sampleIdxs)))
		buildDir := filepath.Join(proverPath, snarkBuildDir, common.Bytes2Hex(tempDir[:]))
		defer os.RemoveAll(buildDir)

		xIn, err := readXIn(buildDir)
		if err != nil {
			t.Fatalf("get xIn failed %v", err)
		}
		for i, x := range xIn {
			if x != xIns[i] {
				t.Errorf("ZKProver.GenerateZKProof() xIn[%d] = %v, expected %v", i, x, xIns[i])
				return
			}
		}
		for i, encodingKey := range encodingKeys {
			maskGo, err := GenerateMask(encodingKey, sampleIdxs[i])
			if err != nil {
				t.Errorf("GenerateMask() error = %v", err)
				return
			}
			if maskGo != masks[i] {
				t.Errorf("ZKProver.GenerateZKProof() mask = %v, GeneratedMask %v", masks[i], maskGo)
				return
			}
		}
		err = localVerify(t, libDir, buildDir)
		if err != nil {
			t.Errorf("ZKProver.GenerateZKProof() localVerify failed: %v", err)
			return
		}
		err = verifyProof(t, masks, proof)
		if err != nil {
			t.Errorf("ZKProver.GenerateZKProof() verifyProof err: %v", err)
		} else {
			t.Log("verifyProof success!")
		}
		err = verifyDecodeSample(t, masks, proof)
		if err != nil {
			t.Errorf("ZKProver.GenerateZKProof() verifyDecodeSample err: %v", err)
			return
		}
		t.Log("verifyDecodeSample success!")
	})
}

func GenerateMask(encodingKey common.Hash, chunkIdx uint64) (*big.Int, error) {
	if int(chunkIdx) >= eth.FieldElementsPerBlob {
		return nil, fmt.Errorf("chunk index out of scope")
	}
	encodingKeyMod := fr.Modulus().Mod(encodingKey.Big(), fr.Modulus())
	masks, err := encoder.Encode(common.BigToHash(encodingKeyMod), eth.FieldElementsPerBlob*32)
	if err != nil {
		return nil, err
	}
	bytesIdx := chunkIdx * 32
	mask := masks[bytesIdx : bytesIdx+32]
	return new(big.Int).SetBytes(mask), nil
}

func readXIn(buildDir string) ([]string, error) {
	f, err := os.Open(filepath.Join(buildDir, "input_blob_poseidon.json"))
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

func localVerify(t *testing.T, libDir string, buildDir string) error {
	cmd := exec.Command("snarkjs", "groth16", "verify",
		filepath.Join(libDir, "blob_poseidon_verification_key.json"),
		filepath.Join(buildDir, publicName),
		filepath.Join(buildDir, proofName),
	)
	out, err := cmd.Output()
	if err != nil {
		t.Logf("Local verify failed: %v, cmd: %s, output: %v", err, cmd.String(), string(out))
		return err
	}
	t.Logf("Local verify done. result: %s", out)
	return nil
}

func verifyDecodeSample(t *testing.T, masks []*big.Int, proof []byte) error {
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
