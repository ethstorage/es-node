// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/crate-crypto/go-proto-danksharding-crypto/eth"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/iden3/go-rapidsnark/prover"
	"github.com/iden3/go-rapidsnark/types"
	"github.com/iden3/go-rapidsnark/witness/v2"

	// "github.com/iden3/go-rapidsnark/witness/wazero"
	"github.com/iden3/go-rapidsnark/witness/wasmer"
)

type ZKProver struct {
	calc witness.Calculator
	zkey []byte
	lg   log.Logger
}

func NewZKProver(libDir, zkeyName, wasmName string, lg log.Logger) (*ZKProver, error) {
	wasmBytes, err := os.ReadFile(filepath.Join(libDir, wasmName))
	if err != nil {
		lg.Error("Read wasm file failed", "error", err)
		return nil, err
	}
	calc, err := witness.NewCalculator(wasmBytes, witness.WithWasmEngine(wasmer.NewCircom2WitnessCalculator))
	if err != nil {
		lg.Error("Create witness calculator failed", "error", err)
		return nil, err
	}
	zkey, err := os.ReadFile(filepath.Join(libDir, zkeyName))
	if err != nil {
		lg.Error("Read zkey file failed", "error", err)
		return nil, err
	}
	return &ZKProver{
		zkey: zkey,
		calc: calc,
		lg:   lg,
	}, nil
}

// Generate ZK Proof for the given encoding key and sample index
func (p *ZKProver) GenerateZKProofPerSample(encodingKey common.Hash, sampleIdx uint64) ([]byte, *big.Int, error) {
	p.lg.Debug("Generate zk proof", "encodingKey", encodingKey.Hex(), "sampleIdx", sampleIdx)
	if int(sampleIdx) >= eth.FieldElementsPerBlob {
		return nil, nil, fmt.Errorf("sample index out of scope: %d", sampleIdx)
	}
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		p.lg.Info("Generate zk proof", "sampleIdx", sampleIdx, "timeUsed(s)", dur.Seconds())
	}(start)

	inputBytes, err := p.GenerateInput(encodingKey, sampleIdx)
	if err != nil {
		p.lg.Error("Generate inputs failed", "error", err)
		return nil, nil, err
	}
	proof, publicInputs, err := p.prove(inputBytes)
	if err != nil {
		p.lg.Error("Prove failed", "error", err)
		return nil, nil, err
	}
	mask, err := readMask(publicInputs)
	if err != nil {
		return nil, nil, err
	}
	p.lg.Debug("Generate zk proof", "mask", mask)
	return proof, mask, nil
}

func (p *ZKProver) GenerateInput(encodingKey common.Hash, sampleIdx uint64) ([]byte, error) {
	var b fr.Element
	var exp big.Int
	exp.Div(exp.Sub(fr.Modulus(), common.Big1), big.NewInt(int64(eth.FieldElementsPerBlob)))
	ru := b.Exp(*b.SetInt64(5), &exp)
	xIn := ru.Exp(*ru, big.NewInt(int64(sampleIdx)))
	encodingKeyMod := fr.Modulus().Mod(encodingKey.Big(), fr.Modulus())
	inputObj := InputPair{
		EncodingKeyIn: hexutil.Encode(encodingKeyMod.Bytes()),
		XIn:           xIn.String(),
	}
	p.lg.Debug("Generate zk proof", "input", inputObj)
	return json.Marshal(inputObj)
}

func (p *ZKProver) prove(inputBytes []byte) ([]byte, string, error) {
	parsedInputs, err := witness.ParseInputs(inputBytes)
	if err != nil {
		p.lg.Error("Parse input failed", "error", err)
		return nil, "", err
	}
	wtnsBytes, err := p.calc.CalculateWTNSBin(parsedInputs, true)
	if err != nil {
		p.lg.Error("Calculate witness failed", "error", err)
		return nil, "", err
	}
	proofRaw, publicInputs, err := prover.Groth16ProverRaw(p.zkey, wtnsBytes)
	if err != nil {
		p.lg.Error("Prove failed", "error", err)
		return nil, "", err
	}
	p.lg.Debug("Generate zk proof", "publicInputs", publicInputs)
	proof, err := readProof(proofRaw)
	if err != nil {
		p.lg.Error("Read proof failed", "error", err)
		return nil, "", err
	}
	return proof, publicInputs, nil
}

func readProof(proofRaw string) ([]byte, error) {
	var piOut = types.ProofData{}
	if err := json.Unmarshal([]byte(proofRaw), &piOut); err != nil {
		return nil, err
	}
	u2, _ := abi.NewType("uint256[2]", "", nil)
	u22, _ := abi.NewType("uint256[2][2]", "", nil)
	args := abi.Arguments{
		{Type: u2},
		{Type: u22},
		{Type: u2},
	}
	a, err := toG1Point(piOut.A[:2])
	if err != nil {
		return nil, err
	}
	b, err := toG2Point(piOut.B[0:2])
	if err != nil {
		return nil, err
	}
	c, err := toG1Point(piOut.C[0:2])
	if err != nil {
		return nil, err
	}
	values := []interface{}{[]*big.Int{a.X, a.Y}, [][]*big.Int{b.X[:], b.Y[:]}, []*big.Int{c.X, c.Y}}
	packed, err := args.Pack(values...)
	if err != nil {
		return nil, fmt.Errorf("%v, values: %v", err, values)
	}
	return packed, nil
}

func readMask(publicInputs string) (*big.Int, error) {
	var output []string
	if err := json.Unmarshal([]byte(publicInputs), &output); err != nil {
		return nil, err
	}
	if len(output) != 3 {
		return nil, fmt.Errorf("invalid public output")
	}
	mask, ok := new(big.Int).SetString(output[2], 0)
	if !ok {
		return nil, fmt.Errorf("invalid mask")
	}
	return mask, nil
}

type InputPair struct {
	EncodingKeyIn string `json:"encodingKeyIn"`
	XIn           string `json:"xIn"`
}

func toG1Point(s []string) (G1Point, error) {
	var x, y big.Int
	_, ok := x.SetString(s[0], 10)
	if !ok {
		return G1Point{}, fmt.Errorf("invalid number %s", s[0])
	}
	_, ok = y.SetString(s[1], 10)
	if !ok {
		return G1Point{}, fmt.Errorf("invalid number %s", s[1])
	}
	return G1Point{&x, &y}, nil
}

func toG2Point(s [][]string) (G2Point, error) {
	var x, y [2]*big.Int
	for i, vi := range s {
		for j, vj := range vi {
			z := new(big.Int)
			_, ok := z.SetString(vj, 10)
			if !ok {
				return G2Point{}, fmt.Errorf("invalid number %s", vj)
			}
			// swap so that it can be accepted by the verifier contract
			if i == 0 {
				x[1-j] = z
			} else {
				y[1-j] = z
			}
		}
	}
	return G2Point{x, y}, nil
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
