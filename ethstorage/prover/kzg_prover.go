// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"bytes"
	"fmt"
	"math/big"
	"math/bits"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gokzg4844 "github.com/crate-crypto/go-kzg-4844"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/protolambda/go-kzg/eth"
	"github.com/status-im/keycard-go/hexutils"
)

const (
	ruBLS    = "0x564c0a11a0f704f4fc3e8acfe0f8245f0ad1347b378fbf96e206da11a5d36306"
	blobSize = gokzg4844.ScalarsPerBlob * gokzg4844.SerializedScalarSize
)

type KZGProver struct {
	ctx *gokzg4844.Context
	ru  fr.Element
	lg  log.Logger
}

func NewKZGProver(lg log.Logger) *KZGProver {
	ctx, err := gokzg4844.NewContext4096Insecure1337()
	if err != nil {
		lg.Crit("failed to init KZG prover", "error", err)
	}
	var ru fr.Element
	ru.SetString(ruBLS)
	return &KZGProver{ctx, ru, lg}
}

func (p *KZGProver) GetProof(data []byte, nChunkBits, chunkIdx, chunkSize uint64) ([]byte, error) {
	return p.GenerateKZGProof(data, chunkIdx)
}

func (p *KZGProver) GetRoot(data []byte, chunkPerKV, chunkSize uint64) (common.Hash, error) {
	if len(data) == 0 {
		return common.Hash{}, nil
	}
	if len(data) != blobSize {
		return common.Hash{}, fmt.Errorf("invalid blob size: %v", len(data))
	}
	var blob gokzg4844.Blob
	copy(blob[:], data)
	commitment, err := p.ctx.BlobToKZGCommitment(blob, -1)
	if err != nil {
		return common.Hash{}, fmt.Errorf("could not convert blob to commitment: %v", err)
	}
	versionedHash := eth.KZGToVersionedHash(eth.KZGCommitment(commitment))
	return common.BytesToHash(versionedHash[:]), nil
}

func (p *KZGProver) GetRootWithProof(dataHash common.Hash, chunkIdx uint64, proofs []byte) (common.Hash, error) {
	panic("no implement")
}

// returns []byte point evaluation input
func (p *KZGProver) GenerateKZGProof(data []byte, sampleIdx uint64) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) != blobSize {
		return nil, fmt.Errorf("invalid blob size: %v", len(data))
	}
	if sampleIdx >= gokzg4844.ScalarsPerBlob {
		return nil, fmt.Errorf("sample index out of scope")
	}
	var blob gokzg4844.Blob
	copy(blob[:], data)

	// use bit reverse of sampleIdx to get the right claimedValue
	sampleIdxReversed := reverseBits(sampleIdx)
	var xe fr.Element
	inputPoint := gokzg4844.SerializeScalar(*xe.Exp(p.ru, new(big.Int).SetUint64(sampleIdxReversed)))
	proof, claimedValue, err := p.ctx.ComputeKZGProof(blob, inputPoint, -1)
	if err != nil {
		return nil, fmt.Errorf("failed to compute proofs: %v", err)
	}
	commitment, err := p.ctx.BlobToKZGCommitment(blob, -1)
	if err != nil {
		return nil, fmt.Errorf("could not convert blob to commitment: %v", err)
	}

	versionedHash := eth.KZGToVersionedHash(eth.KZGCommitment(commitment))
	pointEvalInput := bytes.Join(
		[][]byte{
			versionedHash[:],
			inputPoint[:],
			claimedValue[:],
			commitment[:],
			proof[:],
		},
		[]byte{},
	)
	p.lg.Debug("Generate KZG proof", "pointEvalInput", hexutils.BytesToHex(pointEvalInput))
	return pointEvalInput, nil
}

func reverseBits(x uint64) uint64 {
	// The standard library's bits.Reverse64 inverts its input as a 64-bit unsigned integer.
	// However, we need to invert it as a log2(len(list))-bit integer, so we need to correct this by
	// shifting appropriately.
	shiftCorrection := uint64(64 - bits.TrailingZeros64(gokzg4844.ScalarsPerBlob))
	// Find index irev, such that i and irev get swapped
	return bits.Reverse64(x) >> shiftCorrection
}
