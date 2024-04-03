// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package eth

import (
	"github.com/attestantio/go-eth2-client/spec/deneb"
	"github.com/ethereum/go-ethereum/common"
)

type BeaconSidecars struct {
	BeaconRoot common.Hash
	BlobSidecars
}

type BlobSidecarsInput struct {
	BeaconRoot common.Hash
	Data       []*BlobSidecarIn `json:"data"`
}

type BlobSidecars struct {
	Data []*deneb.BlobSidecar `json:"data"`
}

// type BlobSidecarEmpty struct {
// 	Index                       string   `json:"index"`
// 	KZGCommitment               string   `json:"kzg_commitment"`
// 	KZGProof                    string   `json:"kzg_proof"`
// 	SignedBlockHeader           string   `json:"signed_block_header"`
// 	KZGCommitmentInclusionProof []string `json:"kzg_commitment_inclusion_proof"`
// }

type BlobSidecarIn struct {
	deneb.BlobSidecar
	KvIndex uint64 `json:"kv_index"`
}
