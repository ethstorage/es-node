// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

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
	"time"

	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/crate-crypto/go-proto-danksharding-crypto/eth"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethstorage/go-ethstorage/ethstorage/encoder"
)

func TestZKProver_GenerateZKProof(t *testing.T) {
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
	lg := esLog.NewLogger(esLog.DefaultCLIConfig())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maskGo, err := GenerateMask(tt.args.encodingKey, tt.args.chunkIdx)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateMask() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			p := newZKProver("", "blob_poseidon.zkey", false, lg)
			proof, mask, err := p.GenerateZKProof(tt.args.encodingKey, tt.args.chunkIdx)
			if (err != nil) != tt.wantErr {
				t.Errorf("ZKProver.GenerateZKProof() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			buildDir := filepath.Join(snarkBuildDir, strings.Join([]string{
				tt.args.encodingKey.Hex(),
				fmt.Sprint(tt.args.chunkIdx),
			}, "-"))
			xIn, err := readXIn(buildDir)
			if err != nil {
				t.Fatalf("get xIn failed %v", err)
			}
			if !reflect.DeepEqual(xIn, tt.xIn) {
				t.Errorf("ZKProver.GenerateZKProof() xIn = %v, want %v", xIn, tt.xIn)
			}
			maskBig := common.BigToHash(new(big.Int).SetBytes(maskGo[:]))
			if !reflect.DeepEqual(maskBig, mask) {
				t.Errorf("ZKProver.GenerateZKProof() mask = %v, GenerateMask %v", mask, maskBig)
				return
			}
			err = localVerify(buildDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ZKProver.GenerateZKProof() localVerify failed: %v", err)
			}
			err = verifyDecodeSample(proof, tt.args.chunkIdx, tt.args.encodingKey, mask)
			if (err != nil) != tt.wantErr {
				t.Errorf("ZKProver.GenerateZKProof() decodeSample err: %v", err)
			}
			os.RemoveAll(buildDir)
		})
	}
}

func GenerateMask(encodingKey common.Hash, chunkIdx uint64) (common.Hash, error) {
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		log.Printf("GenerateMask() took %0.0f seconds to execute", dur.Seconds())
	}(start)

	if int(chunkIdx) >= eth.FieldElementsPerBlob {
		return common.Hash{}, fmt.Errorf("chunk index out of scope")
	}
	encodingKeyMod := fr.Modulus().Mod(encodingKey.Big(), fr.Modulus())
	masks, err := encoder.Encode(common.BigToHash(encodingKeyMod), eth.FieldElementsPerBlob*32)
	if err != nil {
		return common.Hash{}, err
	}
	bytesIdx := chunkIdx * 32
	mask := masks[bytesIdx : bytesIdx+32]
	log.Printf("generate mask done %x", mask)
	return common.BytesToHash(mask), nil
}

func readXIn(buildDir string) (string, error) {
	f, err := os.Open(filepath.Join(buildDir, "input_blob_poseidon.json"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	var input InputPair
	var decoder *json.Decoder = json.NewDecoder(f)
	err = decoder.Decode(&input)
	if err != nil {
		return "", err
	}
	return input.XIn, nil
}

func localVerify(buildDir string) error {
	cmd := exec.Command("snarkjs", "groth16", "verify",
		filepath.Join(snarkLibDir, "blob_poseidon_verification_key.json"),
		filepath.Join(buildDir, publicName),
		filepath.Join(buildDir, proofName),
	)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("Local verify failed %v", err)
	}
	log.Printf("Local verify done. result: %s", out)
	return nil
}

func verifyDecodeSample(proof ZKProof, trunkIdx uint64, encodingKey, mask common.Hash) error {
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, rpc)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	defer client.Close()

	encodingKeyBN := new(big.Int).SetBytes(encodingKey[:])
	indexBN := new(big.Int).SetInt64((int64(trunkIdx)))
	maskBN := new(big.Int).SetBytes(mask[:])

	h := crypto.Keccak256Hash([]byte("decodeSample(((uint256,uint256),(uint256[2],uint256[2]),(uint256,uint256)),uint256,uint256,uint256)"))
	mid := h[0:4]
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
	values := []interface{}{proof, encodingKeyBN, indexBN, maskBN}
	dataField, err := args.Pack(values...)
	if err != nil {
		return fmt.Errorf("Err: %v, args.Pack: %v", err, values)
	}
	calldata := append(mid, dataField...)
	return callVerify(calldata)
}
