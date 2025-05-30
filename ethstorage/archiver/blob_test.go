// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ethstorage/go-ethstorage/ethstorage/blobs"
)

func TestMarshalUnmarshal(t *testing.T) {
	testBlob := BlobSidecar{
		Index:         3,
		KZGCommitment: [48]byte{1, 2, 3},
		KZGProof:      [48]byte{4, 5, 6},
		Blob:          [blobs.BlobLength]byte{7, 8, 9},
	}

	marshalled, err := json.Marshal(testBlob)
	if err != nil {
		t.Fatalf("Error marshaling: %v", err)
	}

	var unmarshaled BlobSidecar
	err = json.Unmarshal(marshalled, &unmarshaled)
	if err != nil {
		t.Fatalf("Error unmarshaling: %v", err)
	}
	t.Log(unmarshaled.String())
	if !reflect.DeepEqual(testBlob, unmarshaled) {
		t.Errorf("Expected %x, got %x", testBlob, unmarshaled)
	}
}
