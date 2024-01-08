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

const (
	SnarkLib  = "snark_lib"
	WasmName  = "blob_poseidon.wasm"
	Wasm2Name = "blob_poseidon2.wasm"
)

type KZGPoseidonProver struct {
	version uint64
	libDir  string
	zkey    string
	lg      log.Logger
}

// Prover that can be used directly by miner to prove both KZG and Poseidon hash
// workingDir specifies the working directory of the command relative to the caller.
// zkeyFileName specifies the zkey file name to generate snark proof
// returns a prover that can generate a combined KZG + zk proof
func NewKZGPoseidonProver(workingDir, zkeyFileName string, version uint64, lg log.Logger) KZGPoseidonProver {
	libDir := filepath.Join(workingDir, SnarkLib)
	if _, err := os.Stat(libDir); errors.Is(err, os.ErrNotExist) {
		lg.Crit("Init ZK prover failed", "error", "snark lib does not exist", "dir", libDir)
	}
	zkeyFile := filepath.Join(libDir, zkeyFileName)
	if _, err := os.Stat(zkeyFile); errors.Is(err, os.ErrNotExist) {
		lg.Crit("Init ZK prover failed", "error", "zkey does not exist", "dir", zkeyFile)
	}
	var wasmFile string
	if version == 2 {
		wasmFile = filepath.Join(libDir, wasm2Name)
	} else if version == 1 {
		wasmFile = filepath.Join(libDir, wasmName)
	} else {
		lg.Crit("Init ZK prover failed", "error", "invalid version", "version", version)
	}
	if _, err := os.Stat(zkeyFile); errors.Is(err, os.ErrNotExist) {
		lg.Crit("Init ZK prover failed", "error", "wasm does not exist", "dir", wasmFile)
	}
	return KZGPoseidonProver{
		version: version,
		libDir:  libDir,
		zkey:    zkeyFileName,
		lg:      lg,
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
	var zkProofs [][]byte
	var masks []*big.Int
	if p.version == 1 {
		prvr, err := NewZKProver(p.libDir, p.zkey, wasmName, p.lg)
		if err != nil {
			return nil, nil, nil, err
		}
		for i, encodingKey := range encodingKeys {
			zkProof, mask, err := prvr.GenerateZKProofPerSample(encodingKey, sampleIdxInKv[i])
			if err != nil {
				return nil, nil, nil, err
			}
			zkProofs = append(zkProofs, zkProof)
			masks = append(masks, mask)
		}
	} else if p.version == 2 {
		prvr, err := NewZKProver(p.libDir, p.zkey, wasm2Name, p.lg)
		if err != nil {
			return nil, nil, nil, err
		}
		zkProof, msks, err := prvr.GenerateZKProof(encodingKeys, sampleIdxInKv)
		if err != nil {
			return nil, nil, nil, err
		}
		zkProofs = append(zkProofs, zkProof)
		masks = msks
	} else {
		return nil, nil, nil, fmt.Errorf("invalid zk proof version")
	}
	return masks, zkProofs, peInputs, nil
}
