// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

type KZGPoseidonProver struct {
	dir, zkey string
	version   uint64
	lg        log.Logger
}

// Prover that can be used directly by miner to prove both KZG and Poseidon hash
// workingDir specifies the working directory of the command relative to the caller.
// zkeyFileName specifies the zkey file name used by snarkjs to generate snark proof
// returns a prover that can generate a combined KZG + zk proof
func NewKZGPoseidonProver(workingDir, zkeyFileName string, version uint64, lg log.Logger) KZGPoseidonProver {
	return KZGPoseidonProver{
		dir:     workingDir,
		version: version,
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
		for i, encodingKey := range encodingKeys {
			zkProof, mask, err := NewZKProver(p.dir, p.zkey, p.lg).GenerateZKProofPerSample(encodingKey, sampleIdxInKv[i])
			if err != nil {
				return nil, nil, nil, err
			}
			zkProofs = append(zkProofs, zkProof)
			masks = append(masks, mask)
		}
	} else if p.version == 2 {
		zkProof, msks, err := NewZKProver(p.dir, p.zkey, p.lg).GenerateZKProof(encodingKeys, sampleIdxInKv)
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
