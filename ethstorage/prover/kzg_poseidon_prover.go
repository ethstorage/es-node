// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

type IZKProver interface {
	// Generate a zk proof for the given encoding keys and samples and return proof and masks (mode 2)
	GenerateZKProof(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error)
	// Generate a zk proof for a given sample (mode 1)
	GenerateZKProofPerSample(encodingKey common.Hash, sampleIdx uint64) ([]byte, *big.Int, error)
	// Generate ZK Proof for the given encoding keys and samples and return proof and all publics (mode 2)
	GenerateZKProofRaw(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error)
}

const (
	SnarkLib  = "snark_lib"
	WasmName  = "blob_poseidon.wasm"
	Wasm2Name = "blob_poseidon2.wasm"
)

type KZGPoseidonProver struct {
	zkProverMode uint64
	zkProverImpl uint64
	libDir       string
	zkey         string
	wasm         string
	lg           log.Logger
}

// Prover that can be used directly by miner to prove both KZG and Poseidon hash
// workingDir specifies the working directory of the command relative to the caller.
// zkeyFile specifies the zkey file with path to generate snark proof
// zkProverMode specifies the mode of the zk prover, 1 for per sample, 2 for samples
// zkProverImpl specifies the implementation of the snark prover, 1 for snarkjs, 2 for go-rapidsnark
// lg specifies the logger to log the info
// returns a prover that can generate a combined KZG + zk proof
func NewKZGPoseidonProver(workingDir, zkeyFile string, zkProverMode, zkProverImpl uint64, lg log.Logger) KZGPoseidonProver {
	// check dependencies when es-node starts
	libDir := filepath.Join(workingDir, SnarkLib)
	if _, err := os.Stat(libDir); errors.Is(err, os.ErrNotExist) {
		lg.Crit("Init ZK prover failed", "error", "snark lib does not exist", "dir", libDir)
	}
	if _, err := os.Stat(zkeyFile); errors.Is(err, os.ErrNotExist) {
		lg.Crit("Init ZK prover failed", "error", "zkey does not exist", "dir", zkeyFile)
	}
	var wasmName string
	if zkProverMode == 2 {
		wasmName = Wasm2Name
	} else if zkProverMode == 1 {
		wasmName = WasmName
	} else {
		lg.Crit("Init ZK prover failed", "error", "invalid zkProverMode", "mode", zkProverMode)
	}
	wasmFile := filepath.Join(libDir, wasmName)
	if _, err := os.Stat(wasmFile); errors.Is(err, os.ErrNotExist) {
		lg.Crit("Init ZK prover failed", "error", "wasm does not exist", "dir", wasmFile)
	}
	return KZGPoseidonProver{
		zkProverMode: zkProverMode,
		zkProverImpl: zkProverImpl,
		libDir:       libDir,
		zkey:         zkeyFile,
		wasm:         wasmName,
		lg:           lg,
	}
}

// data: an array of blob / []byte size of 131072
// encodingKeys: unique keys to generate mask
// sampleIdxInKv: sample indexes in the blob ranges [0, 4095]
// returns
// 1. masks,
// 2. the zk proof of how mask is generated with Poseidon hash,
// 3. the KZG proof required by point evaluation precompile
func (p *KZGPoseidonProver) GetStorageProof(data [][]byte, encodingKeys []common.Hash, sampleIdxInKv []uint64) ([]*big.Int, [][]byte, [][]byte, error) {
	var peInputs [][]byte
	for i, d := range data {
		peInput, err := NewKZGProver(p.lg).GenerateKZGProof(d, sampleIdxInKv[i])
		if err != nil {
			return nil, nil, nil, err
		}
		peInputs = append(peInputs, peInput)
	}
	prvr, err := p.getZKProver()
	if err != nil {
		return nil, nil, nil, err
	}
	var zkProofs [][]byte
	var masks []*big.Int
	if p.zkProverMode == 1 {
		for i, encodingKey := range encodingKeys {
			zkProof, mask, err := prvr.GenerateZKProofPerSample(encodingKey, sampleIdxInKv[i])
			if err != nil {
				return nil, nil, nil, err
			}
			zkProofs = append(zkProofs, zkProof)
			masks = append(masks, mask)
		}
	} else if p.zkProverMode == 2 {
		zkProof, msks, err := prvr.GenerateZKProof(encodingKeys, sampleIdxInKv)
		if err != nil {
			return nil, nil, nil, err
		}
		zkProofs = append(zkProofs, zkProof)
		masks = msks
	} else {
		return nil, nil, nil, fmt.Errorf("invalid zk proof mode")
	}
	return masks, zkProofs, peInputs, nil
}

func (p *KZGPoseidonProver) getZKProver() (IZKProver, error) {
	if p.zkProverImpl == 1 {
		p.lg.Info("Using snarkjs zk prover")
		return NewZKProver(filepath.Dir(p.libDir), p.zkey, p.wasm, p.lg), nil
	}
	if p.zkProverImpl == 2 {
		p.lg.Info("Using go-rapidsnark zk prover")
		return NewZKProverGo(p.libDir, p.zkey, p.wasm, p.lg)
	}
	return nil, fmt.Errorf("invalid zk prover implementation: %d", p.zkProverImpl)
}
