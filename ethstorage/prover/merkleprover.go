// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type MerkleProver struct {
}

func (MerkleProver) GetProof(data []byte, nChunkBits, chunkIdx, chunkSize uint64) ([]common.Hash, error) {
	if len(data) == 0 {
		return nil, nil
	}
	nChunks := uint64(1) << nChunkBits
	if chunkIdx >= nChunks {
		return []common.Hash{}, fmt.Errorf("index out of scope")
	}
	nodes := make([]common.Hash, nChunks)
	for i := uint64(0); i < nChunks; i++ {
		off := i * chunkSize
		if off > uint64(len(data)) {
			break
		}
		l := min(uint64(len(data))-off, chunkSize)
		nodes[i] = crypto.Keccak256Hash(data[off : off+l])
	}
	n, proofIdx := nChunks, uint64(0)
	proofs := make([]common.Hash, nChunkBits)
	for n != 1 {
		proofs[proofIdx] = nodes[(chunkIdx/2)*2+1-chunkIdx%2]
		for i := uint64(0); i < n/2; i++ {
			nodes[i] = crypto.Keccak256Hash(nodes[i*2].Bytes(), nodes[i*2+1].Bytes())
		}
		n = n / 2
		chunkIdx = chunkIdx / 2
		proofIdx = proofIdx + 1
	}
	return proofs, nil
}

func (MerkleProver) GetRootWithProof(dataHash common.Hash, chunkIdx uint64, proofs []common.Hash) (common.Hash, error) {
	if len(proofs) == 0 {
		return dataHash, nil
	}
	hash := dataHash
	nChunkBits := uint64(len(proofs))
	if chunkIdx >= uint64(1)<<nChunkBits {
		return common.Hash{}, fmt.Errorf("chunkId overflows")
	}
	for i := uint64(0); i < nChunkBits; i++ {
		if chunkIdx%2 == 0 {
			hash = crypto.Keccak256Hash(hash.Bytes(), proofs[i].Bytes())
		} else {
			hash = crypto.Keccak256Hash(proofs[i].Bytes(), hash.Bytes())
		}
		chunkIdx = chunkIdx / 2
	}
	return hash, nil
}

func (MerkleProver) GetRoot(data []byte, chunkPerKV, chunkSize uint64) common.Hash {
	l := uint64(len(data))
	if l == 0 {
		return common.Hash{}
	}
	nodes := make([]common.Hash, chunkPerKV)
	for i := uint64(0); i < chunkPerKV; i++ {
		off := i * chunkSize
		if off >= l {
			// empty mean the leaf is zero
			break
		}
		size := min(l-off, chunkSize)
		hash := crypto.Keccak256Hash(data[off : off+size])
		nodes[i] = hash
	}
	n := chunkPerKV
	for n != 1 {
		for i := uint64(0); i < n/2; i++ {
			nodes[i] = crypto.Keccak256Hash(nodes[i*2].Bytes(), nodes[i*2+1].Bytes())
		}

		n = n / 2
	}
	return nodes[0]
}
