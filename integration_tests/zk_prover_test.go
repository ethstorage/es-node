// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/crate-crypto/go-proto-danksharding-crypto/eth"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethstorage/go-ethstorage/ethstorage/encoder"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
)

const (
	l1ContractV1  = "0xc3208C27285ed9516F21a89053326Bb895DD78F7"
	snarkBuildDir = "snarkbuild"
	inputName     = "input_blob_poseidon.json"
	proofName     = "proof_blob_poseidon.json"
	publicName    = "public_blob_poseidon.json"
	vKeyName      = "blob_poseidon_verification_key.json"
	prPath        = "../ethstorage/prover"
	zkeyFile      = "blob_poseidon.zkey"
)

func TestZKProver_GenerateZKProofPerSample(t *testing.T) {
	proverPath, _ := filepath.Abs(prPath)
	zkeyFull := filepath.Join(proverPath, prover.SnarkLib, zkeyFile)
	if _, err := os.Stat(zkeyFull); os.IsNotExist(err) {
		t.Fatalf("%s not found", zkeyFull)
	}
	type args struct {
		encodingKey common.Hash
		chunkIdx    uint64
	}
	tests := []struct {
		name    string
		args    args
		xIn     string
		wantErr bool
	}{
		{
			"test chunk 0",
			args{encodingKey: common.HexToHash("0x1"), chunkIdx: 0},
			"1",
			false,
		},
		{
			"test chunk 2222",
			args{encodingKey: common.HexToHash("0x22222222222"), chunkIdx: 2222},
			"13571350061658342048390665596699168162893949286891081722317471185110722978977",
			false,
		},
		{
			"test chunk 4095",
			args{encodingKey: common.HexToHash("0x1e88fb83944b20562a100533d0521b90bf7df7cc6e0aaa1c46482b67c7b370ab"), chunkIdx: 4095},
			"12199007973319674300030596965685270475268514105269206407619072166392043015767",
			false,
		},
	}
	libDir := filepath.Join(proverPath, prover.SnarkLib)
	p := prover.NewZKProverInternal(proverPath, zkeyFile, lg)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maskGo, err := GenerateMask(tt.args.encodingKey, tt.args.chunkIdx)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateMask() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			proof, mask, err := p.GenerateZKProofPerSample(tt.args.encodingKey, tt.args.chunkIdx)
			if (err != nil) != tt.wantErr {
				t.Errorf("ZKProver.GenerateZKProofPerSample() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			buildDir := filepath.Join(proverPath, snarkBuildDir, strings.Join([]string{
				tt.args.encodingKey.Hex(),
				fmt.Sprint(tt.args.chunkIdx),
			}, "-"))
			xIn, err := readXIn(buildDir)
			if err != nil {
				t.Fatalf("get xIn failed %v", err)
			}
			if !reflect.DeepEqual(xIn, tt.xIn) {
				t.Errorf("ZKProver.GenerateZKProofPerSample() xIn = %v, want %v", xIn, tt.xIn)
			}
			if maskGo.Cmp(mask) != 0 {
				t.Errorf("ZKProver.GenerateZKProofPerSample() mask = %v, GenerateMask %v", mask, maskGo)
				return
			}
			err = localVerify(t, libDir, buildDir, vKeyName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ZKProver.GenerateZKProofPerSample() localVerify failed: %v", err)
				return
			}
			err = verifyDecodeSample(proof, tt.args.chunkIdx, tt.args.encodingKey, mask)
			if (err != nil) != tt.wantErr {
				t.Errorf("ZKProver.GenerateZKProofPerSample() verifyDecodeSample err: %v", err)
				return
			}
			t.Log("verifyDecodeSample success!")
			os.RemoveAll(buildDir)
		})
	}
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

func readXIn(buildDir string) (string, error) {
	f, err := os.Open(filepath.Join(buildDir, inputName))
	if err != nil {
		return "", err
	}
	defer f.Close()
	var input prover.InputPair
	var decoder = json.NewDecoder(f)
	err = decoder.Decode(&input)
	if err != nil {
		return "", err
	}
	return input.XIn, nil
}

