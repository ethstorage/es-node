// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestMarshalUnmarshal(t *testing.T) {
	testBlob := BlobSidecar{
		Index:         uint64(3),
		KZGCommitment: [48]byte{1, 2, 3},
		KZGProof:      [48]byte{4, 5, 6},
		Blob:          [BlobLength]byte{7, 8, 9},
	}

	marshalled, err := json.Marshal(testBlob)
	if err != nil {
		t.Errorf("Error marshaling: %v", err)
	}

	var unmarshaled BlobSidecar
	err = json.Unmarshal(marshalled, &unmarshaled)
	if err != nil {
		t.Errorf("Error unmarshaling: %v", err)
	}
	fmt.Println(unmarshaled.String())
	if !reflect.DeepEqual(testBlob, unmarshaled) {
		t.Errorf("Expected %v, got %v", testBlob, unmarshaled)
	}
}
