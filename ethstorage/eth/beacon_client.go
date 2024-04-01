// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package eth

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

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
	VersionedHash common.Hash
	Data          []byte
}

type beaconBlobs struct {
	Data []beaconBlobData `json:"data"`
}

type beaconBlobData struct {
	BlockRoot       string `json:"block_root"`
	Index           string `json:"index"`
	Slot            string `json:"slot"`
	BlockParentRoot string `json:"block_parent_root"`
	ProposerIndex   string `json:"proposer_index"`
	Blob            string `json:"blob"`
	KZGCommitment   string `json:"kzg_commitment"`
	KZGProof        string `json:"kzg_proof"`
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

func (c *BeaconClient) DownloadBlobs(slot uint64) (map[common.Hash]Blob, error) {
	// TODO: @Qiang There will be a change to the URL schema and a new indices query parameter
	// We should do the corresponding change when it takes effect, maybe 4844-devnet-6?
	// The details here: https://github.com/sigp/lighthouse/issues/4317
	beaconUrl, err := url.JoinPath(c.beaconURL, fmt.Sprintf("eth/v1/beacon/blob_sidecars/%d", slot))
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(beaconUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var blobs beaconBlobs
	err = json.NewDecoder(resp.Body).Decode(&blobs)
	if err != nil {
		return nil, err
	}

	res := map[common.Hash]Blob{}
	for _, beaconBlob := range blobs.Data {
		// decode hex string to bytes
		asciiBytes, err := hex.DecodeString(beaconBlob.Blob[2:])
		if err != nil {
			return nil, err
		}
		hash, err := KzgToVersionedHash(beaconBlob.KZGCommitment)
		if err != nil {
			return nil, err
		}
		res[hash] = Blob{VersionedHash: hash, Data: asciiBytes}
	}

	return res, nil
}

func KzgToVersionedHash(commit string) (common.Hash, error) {
	b, err := hex.DecodeString(commit[2:])
	if err != nil {
		return common.Hash{}, err
	}

	c := [48]byte{}
	copy(c[:], b[:])
	return common.Hash(eth.KZGToVersionedHash(c)), nil
}

func (c *BeaconClient) BeaconBlockRootHash(ctx context.Context, block string) (*common.Hash, error) {
	beaconUrl, err := url.JoinPath(c.beaconURL, fmt.Sprintf("/eth/v1/beacon/blocks/%s/root", block))
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(beaconUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var hash common.Hash
	err = json.NewDecoder(resp.Body).Decode(&hash)
	if err != nil {
		return nil, err
	}
	return &hash, nil
}
