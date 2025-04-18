// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/rpc"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/blobs"
)

var (
	reportCh = make(chan report, 1)
	errCh    = make(chan scanError, 1)
)

type scanError struct {
	kvIndex uint64
	err     error
}

type report struct {
	total      int
	mismatched int
	fixed      int
	failed     int
}

func DownloadBlobFromRPC(rpcEndpoint string, kvIndex uint64, hash common.Hash) ([]byte, error) {
	if rpcEndpoint == "" {
		return nil, fmt.Errorf("RPC endpoint is empty")
	}
	client, err := rpc.DialHTTP(rpcEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to dial RPC: error=%v, rpc=%s", err, rpcEndpoint)
	}
	var result hexutil.Bytes
	// kvIndex, blobHash, encodeType, offset, length
	if err := client.Call(&result, "es_getBlob", kvIndex, hash, 0, 0, blobs.BlobLength); err != nil {
		return nil, err
	}

	var blob kzg4844.Blob
	copy(blob[:], result)
	commit, err := kzg4844.BlobToCommitment(&blob)
	if err != nil {
		return nil, fmt.Errorf("blobToCommitment failed: %w", err)
	}
	cmt := common.Hash(kzg4844.CalcBlobHashV1(sha256.New(), &commit))
	if bytes.Compare(cmt[:es.HashSizeInContract], hash[:es.HashSizeInContract]) != 0 {
		return nil, fmt.Errorf("commit mismatch for blob %d: want: %x, got: %x", kvIndex, hash, cmt)
	}

	return result, nil
}

func shortPrt(arr []uint64) string {
	if len(arr) <= 6 {
		return fmt.Sprintf("%v", arr)
	}
	return fmt.Sprintf("[%d %d %d... %d %d %d]", arr[0], arr[1], arr[2], arr[len(arr)-3], arr[len(arr)-2], arr[len(arr)-1])
}
