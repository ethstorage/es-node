// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

const BlobLength = 131072

type BlobSidecars struct {
	Data []*BlobSidecar `json:"data"`
}

type BlobSidecar struct {
	Index         uint64      `json:"index"`
	KZGCommitment byte48      `json:"kzg_commitment"`
	KZGProof      byte48      `json:"kzg_proof"`
	Blob          blobContent `json:"blob"`
	// signed_block_header and inclusion-proof are ignored compared to beacon API
}

func (a *BlobSidecar) String() string {
	return "BlobSidecar: {" +
		"Index: " + strconv.FormatUint(a.Index, 10) + "," +
		"KZGCommitment: " + hexutil.Encode(a.KZGCommitment[:]) + "," +
		"KZGProof: " + hexutil.Encode(a.KZGProof[:]) + "," +
		"Blob: " + hexutil.Encode(a.Blob[:20]) + "...," +
		"}, "
}

type byte48 [48]byte

func (b byte48) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%#x"`, b)), nil
}

func (b *byte48) UnmarshalJSON(data []byte) error {
	return hexutil.UnmarshalFixedJSON(reflect.TypeOf(b), data, b[:])
}

type blobContent [BlobLength]byte

func (b blobContent) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%#x"`, b)), nil
}

func (b *blobContent) UnmarshalJSON(data []byte) error {
	return hexutil.UnmarshalFixedJSON(reflect.TypeOf(b), data, b[:])
}
