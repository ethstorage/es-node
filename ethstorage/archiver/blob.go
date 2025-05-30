// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethstorage/go-ethstorage/ethstorage/blobs"
)

type BlobSidecars struct {
	Data []*BlobSidecar `json:"data"`
}

type BlobSidecar struct {
	Index         Index       `json:"index"`
	KZGCommitment byte48      `json:"kzg_commitment"`
	KZGProof      byte48      `json:"kzg_proof"`
	Blob          blobContent `json:"blob"`
	// signed_block_header and inclusion-proof are ignored compared to beacon API
}

func (a *BlobSidecar) String() string {
	jsn, _ := json.MarshalIndent(a, "", "  ")
	return string(jsn)[:512] + "..."
}

type Index uint64

func (i Index) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%d"`, i)), nil
}

func (i *Index) UnmarshalJSON(data []byte) error {
	if !bytes.HasPrefix(data, []byte{'"'}) {
		return errors.New("no prefix \"")
	}
	if !bytes.HasSuffix(data, []byte{'"'}) {
		return errors.New("no suffix \"")
	}

	num, err := strconv.ParseUint(string(data[1:len(data)-1]), 10, 64)
	if err != nil {
		return err
	}
	*i = Index(num)
	return nil
}

type byte48 [48]byte

func (b byte48) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%#x"`, b)), nil
}

func (b *byte48) UnmarshalJSON(data []byte) error {
	return hexutil.UnmarshalFixedJSON(reflect.TypeOf(b), data, b[:])
}

type blobContent [blobs.BlobLength]byte

func (b blobContent) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%#x"`, b)), nil
}

func (b *blobContent) UnmarshalJSON(data []byte) error {
	return hexutil.UnmarshalFixedJSON(reflect.TypeOf(b), data, b[:])
}
