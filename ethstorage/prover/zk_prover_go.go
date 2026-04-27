//go:build !windows

// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/iden3/go-rapidsnark/prover"
	"github.com/iden3/go-rapidsnark/witness/v2"
	"github.com/iden3/go-rapidsnark/witness/wazero"
	"github.com/protolambda/go-kzg/eth"
)

type ZKProverGo struct {
	zkey []byte
	wasm []byte
	lg   log.Logger
	mu   sync.Mutex
}

var (
	zkProverGoInstance *ZKProverGo
	zkProverGoOnce     sync.Once
)

// Singleton with a lock is used as RapidsNARK may not be thread-safe at the C++ level.
func NewZKProverGo(libDir, zkeyFile, wasmName string, lg log.Logger) (*ZKProverGo, error) {
	var err error
	zkProverGoOnce.Do(func() {
		wasmBytes, e1 := os.ReadFile(filepath.Join(libDir, wasmName))
		if e1 != nil {
			lg.Error("Read wasm file failed", "error", e1)
			err = e1
			return
		}
		zkey, e2 := os.ReadFile(zkeyFile)
		if e2 != nil {
			lg.Error("Read zkey file failed", "error", e2)
			err = e2
			return
		}
		zkProverGoInstance = &ZKProverGo{
			zkey: zkey,
			wasm: wasmBytes,
			lg:   lg,
		}
	})
	if err != nil {
		return nil, err
	}
	return zkProverGoInstance, nil
}

// Generate ZK Proof for the given encoding key and sample index
func (p *ZKProverGo) GenerateZKProof(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	proof, publics, err := p.GenerateZKProofRaw(encodingKeys, sampleIdxs)
	if err != nil {
		return nil, nil, err
	}
	if len(publics) != 6 {
		return nil, nil, fmt.Errorf("publics length is %d", len(publics))
	}
	return proof, publics[4:], nil
}

func (p *ZKProverGo) GenerateZKProofRaw(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error) {
	p.lg.Debug("Generate zk proof", "encodingKeys", encodingKeys, "sampleIndexes", sampleIdxs)
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		p.lg.Info("Generate zk proof done", "sampleIdx", sampleIdxs, "timeUsed(s)", dur.Seconds())
	}(start)

	inputBytes, err := GenerateInputs(encodingKeys, sampleIdxs)
	if err != nil {
		p.lg.Error("Generate inputs failed", "error", err)
		return nil, nil, err
	}
	proof, publicInputs, err := p.prove(inputBytes)
	if err != nil {
		p.lg.Error("Prove failed", "error", err)
		return nil, nil, err
	}
	publics, err := readPublics([]byte(publicInputs))
	if err != nil {
		p.lg.Error("Read publics failed", "error", err)
		return nil, nil, err
	}
	return proof, publics, nil
}

func (p *ZKProverGo) GenerateZKProofPerSample(encodingKey common.Hash, sampleIdx uint64) ([]byte, *big.Int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.lg.Debug("Generate zk proof", "encodingKey", encodingKey.Hex(), "sampleIdx", sampleIdx)
	if int(sampleIdx) >= eth.FieldElementsPerBlob {
		return nil, nil, fmt.Errorf("sample index out of scope: %d", sampleIdx)
	}
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		p.lg.Info("Generate zk proof", "sampleIdx", sampleIdx, "timeUsed(s)", dur.Seconds())
	}(start)

	inputBytes, err := GenerateInput(encodingKey, sampleIdx)
	if err != nil {
		p.lg.Error("Generate inputs failed", "error", err)
		return nil, nil, err
	}
	proof, publicInputs, err := p.prove(inputBytes)
	if err != nil {
		p.lg.Error("Prove failed", "error", err)
		return nil, nil, err
	}
	mask, err := readMask([]byte(publicInputs))
	if err != nil {
		return nil, nil, err
	}
	p.lg.Debug("Generate zk proof", "mask", mask)
	return proof, mask, nil
}

func (p *ZKProverGo) prove(inputBytes []byte) ([]byte, string, error) {
	parsedInputs, err := witness.ParseInputs(inputBytes)
	if err != nil {
		p.lg.Error("Parse input failed", "error", err)
		return nil, "", err
	}
	calc, err := witness.NewCalculator(p.wasm, witness.WithWasmEngine(wazero.NewCircom2WZWitnessCalculator))
	if err != nil {
		p.lg.Error("Create witness calculator failed", "error", err)
		return nil, "", err
	}
	wtnsBytes, err := calc.CalculateWTNSBin(parsedInputs, true)
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
	proof, err := readProof([]byte(proofRaw))
	if err != nil {
		p.lg.Error("Read proof failed", "error", err)
		return nil, "", err
	}
	return proof, publicInputs, nil
}
