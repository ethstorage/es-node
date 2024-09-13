// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var maxUint256 = new(big.Int).Sub(new(big.Int).Exp(new(big.Int).SetUint64(2),
	new(big.Int).SetUint64(256), nil), new(big.Int).SetUint64(1))

type SampleReader func(uint64, uint64) (common.Hash, error)

func hashimoto(kvEntriesBits, kvSizeBits, sampleSizeBits, shardIdx, randomChecks uint64, sampleReader SampleReader, hash0 common.Hash) (common.Hash, []uint64, error) {
	var sampleIdxs []uint64
	rowBits := kvEntriesBits + kvSizeBits - sampleSizeBits
	for i := uint64(0); i < randomChecks; i++ {
		parent := big.NewInt(0)
		parent.Mod(new(big.Int).SetBytes(hash0.Bytes()), big.NewInt(1<<rowBits))
		sampleIdx := parent.Uint64() + shardIdx<<rowBits
		encodedSample, err := sampleReader(shardIdx, sampleIdx)
		if err != nil {
			return common.Hash{}, nil, err
		}
		hash0 = crypto.Keccak256Hash(hash0.Bytes(), encodedSample.Bytes())
		sampleIdxs = append(sampleIdxs, sampleIdx)
	}
	return hash0, sampleIdxs, nil
}

func initHash(miner common.Address, mixedHash common.Hash, nonce uint64) common.Hash {
	return crypto.Keccak256Hash(
		common.BytesToHash(miner.Bytes()).Bytes(),
		mixedHash.Bytes(),
		common.BigToHash(new(big.Int).SetUint64(nonce)).Bytes(),
	)
}

func expectedDiff(interval uint64, difficulty, cutoff, diffAdjDivisor, minDiff *big.Int) *big.Int {
	diff := new(big.Int).Set(difficulty)
	x := interval / cutoff.Uint64()
	if x == 0 {
		// diff = diff + ((1 - interval / _cutoff) * diff) / _diffAdjDivisor;
		adjust := new(big.Int).Div(diff, diffAdjDivisor)
		diff = new(big.Int).Add(diff, adjust)
		if diff.Cmp(minDiff) < 0 {
			diff = minDiff
		}
	} else {
		// diff = diff - ((interval / _cutoff - 1) * diff) / _diffAdjDivisor;
		adjust := new(big.Int).Div(
			new(big.Int).Mul(
				new(big.Int).SetUint64(x-1),
				diff,
			),
			diffAdjDivisor,
		)
		if new(big.Int).Add(adjust, minDiff).Cmp(diff) > 0 {
			diff = minDiff
		} else {
			diff = new(big.Int).Sub(diff, adjust)
		}
	}

	return diff
}
