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
	return common.Hash(eth.KZGToVersionedHash(c)), nil
}

func (c *BeaconClient) BeaconBlockRootHash(block string) (common.Hash, error) {
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

func (c *BeaconClient) BeaconToExectutionBlockNumber(clBlock string) (uint64, error) {
	beaconUrl, err := url.JoinPath(c.beaconURL, fmt.Sprintf("/eth/v2/beacon/blocks/%s", clBlock))
	if err != nil {
		return 0, err
	}
	resp, err := http.Get(beaconUrl)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	type rootHashResp struct {
		Data struct {
			Message struct {
				Body struct {
					ExecutionPayload struct {
						BlockNumber string `json:"block_number"`
					} `json:"execution_payload"`
				} `json:"body"`
			} `json:"message"`
		} `json:"data"`
	}

	var respObj rootHashResp
	err = json.NewDecoder(resp.Body).Decode(&respObj)
	if err != nil {
		return 0, err
	}
	elBlock, err := strconv.ParseUint(respObj.Data.Message.Body.ExecutionPayload.BlockNumber, 10, 64)
	if err != nil {
		return 0, err
	}

	return elBlock, nil
}
