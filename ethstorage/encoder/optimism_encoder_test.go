// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package encoder

import (
	"bytes"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/params"
	"testing"
)

func EncodeBlobs(data []byte) []kzg4844.Blob {
	blobs := []kzg4844.Blob{{}}
	blobIndex := 0
	fieldIndex := -1
	for i := 0; i < len(data); i += 31 {
		fieldIndex++
		if fieldIndex == params.BlobTxFieldElementsPerBlob {
			blobs = append(blobs, kzg4844.Blob{})
			blobIndex++
			fieldIndex = 0
		}
		max := i + 31
		if max > len(data) {
			max = len(data)
		}
		copy(blobs[blobIndex][fieldIndex*32+1:], data[i:max])
	}
	return blobs
}

func TestOpEncodeAndDecode(t *testing.T) {
	data := make([]byte, 4096*31)
	for i := 0; i < 4096*31; i++ {
		data[i] = byte(i % 32)
	}

	blobs := EncodeBlobs(data)
	blob := blobs[0]

	opData, err := ToData(blob[:])
	if err != nil {
		t.Errorf("Encode Op Error: %s", err.Error())
	}
	t.Log(len(opData))

	fromData, err := FromData(opData)
	if err != nil {
		t.Errorf("Decode Op Error: %s", err.Error())
	}
	if !bytes.Equal(blob[0:], fromData[:]) {
		t.Errorf("Decode Op fail!")
	}
}
