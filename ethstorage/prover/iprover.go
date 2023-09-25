// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"github.com/ethereum/go-ethereum/common"
)

type IProver interface {
	GetProof(data []byte, nChunkBits, chunkIdx, chunkSize uint64) ([]byte, error)
	GetRoot(data []byte, chunkPerKV, chunkSize uint64) (common.Hash, error)
	GetRootWithProof(dataHash common.Hash, chunkIdx uint64, proofs []byte) (common.Hash, error)
}
