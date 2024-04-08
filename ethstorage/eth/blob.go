// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package eth

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
)

const BlobLength = 131072

type BeaconSidecars struct {
	BeaconRoot common.Hash
	BlobSidecars
}

type BlobSidecarsInput struct {
	BeaconRoot common.Hash
	Data       []*BlobSidecarIn `json:"data"`
}

type BlobSidecars struct {
	Data []*BlobSidecar `json:"data"`
}

// APIBlobSidecars serves blob archiver API service so it contains blob content
type APIBlobSidecars struct {
	Data []*APIBlobSidecar `json:"data"`
}

func (b *APIBlobSidecars) toBlobSidecars() BlobSidecars {
	var blobSidecars BlobSidecars
	for _, v := range b.Data {
		blobSidecars.Data = append(blobSidecars.Data, &v.BlobSidecar)
	}
	return blobSidecars
}

type APIBlobSidecar struct {
	BlobSidecar
	Blob [BlobLength]byte `json:"blob"`
}

func (b *APIBlobSidecar) toBlobSidecar() *BlobSidecar {
	return &BlobSidecar{
		Index:         b.Index,
		KZGCommitment: b.KZGCommitment,
		KZGProof:      b.KZGProof,
	}
}

type BlobSidecarIn struct {
	BlobSidecar
	KvIndex uint64 `json:"kv_index"`
}

type BlobSidecar struct {
	Index         uint64   `json:"index"`
	KZGCommitment [48]byte `json:"kzg_commitment"`
	KZGProof      [48]byte `json:"kzg_proof"`
	// signed_block_header and inclusion-proof are ignored
}

func (a *BlobSidecar) MarshalJSON() ([]byte, error) {
	type Alias BlobSidecar
	return json.Marshal(&struct {
		Index         string `json:"index"`
		KZGCommitment string `json:"kzg_commitment"`
		KZGProof      string `json:"kzg_proof"`
		*Alias
	}{
		Index:         strconv.FormatUint(a.Index, 10),
		KZGCommitment: string(a.KZGCommitment[:]),
		KZGProof:      string(a.KZGProof[:]),
		Alias:         (*Alias)(a),
	})
}

func (a *BlobSidecar) UnmarshalJSON(data []byte) error {
	type Alias BlobSidecar
	aux := &struct {
		Index         string `json:"index"`
		KZGCommitment string `json:"kzg_commitment"`
		KZGProof      string `json:"kzg_proof"`
		*Alias
	}{
		Alias: (*Alias)(a),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	num, err := strconv.ParseUint(aux.Index, 10, 64)
	if err != nil {
		return err
	}
	a.Index = num
	copy(a.KZGCommitment[:], aux.KZGCommitment)
	copy(a.KZGProof[:], aux.KZGProof)

	return nil
}

func format(in []byte) string {
	var out bytes.Buffer
	err := json.Indent(&out, in, "", "\t")
	if err != nil {
		return string(in)
	}
	return out.String()
}
