// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package eth

import (
	"github.com/ethereum/go-ethereum/common"
)

const (
	BlobSize                        = 4096 * 32
	kzgCommitmentProofElementLength = 32
	kzgCommitmentProofElements      = 17
	signatureLength                 = 96
)

type BlobSidecarsInput struct {
	Data []*BlobSidecarIn `json:"data"`
}

type BlobSidecarsOutput struct {
	Data []*BlobSidecarOut `json:"data"`
}

type BlobSidecarIn struct {
	KvIndex                     uint64                      `json:"kv_index"`
	Index                       uint64                      `json:"index"`
	KZGCommitment               [48]byte                    `json:"kzg_commitment"`
	KZGProof                    [48]byte                    `json:"kzg_proof"`
	SignedBlockHeader           SignedBeaconBlockHeader     `json:"signed_block_header"`
	KZGCommitmentInclusionProof KZGCommitmentInclusionProof `json:"kzg_commitment_inclusion_proof"`
}

type BlobSidecarOut struct {
	BlobSidecarIn
	Blob [BlobSize]byte `json:"blob"`
}

type SignedBeaconBlockHeader struct {
	Message   BeaconBlockHeader `json:"message"`
	Signature BLSSignature      `json:"signature"`
}

type BeaconBlockHeader struct {
	Slot          uint64      `json:"slot"`
	ProposerIndex uint64      `json:"proposer_index"`
	ParentRoot    common.Hash `json:"parent_root"`
	StateRoot     common.Hash `json:"state_root"`
	BodyRoot      common.Hash `json:"body_root"`
}

type BLSSignature [96]byte
type KZGCommitmentInclusionProof [kzgCommitmentProofElements][kzgCommitmentProofElementLength]byte
