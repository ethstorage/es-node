// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"math/big"
	"path/filepath"
	"runtime"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
	"github.com/ethstorage/go-ethstorage/ethstorage/signer"
)

type EmailConfig struct {
	Username string
	Password string
	Host     string
	Port     uint64
	To       []string
	From     string
}

type Config struct {
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

	EmailConfig EmailConfig
}

var DefaultEmailConfig = EmailConfig{
	Username: "web3url.gateway@gmail.com",
	Password: "",
	Host:     "smtp.gmail.com",
	Port:     587,
	To:       []string{},
	From:     "",
}

var DefaultConfig = Config{
	RandomChecks:   2,
	NonceLimit:     1048576,
	MinimumDiff:    new(big.Int).SetUint64(1000000),
	Cutoff:         new(big.Int).SetUint64(60),
	DiffAdjDivisor: new(big.Int).SetUint64(1024),

	GasPrice:         nil,
	PriorityGasPrice: nil,
	ZKeyFile:         filepath.Join("build", "bin", prover.SnarkLib, "zkey", "blob_poseidon2.zkey"),
	ZKWorkingDir:     filepath.Join("build", "bin"),
	ZKProverMode:     2,
	ZKProverImpl:     1,
	ThreadsPerShard:  uint64(2 * runtime.NumCPU()),
	MinimumProfit:    common.Big0,
	EmailConfig:      DefaultEmailConfig,
}
