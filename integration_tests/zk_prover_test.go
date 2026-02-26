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

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethstorage/go-ethstorage/ethstorage/encoder"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
	"github.com/protolambda/go-kzg/eth"
)

var zkp1Contract = common.HexToAddress(os.Getenv("ES_NODE_STORAGE_L1CONTRACT_ZKP1"))

const zkeyName = "blob_poseidon.zkey"

func TestZKProver_GenerateZKProofPerSample(t *testing.T) {
	proverPath, _ := filepath.Abs(prPath)
	zkeyFull := filepath.Join(proverPath, prover.SnarkLib, "zkey", zkeyName)
	if _, err := os.Stat(zkeyFull); os.IsNotExist(err) {
		t.Fatalf("%s not found", zkeyFull)
	}
	type args struct {
		encodingKey common.Hash
		sampleIdx   uint64
	}
	tests := []struct {
		name    string
		args    args
		xIn     string
		wantErr bool
	}{
		{
			"test sample 0",
			args{encodingKey: common.HexToHash("0x1"), sampleIdx: 0},
			"1",
			false,
		},
		{
			"test sample 2222",
			args{encodingKey: common.HexToHash("0x22222222222"), sampleIdx: 2222},
			"13571350061658342048390665596699168162893949286891081722317471185110722978977",
			false,
		},
		{
			"test sample 4095",
			args{encodingKey: common.HexToHash("0x1e88fb83944b20562a100533d0521b90bf7df7cc6e0aaa1c46482b67c7b370ab"), sampleIdx: 4095},
			"12199007973319674300030596965685270475268514105269206407619072166392043015767",
			false,
		},
	}
	libDir := filepath.Join(proverPath, prover.SnarkLib)
	pjs := prover.NewZKProver(proverPath, zkeyFull, prover.WasmName, lg)
	pgo, err := prover.NewZKProverGo(libDir, zkeyFull, prover.WasmName, lg)
	if err != nil {
		t.Fatal(err)
	}
	prvs := []prover.IZKProver{pjs, pgo}
	for _, p := range prvs {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				inputBytes, err := prover.GenerateInput(tt.args.encodingKey, tt.args.sampleIdx)
				if err != nil {
					t.Errorf("ZKProver.GenerateInput() error = %v", err)
					return
				}
				var inputs map[string]any
				err = json.Unmarshal(inputBytes, &inputs)
				if err != nil {
					t.Errorf("ZKProver.GenerateInput() error = %v", err)
					return
				}
				inputStr, ok := inputs["xIn"].(string)
				if !ok {
					t.Errorf("ZKProver.GenerateInput() type: %v, want string", reflect.TypeOf(inputs["xIn"]))
					return
				}
				if inputStr != tt.xIn {
					t.Errorf("ZKProver.GenerateInput() xIn = %v, want %v", inputs["xIn"], tt.xIn)
					return
				}
				maskGo, err := GenerateMask(tt.args.encodingKey, tt.args.sampleIdx)
				if (err != nil) != tt.wantErr {
					t.Errorf("GenerateMask() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				proofRaw, mask, err := p.GenerateZKProofPerSample(tt.args.encodingKey, tt.args.sampleIdx)
				if (err != nil) != tt.wantErr {
					t.Errorf("ZKProver.GenerateZKProofPerSample() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if maskGo.Cmp(mask) != 0 {
					t.Errorf("ZKProver.GenerateZKProofPerSample() mask = %v, GenerateMask %v", mask, maskGo)
					return
				}
				xInBig, _ := new(big.Int).SetString(tt.xIn, 10)
				pubs := []*big.Int{
					tt.args.encodingKey.Big(),
					xInBig,
					mask,
				}

				// printProof(proofRaw)

				err = verifyProof1(t, pubs, proofRaw)
				if (err != nil) != tt.wantErr {
					t.Errorf("ZKProver.GenerateZKProofPerSample() verifyProof err: %v", err)
					return
				}
				t.Log("verifyProof success!")
			})
		}
	}
}

// Print the proof in the format required by the contract unit tests
func printProof(proof []byte) {
	var a, c [2][32]byte
	var b [2][2][32]byte

	if len(proof) < 256 {
		fmt.Println("Invalid proof length")
		return
	}

	copy(a[0][:], proof[0:32])
	copy(a[1][:], proof[32:64])

	copy(b[0][0][:], proof[64:96])
	copy(b[0][1][:], proof[96:128])
	copy(b[1][0][:], proof[128:160])
	copy(b[1][1][:], proof[160:192])

	copy(c[0][:], proof[192:224])
	copy(c[1][:], proof[224:256])

	fmt.Printf("[\n  [\n    \"%s\",\n    \"%s\"\n  ],\n  [\n    [\n      \"%s\",\n      \"%s\"\n    ],\n    [\n      \"%s\",\n      \"%s\"\n    ]\n  ],\n  [\n    \"%s\",\n    \"%s\"\n  ]\n]",
		common.BytesToHash(a[0][:]).Hex(),
		common.BytesToHash(a[1][:]).Hex(),
		common.BytesToHash(b[0][0][:]).Hex(),
		common.BytesToHash(b[0][1][:]).Hex(),
		common.BytesToHash(b[1][0][:]).Hex(),
		common.BytesToHash(b[1][1][:]).Hex(),
		common.BytesToHash(c[0][:]).Hex(),
		common.BytesToHash(c[1][:]).Hex())
}

func GenerateMask(encodingKey common.Hash, sampleIdx uint64) (*big.Int, error) {
	if int(sampleIdx) >= eth.FieldElementsPerBlob {
		return nil, fmt.Errorf("sample index out of scope")
	}
	encodingKeyMod := fr.Modulus().Mod(encodingKey.Big(), fr.Modulus())
	masks, err := encoder.Encode(common.BigToHash(encodingKeyMod), eth.FieldElementsPerBlob*32)
	if err != nil {
		return nil, err
	}
	bytesIdx := sampleIdx * 32
	mask := masks[bytesIdx : bytesIdx+32]
	return new(big.Int).SetBytes(mask), nil
}

// call Decoder.sol
func verifyProof1(t *testing.T, pubs []*big.Int, proof []byte) error {
	u2, _ := abi.NewType("uint256[2]", "", nil)
	u3, _ := abi.NewType("uint256[3]", "", nil)
	u22, _ := abi.NewType("uint256[2][2]", "", nil)
	unpacked, err := abi.Arguments{
		{Type: u2},
		{Type: u22},
		{Type: u2},
	}.UnpackValues(proof)
	if err != nil {
		return err
	}
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, l1Endpoint)
	if err != nil {
		t.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	defer client.Close()
	h := crypto.Keccak256Hash([]byte("verifyProof(uint256[2],uint256[2][2],uint256[2],uint256[3])"))
	args := abi.Arguments{
		{Type: u2},
		{Type: u22},
		{Type: u2},
		{Type: u3},
	}
	values := append(unpacked, pubs)
	dataField, err := args.Pack(values...)
	if err != nil {
		return fmt.Errorf("%v, values: %v", err, values)
	}
	t.Logf("values: %x", values)
	calldata := append(h[0:4], dataField...)
	return callVerify(calldata, zkp1Contract)
}
