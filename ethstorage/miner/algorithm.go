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

func initHash(miner common.Address, blockHash common.Hash, nonce uint64) common.Hash {
	return crypto.Keccak256Hash(
		common.BytesToHash(miner.Bytes()).Bytes(),
		blockHash.Bytes(),
		common.BigToHash(new(big.Int).SetUint64(nonce)).Bytes(),
	)
}

func expectedDiff(lastMineTime uint64, difficulty *big.Int, minedTime uint64, cutoff, diffAdjDivisor, minDiff *big.Int) *big.Int {
	interval := new(big.Int).SetUint64(minedTime - lastMineTime)
	diff := difficulty
	if interval.Cmp(cutoff) < 0 {
		// diff = diff + (diff-interval*diff/cutoff)/diffAdjDivisor
		diff = new(big.Int).Add(diff, new(big.Int).Div(
			new(big.Int).Sub(diff, new(big.Int).Div(new(big.Int).Mul(interval, diff), cutoff)), diffAdjDivisor))
		if diff.Cmp(minDiff) < 0 {
			diff = minDiff
		}
	} else {
		// dec := (interval*diff/cutoff - diff) / diffAdjDivisor
		dec := new(big.Int).Div(new(big.Int).Sub(new(big.Int).Div(new(big.Int).Mul(interval, diff), cutoff), diff), diffAdjDivisor)
		if new(big.Int).Add(dec, minDiff).Cmp(diff) > 0 {
			diff = minDiff
		} else {
			diff = new(big.Int).Sub(diff, dec)
		}
	}

	return diff
}
