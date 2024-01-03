// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

type KZGPoseidonProver struct {
	dir, zkey string
	lg        log.Logger
}

// Prover that can be used directly by miner to prove both KZG and Poseidon hash
// workingDir specifies the working directory of the command relative to the caller.
// zkeyFileName specifies the zkey file name used by snarkjs to generate snark proof
// returns a prover that can generate a combined KZG + zk proof
func NewKZGPoseidonProver(workingDir, zkeyFileName string, lg log.Logger) KZGPoseidonProver {
	return KZGPoseidonProver{
		dir:  workingDir,
		zkey: zkeyFileName,
		lg:   lg,
	}
}

// data: an array of blob / []byte size of 131072
// encodingKeys: unique keys to generate mask
// sampleIdxInKv: sample indexes in the blob ranges [0, 4095]
// returns
// 1. masks,
// 2. the zk proof of how mask is generated with Poseidon hash,
// 3. the KZG proof required by point evaluation precompile
func (p *KZGPoseidonProver) GetStorageProof(data [][]byte, encodingKeys []common.Hash, sampleIdxInKv []uint64) ([]common.Hash, [][]byte, [][]byte, error) {
	var peInputs [][]byte
	for i, d := range data {
		peInput, err := NewKZGProver(p.lg).GenerateKZGProof(d, sampleIdxInKv[i])
		if err != nil {
			return nil, nil, nil, err
		}
		peInputs = append(peInputs, peInput)
	}
	zkProof, masks, err := NewZKProver(p.dir, p.zkey, p.lg).GenerateZKProof(encodingKeys, sampleIdxInKv)
	if err != nil {
		return nil, nil, nil, err
	}
	// TODO backward compatible of one proof per sample
	var zkProofs [][]byte
	zkProofs = append(zkProofs, zkProof)
	return masks, zkProofs, peInputs, nil
}
