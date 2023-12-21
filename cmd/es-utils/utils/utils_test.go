// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package utils

import (
	"bytes"
	"os"
	"testing"
)

func TestEncodeDecodeBlob(t *testing.T) {
	type gen func(*testing.T, int) []byte
	tests := []gen{
		generateSequentialBytes,
		readTxt,
	}
	for _, tt := range tests {
		data := tt(t, 10*32)
		t.Run("", func(t *testing.T) {
			encoded := EncodeBlobs(data)
			decoded := DecodeBlob(encoded[0][:])
			decoded = decoded[:len(data)]
			if !bytes.Equal(data, decoded) {
				t.Errorf("data:\n%v\nencoded/decoded:\n%v\n", data, decoded)
			}
		})
	}
}

func generateSequentialBytes(t *testing.T, n int) []byte {
	data := make([]byte, n)
	for i := 0; i < n; i++ {
		data[i] = byte(i % 32)
	}
	return data
}

func readTxt(t *testing.T, n int) []byte {
	f, err := os.Open("../blob_8k.dat")
	if err != nil {
		t.Fatal("read file error")
	}
	defer f.Close()

	data := make([]byte, 1024)
	_, err = f.Read(data)
	if err != nil {
		t.Fatal("read txt error")
	}
	return data[:n]
}
