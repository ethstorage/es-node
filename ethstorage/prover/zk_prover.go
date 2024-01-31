// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/crate-crypto/go-proto-danksharding-crypto/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
)

const (
	snarkBuildDir    = "snarkbuild"
	witnessGenerator = "generate_witness.js"
	witnessName      = "witness_blob_poseidon.wtns"
	inputName        = "input_blob_poseidon.json"
	proofName        = "proof_blob_poseidon.json"
	publicName       = "public_blob_poseidon.json"
)

type ZKProver struct {
	dir, zkeyName, wasmName string
	lg                      log.Logger
}

func NewZKProver(workingDir, zkeyFile, wasmName string, lg log.Logger) *ZKProver {
	return newZKProver(workingDir, zkeyFile, wasmName, lg)
}

func newZKProver(workingDir, zkeyFile, wasmName string, lg log.Logger) *ZKProver {
	return &ZKProver{
		dir:      workingDir,
		zkeyName: zkeyFile,
		wasmName: wasmName,
		lg:       lg,
	}
}

func (p *ZKProver) GenerateZKProof(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error) {
	proof, publics, err := p.GenerateZKProofRaw(encodingKeys, sampleIdxs)
	if err != nil {
		return nil, nil, err
	}
	if len(publics) != 6 {
		return nil, nil, fmt.Errorf("publics length is %d", len(publics))
	}
	return proof, publics[4:], nil
}

func (p *ZKProver) GenerateZKProofRaw(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error) {
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

	tempDir := crypto.Keccak256Hash([]byte(fmt.Sprint(encodingKeys, sampleIdxs)))
	p.lg.Debug("Generate zk proof", "path", common.Bytes2Hex(tempDir[:]))
	buildDir := filepath.Join(p.dir, snarkBuildDir, common.Bytes2Hex(tempDir[:]))
	if _, err := os.Stat(buildDir); err == nil {
		os.RemoveAll(buildDir)
	}
	err := os.Mkdir(buildDir, os.ModePerm)
	if err != nil {
		p.lg.Error("Generate zk proof failed", "mkdir", buildDir, "error", err)
		return nil, nil, err
	}
	libDir := filepath.Join(p.dir, SnarkLib)

	// 1. Generate input
	inputs, err := p.GenerateInputs(encodingKeys, sampleIdxs)
	if err != nil {
		p.lg.Error("Generate inputs failed", "error", err)
		return nil, nil, err
	}
	inputFile := filepath.Join(buildDir, inputName)
	err = os.WriteFile(inputFile, inputs, 0644)
	if err != nil {
		fmt.Println("Unable to write file:", err)
		return nil, nil, err
	}

	// 2. Generate witness
	wtnsFile := filepath.Join(buildDir, witnessName)
	cmd := exec.Command("node",
		filepath.Join(libDir, witnessGenerator),
		filepath.Join(libDir, p.wasmName),
		inputFile,
		wtnsFile,
	)
	cmd.Dir = libDir
	out, err := cmd.Output()
	if err != nil {
		p.lg.Error("Generate witness failed", "error", err, "cmd", cmd.String(), "output", string(out))
		return nil, nil, err
	}
	p.lg.Debug("Generate witness done")

	// 3. Generate proof
	proofFile := filepath.Join(buildDir, proofName)
	publicFile := filepath.Join(buildDir, publicName)
	cmd = exec.Command("snarkjs", "groth16", "prove",
		filepath.Join(libDir, p.zkeyName),
		wtnsFile,
		proofFile,
		publicFile,
	)
	cmd.Dir = libDir
	out, err = cmd.Output()
	if err != nil {
		p.lg.Error("Generate proof failed", "error", err, "cmd", cmd.String(), "output", string(out))
		return nil, nil, err
	}
	p.lg.Debug("Generate proof done")

	// 4. Read proof and masks
	proof, err := readProofFrom(proofFile)
	if err != nil {
		p.lg.Error("Parse proof failed", "error", err)
		return nil, nil, err
	}
	publics, err := readPublicsFrom(publicFile)
	if err != nil {
		p.lg.Error("Read publics failed", "error", err)
		return nil, nil, err
	}
	return proof, publics, nil
}

func (p *ZKProver) GenerateInputs(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, error) {
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
	inputs, err := json.Marshal(inputObj)
	if err != nil {
		p.lg.Error("Encode input failed", "error", err)
		return nil, err
	}
	p.lg.Debug("Generate zk proof", "input", inputObj)
	return inputs, nil
}

// Generate ZK Proof for the given encoding key and chunck index using snarkjs
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
	buildDir := filepath.Join(p.dir, snarkBuildDir, strings.Join([]string{
		encodingKey.Hex(),
		fmt.Sprint(sampleIdx),
	}, "-"))
	if _, err := os.Stat(buildDir); err == nil {
		os.RemoveAll(buildDir)
	}
	err := os.Mkdir(buildDir, os.ModePerm)
	if err != nil {
		p.lg.Error("Generate zk proof failed", "mkdir", buildDir, "error", err)
		return nil, nil, err
	}
	libDir := filepath.Join(p.dir, SnarkLib)

	// 1. Generate input
	input, err := p.GenerateInput(encodingKey, sampleIdx)
	if err != nil {
		p.lg.Error("Generate input failed", "error", err)
		return nil, nil, err
	}
	inputFile := filepath.Join(buildDir, inputName)
	err = os.WriteFile(inputFile, input, 0644)
	if err != nil {
		fmt.Println("Unable to write file:", err)
		return nil, nil, err
	}

	// 2. Generate witness
	wtnsFile := filepath.Join(buildDir, witnessName)
	cmd := exec.Command("node",
		filepath.Join(libDir, witnessGenerator),
		filepath.Join(libDir, p.wasmName),
		inputFile,
		wtnsFile,
	)
	cmd.Dir = libDir
	out, err := cmd.Output()
	if err != nil {
		p.lg.Error("Generate witness failed", "error", err, "cmd", cmd.String(), "output", string(out))
		return nil, nil, err
	}
	p.lg.Debug("Generate witness done")

	// 3. Generate proof
	proofFile := filepath.Join(buildDir, proofName)
	publicFile := filepath.Join(buildDir, publicName)
	cmd = exec.Command("snarkjs", "groth16", "prove",
		filepath.Join(libDir, p.zkeyName),
		wtnsFile,
		proofFile,
		publicFile,
	)
	cmd.Dir = libDir
	out, err = cmd.Output()
	if err != nil {
		p.lg.Error("Generate proof failed", "error", err, "cmd", cmd.String(), "output", string(out))
		return nil, nil, err
	}
	p.lg.Debug("Generate proof done")

	// 4. Read proof and mask
	proof, err := readProofFrom(proofFile)
	if err != nil {
		p.lg.Error("Parse proof failed", "error", err)
		return nil, nil, err
	}
	mask, err := readMaskFrom(publicFile)
	if err != nil {
		p.lg.Error("Read mask failed", "error", err)
		return nil, nil, err
	}
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
	input, err := json.Marshal(inputObj)
	if err != nil {
		p.lg.Error("Encode input failed", "error", err)
		return nil, err
	}
	p.lg.Debug("Generate zk proof", "input", inputObj)
	return input, nil
}
