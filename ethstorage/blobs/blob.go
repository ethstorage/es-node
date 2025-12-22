// Copyright 2022-2023, es.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package blobs

import (
	"crypto/sha256"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
)

type BeaconBlobs struct {
	Data []string `json:"data"`
}

func BlobToVersionedHash(blobBytes []byte) (common.Hash, error) {
	var blob kzg4844.Blob
	copy(blob[:], blobBytes)
	cmt, err := kzg4844.BlobToCommitment(&blob)
	if err != nil {
		return common.Hash{}, fmt.Errorf("blobToCommitment failed: %w", err)
	}
	return common.Hash(kzg4844.CalcBlobHashV1(sha256.New(), &cmt)), nil
}
