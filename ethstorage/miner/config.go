// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"math/big"
	"runtime"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
)

type Config struct {
	RandomChecks   uint64
	NonceLimit     uint64
	MinimumDiff    *big.Int
	Cutoff         *big.Int
	DiffAdjDivisor *big.Int

	GasPrice         *big.Int
	PriorityGasPrice *big.Int
	ZKeyFileName     string
	ZKWorkingDir     string
	ThreadsPerShard  uint64
	SignerFnFactory  signer.SignerFactory
	SignerAddr       common.Address
}

var DefaultConfig = Config{
	RandomChecks:   2,
	NonceLimit:     1048576,
	MinimumDiff:    new(big.Int).SetUint64(1000000),
	Cutoff:         new(big.Int).SetUint64(60),
	DiffAdjDivisor: new(big.Int).SetUint64(1024),

	GasPrice:         nil,
	PriorityGasPrice: nil,
	ZKeyFileName:     "blob_poseidon.zkey",
	ZKWorkingDir:     "build/bin",
	ThreadsPerShard:  uint64(2 * runtime.NumCPU()),
}
