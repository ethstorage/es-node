// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package eth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
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
		hash, err := kzgToVersionedHash(beaconBlob.KZGCommitment)
		if err != nil {
			return nil, err
		}
		res[hash] = Blob{VersionedHash: hash, Data: asciiBytes}
	}

	return res, nil
}

func kzgToVersionedHash(commit string) (common.Hash, error) {
	b, err := hex.DecodeString(commit[2:])
	if err != nil {
		return common.Hash{}, err
	}

	c := [48]byte{}
	copy(c[:], b[:])
	cmt := kzg4844.Commitment(c)
	return common.Hash(kzg4844.CalcBlobHashV1(sha256.New(), &cmt)), nil
}

func (c *BeaconClient) QueryUrlForV2BeaconBlock(clBlock string) (string, error) {
	return url.JoinPath(c.beaconURL, fmt.Sprintf("/eth/v2/beacon/blocks/%s", clBlock))
}
