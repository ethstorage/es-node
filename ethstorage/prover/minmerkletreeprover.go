// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

type MinMerkleTreeProver struct {
	MerkleProver
}

func findNChunk(dataLen, chunkSize uint64) (uint64, uint64) {
	if dataLen == 0 {
		return 0, 0
	}
	n := (dataLen+chunkSize-1)/chunkSize - 1
	nChunkBits := uint64(0)
	for n != 0 {
		nChunkBits++
		n = n >> 1
	}

	return uint64(1) << nChunkBits, nChunkBits
}

func (p *MinMerkleTreeProver) GetRootWithProof(dataHash common.Hash, chunkIdx uint64, proofs []common.Hash) (common.Hash, error) {
	return p.MerkleProver.GetRootWithProof(dataHash, chunkIdx, proofs)
}

func (p *MinMerkleTreeProver) GetProof(data []byte, nChunkBits, chunkIdx, chunkSize uint64) ([]common.Hash, error) {
	if len(data) == 0 {
		return []common.Hash{}, nil
	}
	nChunks := uint64(1) << nChunkBits
	if chunkIdx >= nChunks {
		return []common.Hash{}, fmt.Errorf("index out of scope")
	}
	nMinChunks, nMinChunkBits := findNChunk(uint64(len(data)), chunkSize)
	if chunkIdx >= nMinChunks {
		return []common.Hash{}, nil
	}
	return p.MerkleProver.GetProof(data, nMinChunkBits, chunkIdx, chunkSize)
}

func (p *MinMerkleTreeProver) GetRoot(data []byte, nChunk, chunkSize uint64) common.Hash {
	l := uint64(len(data))
	if l == 0 {
		return common.Hash{}
	}
	nMinChunks, _ := findNChunk(uint64(len(data)), chunkSize)
	return p.MerkleProver.GetRoot(data, nMinChunks, chunkSize)
}
