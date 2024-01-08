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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/iden3/go-rapidsnark/prover"
	"github.com/iden3/go-rapidsnark/witness/v2"
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

// Generate ZK Proof for the given encoding keys and chunck indexes
func (p *ZKProver) GenerateZKProof(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error) {
	for i, idx := range sampleIdxs {
		p.lg.Debug("Generate zk proof", "encodingKey", encodingKeys[i], "sampleIdx", sampleIdxs[i])
		if int(idx) >= eth.FieldElementsPerBlob {
			return nil, nil, fmt.Errorf("chunk index out of scope: %d", idx)
		}
	}
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		p.lg.Info("Generate zk proof done", "sampleIdx", sampleIdxs, "took(sec)", dur.Seconds())
	}(start)

	var encodingKeyModStr, xInStr []string
	for i, sampleIdx := range sampleIdxs {
		var b fr.Element
		var exp big.Int
		exp.Div(exp.Sub(fr.Modulus(), common.Big1), big.NewInt(int64(eth.FieldElementsPerBlob)))
		ru := b.Exp(*b.SetInt64(5), &exp)
		xIn := ru.Exp(*ru, new(big.Int).SetUint64(sampleIdx))
		xInStr = append(xInStr, xIn.String())
		encodingKeyMod := fr.Modulus().Mod(encodingKeys[i].Big(), fr.Modulus())
		encodingKeyModStr = append(encodingKeyModStr, hexutil.Encode(encodingKeyMod.Bytes()))
	}
	inputObj := InputPairV2{
		EncodingKeyIn: encodingKeyModStr,
		XIn:           xInStr,
	}
	p.lg.Debug("Generate zk proof", "input", inputObj)
	inputBytes, err := json.Marshal(inputObj)
	if err != nil {
		p.lg.Error("Marshal input failed", "error", err)
		return nil, nil, err
	}
	parsedInputs, err := witness.ParseInputs(inputBytes)
	if err != nil {
		p.lg.Error("Parse input failed", "error", err)
		return nil, nil, err
	}
	wtnsBytes, err := p.calc.CalculateWTNSBin(parsedInputs, true)
	if err != nil {
		p.lg.Error("Calculate witness failed", "error", err)
		return nil, nil, err
	}
	proof, publicInputs, err := prover.Groth16ProverRaw(p.zkey, wtnsBytes)
	if err != nil {
		p.lg.Error("Prove failed", "error", err)
		return nil, nil, err
	}
	masks, err := readMasks(publicInputs)
	if err != nil {
		p.lg.Error("Read masks failed", "error", err)
		return nil, nil, err
	}
	return []byte(proof), masks, nil
}

// Generate ZK Proof for the given encoding key and chunck index
func (p *ZKProver) GenerateZKProofPerSample(encodingKey common.Hash, sampleIdx uint64) ([]byte, *big.Int, error) {
	p.lg.Debug("Generate zk proof", "encodingKey", encodingKey.Hex(), "sampleIdx", sampleIdx)
	if int(sampleIdx) >= eth.FieldElementsPerBlob {
		return nil, nil, fmt.Errorf("chunk index out of scope: %d", sampleIdx)
	}
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		p.lg.Info("Generate zk proof", "sampleIdx", sampleIdx, "took(sec)", dur.Seconds())
	}(start)

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
	inputBytes, err := json.Marshal(inputObj)
	if err != nil {
		p.lg.Error("Marshal input failed", "error", err)
		return nil, nil, err
	}
	parsedInputs, err := witness.ParseInputs(inputBytes)
	if err != nil {
		p.lg.Error("Parse input failed", "error", err)
		return nil, nil, err
	}
	wtnsBytes, err := p.calc.CalculateWTNSBin(parsedInputs, true)
	if err != nil {
		p.lg.Error("Calculate witness failed", "error", err)
		return nil, nil, err
	}
	proof, publicInputs, err := prover.Groth16ProverRaw(p.zkey, wtnsBytes)
	if err != nil {
		p.lg.Error("Prove failed", "error", err)
		return nil, nil, err
	}
	mask, err := readMask(publicInputs)
	if err != nil {
		return nil, nil, err
	}
	return []byte(proof), mask, nil
}

func readMasks(publicInputs string) ([]*big.Int, error) {
	var output []string
	if err := json.Unmarshal([]byte(publicInputs), &output); err != nil {
		return nil, err
	}
	var masks []*big.Int
	for _, v := range output {
		mask, ok := new(big.Int).SetString(v, 0)
		if !ok {
			return masks, fmt.Errorf("invalid mask %v", v)
		}
		masks = append(masks, mask)
	}
	return masks, nil
}

func readMask(publicFile string) (*big.Int, error) {
	f, err := os.Open(publicFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var output []string
	var decoder = json.NewDecoder(f)
	err = decoder.Decode(&output)
	if err != nil {
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

type InputPairV2 struct {
	EncodingKeyIn []string `json:"encodingKeyIn"`
	XIn           []string `json:"xIn"`
}

type InputPair struct {
	EncodingKeyIn string `json:"encodingKeyIn"`
	XIn           string `json:"xIn"`
}
