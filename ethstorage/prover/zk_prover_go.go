// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/crate-crypto/go-proto-danksharding-crypto/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

type ZKProverGo struct {
	zkey []byte
	wasm []byte
	lg   log.Logger
}

func NewZKProverGo(libDir, zkeyFile, wasmName string, lg log.Logger) (*ZKProverGo, error) {
	wasmBytes, err := os.ReadFile(filepath.Join(libDir, wasmName))
	if err != nil {
		lg.Error("Read wasm file failed", "error", err)
		return nil, err
	}
	zkey, err := os.ReadFile(zkeyFile)
	if err != nil {
		lg.Error("Read zkey file failed", "error", err)
		return nil, err
	}
	return &ZKProverGo{
		zkey: zkey,
		wasm: wasmBytes,
		lg:   lg,
	}, nil
}

// Generate ZK Proof for the given encoding key and sample index
func (p *ZKProverGo) GenerateZKProof(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error) {
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
	return nil, "", nil
}
