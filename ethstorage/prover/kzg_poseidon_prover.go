// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

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
func (p *KZGPoseidonProver) GetStorageProof(data []byte, encodingKey common.Hash, sampleIdxInKv uint64) ([]byte, error) {
	peInput, err := NewKZGProver(p.lg).GenerateKZGProof(data, sampleIdxInKv)
	if err != nil {
		return nil, err
	}
	zkProof, mask, err := NewZKProver(p.dir, p.zkey, p.lg).GenerateZKProof(encodingKey, sampleIdxInKv)
	if err != nil {
		return nil, err
	}
	proofs := combineProofs(mask, zkProof, peInput)
	return proofs, nil
}

// GenerateZKProofs generates zkProofs in batch for multiple samples
// The test result shows no performance gain vs sequential processing so it is currently not used by miner
func (p *KZGPoseidonProver) GenerateZKProofs(encodingKeys []common.Hash, sampleIdxes []uint64) ([]ZKProof, []common.Hash, error) {
	if len(encodingKeys) != len(sampleIdxes) {
		return nil, nil, fmt.Errorf("same length of encoding key and sample index is required")
	}
	proofSize := len(sampleIdxes)
	var wg sync.WaitGroup
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		p.lg.Info("Generate zk proofs done", "proofSize", proofSize, "took(sec)", dur.Seconds())
	}(start)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	zkPrvr := NewZKProver(p.dir, p.zkey, p.lg)
	resultCh := make(chan []interface{}, proofSize)
	defer close(resultCh)
	for i, cix := range sampleIdxes {
		wg.Add(1)
		go func(index int, sampleIdx uint64, encodingKey common.Hash) {
			defer wg.Done()
			proof, mask, err := zkPrvr.GenerateZKProof(encodingKey, sampleIdx)
			if err != nil {
				p.lg.Error("Generate zk proof error", "sampleIdx", sampleIdx, "error", err)
				cancel()
				return
			}
			resultCh <- []interface{}{index, proof, mask}
		}(i, cix, encodingKeys[i])
	}
	wg.Wait()
	err := ctx.Err()
	if err != nil {
		p.lg.Error("Generate zk proofs ctx.Err() catched", "error", err)
		return nil, nil, err
	}
	zkps := make([]ZKProof, proofSize)
	masks := make([]common.Hash, proofSize)
	for i := 0; i < proofSize; i++ {
		temp := <-resultCh
		index := temp[0].(int)
		zkps[index] = temp[1].(ZKProof)
		masks[index] = temp[2].(common.Hash)
		p.lg.Debug("Generate zk proofs", "index", index, "encodingKey", encodingKeys[index], "sampleIdx", sampleIdxes[index], "mask", masks[index])
		proofB, _ := json.MarshalIndent(zkps[index], "", " ")
		p.lg.Debug("Generate zk proofs", "index", index, "proof", proofB)
	}
	return zkps, masks, nil
}

func combineProofs(mask common.Hash, zkProof ZKProof, peInput []byte) []byte {
	uint256Type, _ := abi.NewType("uint256", "", nil)
	bytesType, _ := abi.NewType("bytes", "", nil)
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
	proofs, _ := abi.Arguments{
		{Type: proofType},
		{Type: uint256Type},
		{Type: bytesType},
	}.Pack(
		zkProof,
		new(big.Int).SetBytes(mask.Bytes()),
		peInput,
	)
	return proofs
}
