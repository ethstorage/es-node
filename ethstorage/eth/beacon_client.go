// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package eth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/crate-crypto/go-proto-danksharding-crypto/eth"
	"github.com/ethereum/go-ethereum/common"
)

type BeaconClient struct {
	beaconURL string
	basedTime uint64
	basedSlot uint64
	slotTime  uint64
}

type Blob struct {
	Index         uint64
	VersionedHash common.Hash
	Data          []byte
}

func NewBeaconClient(url string, basedTime uint64, basedSlot uint64, slotTime uint64) *BeaconClient {
	res := &BeaconClient{
		beaconURL: url,
		basedTime: basedTime,
		basedSlot: basedSlot,
		slotTime:  slotTime,
	}
	return res
}

func (c *BeaconClient) Timestamp2Slot(time uint64) uint64 {
	return (time-c.basedTime)/c.slotTime + c.basedSlot
}

func (c *BeaconClient) DownloadBlobs(slot uint64) (map[common.Hash]Blob, *BeaconSidecars, error) {
	// TODO: @Qiang There will be a change to the URL schema and a new indices query parameter
	// We should do the corresponding change when it takes effect, maybe 4844-devnet-6?
	// The details here: https://github.com/sigp/lighthouse/issues/4317
	beaconUrl, err := url.JoinPath(c.beaconURL, fmt.Sprintf("eth/v1/beacon/blob_sidecars/%d", slot))
	if err != nil {
		return nil, nil, err
	}
	resp, err := http.Get(beaconUrl)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	var blobs BlobSidecars
	err = json.NewDecoder(resp.Body).Decode(&blobs)
	if err != nil {
		return nil, nil, err
	}
	res := map[common.Hash]Blob{}
	for _, beaconBlob := range blobs.Data {
		hash, err := KzgToVersionedHash(beaconBlob.KZGCommitment[:])
		if err != nil {
			return nil, nil, err
		}
		res[hash] = Blob{Index: uint64(beaconBlob.Index), VersionedHash: hash, Data: beaconBlob.Blob[:]}
	}

	clHash, err := c.BeaconBlockRootHash(context.Background(), strconv.FormatUint(slot, 10))
	if err != nil {
		return nil, nil, err
	}
	//TODO: save sidecar meta data only
	bsc := &BeaconSidecars{
		BeaconRoot:   clHash,
		BlobSidecars: blobs,
	}

	return res, bsc, nil
}

func KzgToVersionedHash(commit []byte) (common.Hash, error) {
	c := [48]byte{}
	copy(c[:], commit[:])
	return common.Hash(eth.KZGToVersionedHash(c)), nil
}

func (c *BeaconClient) BeaconBlockRootHash(ctx context.Context, block string) (common.Hash, error) {
	beaconUrl, err := url.JoinPath(c.beaconURL, fmt.Sprintf("/eth/v1/beacon/blocks/%s/root", block))
	if err != nil {
		return common.Hash{}, err
	}
	resp, err := http.Get(beaconUrl)
	if err != nil {
		return common.Hash{}, err
	}
	defer resp.Body.Close()

	type Data struct {
		Root string `json:"root"`
	}
	type rootHashResp struct {
		Data Data `json:"data"`
	}

	var respObj rootHashResp
	err = json.NewDecoder(resp.Body).Decode(&respObj)
	if err != nil {
		return common.Hash{}, err
	}
	return common.HexToHash(respObj.Data.Root), nil
}
