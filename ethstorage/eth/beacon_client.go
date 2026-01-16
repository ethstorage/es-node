// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package eth

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethstorage/go-ethstorage/ethstorage/blobs"
)

type BeaconClient struct {
	beaconURL       string
	genesisSlotTime uint64
	slotTime        uint64
}

type Blob struct {
	VersionedHash common.Hash
	Data          []byte
}

func NewBeaconClient(url string, slotTime uint64) (*BeaconClient, error) {
	genesisSlotTime, err := queryGenesisTime(url)
	if err != nil {
		return nil, err
	}
	res := &BeaconClient{
		beaconURL:       url,
		genesisSlotTime: genesisSlotTime,
		slotTime:        slotTime,
	}
	return res, nil
}

func queryGenesisTime(beaconUrl string) (uint64, error) {
	queryUrl, err := url.JoinPath(beaconUrl, "/eth/v1/beacon/genesis")
	if err != nil {
		return 0, err
	}
	resp, err := http.Get(queryUrl)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	genesisResponse := &struct {
		Data struct {
			GenesisTime string `json:"genesis_time"`
		} `json:"data"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&genesisResponse)
	if err != nil {
		return 0, err
	}
	gt, err := strconv.ParseUint(genesisResponse.Data.GenesisTime, 10, 64)
	if err != nil {
		return 0, err
	}
	return gt, nil
}

func (c *BeaconClient) Timestamp2Slot(time uint64) uint64 {
	return (time - c.genesisSlotTime) / c.slotTime
}

func (c *BeaconClient) DownloadBlobs(timestamp uint64, hashes []common.Hash) (map[common.Hash]Blob, error) {
	blobsUrl, err := c.QueryUrlForV1BeaconBlobs(timestamp, hashes)
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(blobsUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to query beacon blobs with url %s: %w", blobsUrl, err)
	}
	defer resp.Body.Close()

	var blobsResp blobs.BeaconBlobs
	if err := json.NewDecoder(resp.Body).Decode(&blobsResp); err != nil {
		return nil, fmt.Errorf("failed to decode beacon blobs response from url %s: %w", blobsUrl, err)
	}
	if len(blobsResp.Data) == 0 {
		err := fmt.Sprintf("no blobs found for url %s", blobsUrl)
		if blobsResp.Code != 0 || blobsResp.Message != "" {
			err = fmt.Sprintf("%s: %d %s", err, blobsResp.Code, blobsResp.Message)
		}
		return nil, fmt.Errorf("%s", err)
	}
	res := map[common.Hash]Blob{}
	for _, beaconBlob := range blobsResp.Data {
		// decode hex string to bytes
		asciiBytes, err := hex.DecodeString(beaconBlob[2:])
		if err != nil {
			return nil, fmt.Errorf("failed to decode beacon blob hex string %s: %w", beaconBlob, err)
		}
		hash, err := blobs.BlobToVersionedHash(asciiBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to compute versioned hash for blob: %w", err)
		}
		res[hash] = Blob{VersionedHash: hash, Data: asciiBytes}
	}

	return res, nil
}

func (c *BeaconClient) QueryUrlForV2BeaconBlock(clBlock string) (string, error) {
	return url.JoinPath(c.beaconURL, fmt.Sprintf("/eth/v2/beacon/blocks/%s", clBlock))
}

func (c *BeaconClient) QueryUrlForV1BeaconBlobs(timestamp uint64, hashes []common.Hash) (string, error) {
	slot := c.Timestamp2Slot(timestamp)
	blobsURL, err := url.JoinPath(c.beaconURL, fmt.Sprintf("eth/v1/beacon/blobs/%d", slot))
	if err != nil {
		return "", fmt.Errorf("failed to join URL path for beacon blobs: %w", err)
	}
	q := url.Values{}
	for _, h := range hashes {
		q.Add("versioned_hash", h.Hex())
	}
	return blobsURL + "?" + q.Encode(), nil
}
