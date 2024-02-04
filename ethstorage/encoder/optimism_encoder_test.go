// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package encoder

import (
	"bytes"
	"testing"
)

func TestOpEncodeAndDecode(t *testing.T) {
	data := make([]byte, 4096*31)
	for i := 0; i < 4096*31; i++ {
		data[i] = byte(i % 31)
	}

	blob, err := FromData(data)
	if err != nil {
		t.Errorf("Encode Op Error: %s", err.Error())
	}

	rawData, err := ToData(blob)
	if err != nil {
		t.Errorf("Decode Op Error: %s", err.Error())
	}
	if !bytes.Equal(data[:], rawData[:]) {
		t.Errorf("Decode Op fail!")
	}
}
