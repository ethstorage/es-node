// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
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

// data: a blob / []byte size of 131072
// encodingKey: a unique key to generate mask
// sampleIdxInKv: sample index in the blob ranges [0, 4095]
// returns a combined proof in the format required by the ethstorage contract, including
// 1. mask, 2. the zk proof of how mask is generated with Poseidon hash,
// and 3. the KZG proof required by point evaluation precompile
func (p *KZGPoseidonProver) GetStorageProof(data [][]byte, encodingKeys []common.Hash, sampleIdxInKv []uint64) ([]byte, error) {
	var peInputs [][]byte
	for i, d := range data {
		peInput, err := NewKZGProver(p.lg).GenerateKZGProof(d, sampleIdxInKv[i])
		if err != nil {
			return nil, err
		}
		peInputs = append(peInputs, peInput)
	}
	zkProof, masks, err := NewZKProver(p.dir, p.zkey, p.lg).GenerateZKProof(encodingKeys, sampleIdxInKv)
	if err != nil {
		return nil, err
	}
	proofs := combineProofs(zkProof, masks, peInputs)
	return proofs, nil
}

func combineProofs(zkProof ZKProof, masks []common.Hash, peInputs [][]byte) []byte {
	proofType := GetAbiTypeOfZkp()
	bytes32ArrayType, _ := abi.NewType("bytes32[]", "", nil)
	bytesType, _ := abi.NewType("bytes", "", nil)
	proofs, _ := abi.Arguments{
		{Type: proofType},
		{Type: bytes32ArrayType},
		{Type: bytesType},
	}.Pack(
		zkProof,
		masks,
		peInputs,
	)
	return proofs
}

func GetAbiTypeOfZkp() abi.Type {
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
	return proofType
}
