// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

const BlobLength = 131072

type BlobSidecars struct {
	Data []*BlobSidecar `json:"data"`
}

type BlobSidecar struct {
	Index         uint64           `json:"index"`
	KZGCommitment [48]byte         `json:"kzg_commitment"`
	KZGProof      [48]byte         `json:"kzg_proof"`
	Blob          [BlobLength]byte `json:"blob"`
	// signed_block_header and inclusion-proof are ignored
}

func (a *BlobSidecar) String() string {
	return "BlobSidecar{\n" +
		"Index: " + strconv.FormatUint(a.Index, 10) + ",\n " +
		"KZGCommitment: " + hexutil.Encode(a.KZGCommitment[:]) + ",\n " +
		"KZGProof: " + hexutil.Encode(a.KZGProof[:]) + ",\n " +
		"}"
}
