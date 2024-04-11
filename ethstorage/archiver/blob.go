// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"encoding/json"
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
	return "BlobSidecar: {" +
		"Index: " + strconv.FormatUint(a.Index, 10) + "," +
		"KZGCommitment: " + hexutil.Encode(a.KZGCommitment[:]) + "," +
		"KZGProof: " + hexutil.Encode(a.KZGProof[:]) + "," +
		"Blob: " + hexutil.Encode(a.Blob[:20]) + "...," +
		"}, "
}

func (a *BlobSidecar) MarshalJSON() ([]byte, error) {
	type Alias BlobSidecar
	return json.Marshal(&struct {
		Index         string `json:"index"`
		KZGCommitment string `json:"kzg_commitment"`
		KZGProof      string `json:"kzg_proof"`
		Blob          string `json:"blob"`
		*Alias
	}{
		Index:         strconv.FormatUint(a.Index, 10),
		KZGCommitment: hexutil.Encode(a.KZGCommitment[:]),
		KZGProof:      hexutil.Encode(a.KZGProof[:]),
		Blob:          hexutil.Encode(a.Blob[:]),
		Alias:         (*Alias)(a),
	})
}

func (a *BlobSidecar) UnmarshalJSON(data []byte) error {
	type Alias BlobSidecar
	aux := &struct {
		Index         string `json:"index"`
		KZGCommitment string `json:"kzg_commitment"`
		KZGProof      string `json:"kzg_proof"`
		Blob          string `json:"blob"`
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
	commitment, err := hexutil.Decode(aux.KZGCommitment)
	if err != nil {
		return err
	}
	proof, err := hexutil.Decode(aux.KZGProof)
	if err != nil {
		return err
	}
	blob, err := hexutil.Decode(aux.Blob)
	if err != nil {
		return err
	}
	copy(a.KZGCommitment[:], commitment)
	copy(a.KZGProof[:], proof)
	copy(a.Blob[:], blob)
	return nil
}
