// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package eth

import (
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRoundTrip(t *testing.T) {

	var cmt, proof [48]byte
	copy(cmt[:], common.Hex2Bytes("0xad5d75d9cec4d87686a060fc42b22658404be576113e89e462c41f14c9caa9f74f5c36e9ada58fcea42dbbce261e13e3"))
	copy(proof[:], common.Hex2Bytes("0x90f47f5cf58b581f2ebe01d6a36ccc6a97dc2f589925c236abd0f8ded6abeba2e2b27d6560cd16cecc2401d923b14083"))
	blob := BlobSidecar{
		Index:         4,
		KZGCommitment: cmt,
		KZGProof:      proof,
	}

	jsonData, err := blob.MarshalJSON()
	if err != nil {
		t.Errorf("MarshalJSON error: %v", err)
	}

	var newBlob BlobSidecar
	err = newBlob.UnmarshalJSON(jsonData)
	if err != nil {
		t.Errorf("UnmarshalJSON error: %v", err)
	}

	if !reflect.DeepEqual(blob, newBlob) {
		t.Errorf("Round trip failed. Expected: %v, Got: %v", blob, newBlob)
	}
}
