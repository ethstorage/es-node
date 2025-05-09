// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package eth

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
)

type DAClient struct {
	daURL string
}

func NewDAClient(url string) *DAClient {
	res := &DAClient{
		daURL: url,
	}
	return res
}

func (c *DAClient) DownloadBlobs(hashes []common.Hash) (map[common.Hash]Blob, error) {
	res := map[common.Hash]Blob{}
	for _, hash := range hashes {
		blob, err := c.DownloadBlob(hash)
		if err != nil {
			return nil, err
		}
		res[hash] = blob
	}
	return res, nil
}

func (c *DAClient) DownloadBlob(hash common.Hash) (Blob, error) {
	// da server
	beaconUrl, err := url.JoinPath(c.daURL, fmt.Sprintf("get/%s", hash.Hex()))
	if err != nil {
		return Blob{}, err
	}
	resp, err := http.Get(beaconUrl)
	if err != nil {
		return Blob{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Blob{}, err
	}
	var blob kzg4844.Blob
	copy(blob[:], data)
	commit, err := kzg4844.BlobToCommitment(&blob)
	if err != nil {
		return Blob{}, fmt.Errorf("blobToCommitment failed: %w", err)
	}
	if common.Hash(kzg4844.CalcBlobHashV1(sha256.New(), &commit)) != hash {
		return Blob{}, fmt.Errorf("invalid blob for %s", hash)
	}
	return Blob{VersionedHash: hash, Data: data}, nil
}
