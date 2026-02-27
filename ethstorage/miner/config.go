// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"math/big"
	"path/filepath"
	"runtime"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethstorage/go-ethstorage/ethstorage/email"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
)

type Config struct {
	// es network chain id
	ChainID *big.Int
	Slot    uint64

	// contract
	RandomChecks   uint64
	NonceLimit     uint64
	StartTime      uint64
	ShardEntry     uint64
	TreasuryShare  uint64
	MinimumDiff    *big.Int
	Cutoff         *big.Int
	DiffAdjDivisor *big.Int
	StorageCost    *big.Int
	PrepaidAmount  *big.Int
	DcfFactor      *big.Int

	// cli
	GasPrice         *big.Int
	PriorityGasPrice *big.Int
	ZKeyFile         string
	ZKWorkingDir     string
	ZKProverMode     uint64
	ZKProverImpl     uint64
	ThreadsPerShard  uint64
	SignerFnFactory  signer.SignerFactory
	SignerAddr       common.Address
	MinimumProfit    *big.Int
	MaxGasPrice      *big.Int

	// for proof submission notifications
	EmailEnabled bool
	EmailConfig  email.EmailConfig
}

var DefaultConfig = Config{
	RandomChecks:   2,
	NonceLimit:     1048576,
	MinimumDiff:    new(big.Int).SetUint64(1000000),
	Cutoff:         new(big.Int).SetUint64(60),
	DiffAdjDivisor: new(big.Int).SetUint64(1024),

	Slot:             12,
	GasPrice:         nil,
	PriorityGasPrice: nil,
	ZKeyFile:         filepath.Join("build", "bin", prover.SnarkLib, "zkey", "blob_poseidon2.zkey"),
	ZKWorkingDir:     filepath.Join("build", "bin"),
	ZKProverMode:     2,
	ZKProverImpl:     1,
	ThreadsPerShard:  uint64(2 * runtime.NumCPU()),
	MinimumProfit:    common.Big0,
	MaxGasPrice:      common.Big0,
}