func localVerify(t *testing.T, libDir, buildDir, vKeyName string) error {
	cmd := exec.Command("snarkjs", "groth16", "verify",
		filepath.Join(libDir, vKeyName),
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

func verifyDecodeSample(proofBytes []byte, trunkIdx uint64, encodingKey common.Hash, mask *big.Int) error {
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, l1Endpoint)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	defer client.Close()

	encodingKeyBN := new(big.Int).SetBytes(encodingKey[:])
	indexBN := new(big.Int).SetInt64(int64(trunkIdx))
	h := crypto.Keccak256Hash([]byte("decodeSample(((uint256,uint256),(uint256[2],uint256[2]),(uint256,uint256)),uint256,uint256,uint256)"))
	uintType, _ := abi.NewType("uint256", "", nil)
	proofType, _ := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{
			Name: "A", Type: "tuple", Components: []abi.ArgumentMarshaling{
				{Name: "X", Type: "uint256"},
				{Name: "Y", Type: "uint256"},
			},
		},
		{
			Name: "B", Type: "tuple", Components: []abi.ArgumentMarshaling{
				{Name: "X", Type: "uint256[2]"},
				{Name: "Y", Type: "uint256[2]"},
			},
		},
		{
			Name: "C", Type: "tuple", Components: []abi.ArgumentMarshaling{
				{Name: "X", Type: "uint256"},
				{Name: "Y", Type: "uint256"},
			},
		},
	})
	args := abi.Arguments{
		{Type: proofType},
		{Type: uintType},
		{Type: uintType},
		{Type: uintType},
	}
	proof := parseProof(proofBytes)
	values := []interface{}{proof, encodingKeyBN, indexBN, mask}
	dataField, err := args.Pack(values...)
	if err != nil {
		return fmt.Errorf("Err: %v, args.Pack: %v", err, values)
	}
	calldata := append(h[0:4], dataField...)
	return callVerify(calldata, common.HexToAddress(l1ContractV1))
}

func parseProof(data []byte) ZKProof {
	zkProof := ZKProof{}
	x1 := new(big.Int).SetBytes(data[:32])
	y1 := new(big.Int).SetBytes(data[32:64])
	zkProof.A = G1Point{X: x1, Y: y1}
	var x2 [2]*big.Int
	var y2 [2]*big.Int
	x2[0] = new(big.Int).SetBytes(data[64:96])
	x2[1] = new(big.Int).SetBytes(data[96:128])
	y2[0] = new(big.Int).SetBytes(data[128:160])
	y2[1] = new(big.Int).SetBytes(data[160:192])
	zkProof.B = G2Point{X: x2, Y: y2}
	x3 := new(big.Int).SetBytes(data[192:224])
	y3 := new(big.Int).SetBytes(data[224:256])
	zkProof.C = G1Point{X: x3, Y: y3}

	return zkProof
}

type ZKProof struct {
	A G1Point `json:"A"`
	B G2Point `json:"B"`
	C G1Point `json:"C"`
}

type G1Point struct {
	X *big.Int
	Y *big.Int
}

func (p G1Point) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		X string `json:"X"`
		Y string `json:"Y"`
	}{
		X: p.X.String(),
		Y: p.Y.String(),
	})
}

func (p *G1Point) UnmarshalJSON(b []byte) error {
	var values struct {
		X string `json:"X"`
		Y string `json:"Y"`
	}
	err := json.Unmarshal(b, &values)
	if err != nil {
		fmt.Printf("Unmarshal %v\n", err)
		return err
	}
	p.X = new(big.Int)
	_, ok := p.X.SetString(values.X, 10)
	if !ok {
		return err
	}
	p.Y = new(big.Int)
	_, ok = p.Y.SetString(values.Y, 10)
	if !ok {
		return err
	}
	return nil
}

type G2Point struct {
	X [2]*big.Int `json:"X"`
	Y [2]*big.Int `json:"Y"`
}

func (p *G2Point) UnmarshalJSON(b []byte) error {
	var values struct {
		X [2]string `json:"X"`
		Y [2]string `json:"Y"`
	}
	err := json.Unmarshal(b, &values)
	if err != nil {
		fmt.Printf("Unmarshal %v\n", err)
		return err
	}
	for j, vj := range values.X {
		z := new(big.Int)
		_, ok := z.SetString(vj, 10)
		if !ok {
			return err
		}
		// swap so that it can be accepted by the verifier contract
		p.X[1-j] = z
	}
	for j, vj := range values.Y {
		z := new(big.Int)
		_, ok := z.SetString(vj, 10)
		if !ok {
			return err
		}
		// swap so that it can be accepted by the verifier contract
		p.Y[1-j] = z
	}
	return nil
}

func (p G2Point) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		X [2]string `json:"X"`
		Y [2]string `json:"Y"`
	}{
		X: [2]string{p.X[0].String(), p.X[1].String()},
		Y: [2]string{p.Y[0].String(), p.Y[1].String()},
	})
}
