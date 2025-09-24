//go:build windows

// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"math/big"
	"sync"
)

type ZKProverGo struct {
	zkey []byte
	wasm []byte
	lg   log.Logger
	mu   sync.Mutex
}

// Singleton with a lock is used as RapidsNARK may not be thread-safe at the C++ level.
func NewZKProverGo(libDir, zkeyFile, wasmName string, lg log.Logger) (*ZKProverGo, error) {
	panic("go-rapidsnark do not support windows")
}

// Generate ZK Proof for the given encoding key and sample index
func (p *ZKProverGo) GenerateZKProof(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error) {
	panic("go-rapidsnark do not support windows")
}

func (p *ZKProverGo) GenerateZKProofRaw(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error) {
	panic("go-rapidsnark do not support windows")
}

func (p *ZKProverGo) GenerateZKProofPerSample(encodingKey common.Hash, sampleIdx uint64) ([]byte, *big.Int, error) {
	panic("go-rapidsnark do not support windows")
}

func (p *ZKProverGo) prove(inputBytes []byte) ([]byte, string, error) {
	panic("go-rapidsnark do not support windows")
}
