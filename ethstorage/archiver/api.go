// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/blobs"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	gkzg "github.com/protolambda/go-kzg/eth"
)

type API struct {
	beaconClient *eth.BeaconClient
	l1Source     *eth.PollingClient
	storageMgr   *ethstorage.StorageManager
	lg           log.Logger
}

func NewAPI(storageMgr *ethstorage.StorageManager, beaconClient *eth.BeaconClient, l1Source *eth.PollingClient, lg log.Logger) *API {
	return &API{
		storageMgr:   storageMgr,
		beaconClient: beaconClient,
		l1Source:     l1Source,
		lg:           lg,
	}
}

// From the getBlobs spec: https://ethereum.github.io/beacon-APIs/beacon-node-oapi.yaml
// The returned blobs are ordered based on their kzg commitments in the block.
// So EthStorage needs to follow the order of kzg commitments in the block as well.
func (a *API) queryBlobs(id string, hashes []common.Hash) (*blobs.BeaconBlobs, *httpError) {
	// /eth/v2/beacon/blocks
	queryUrl, err := a.beaconClient.QueryUrlForV2BeaconBlock(id)
	if err != nil {
		a.lg.Error("Invalid beaconID", "beaconID", id, "err", err)
		return nil, errUnknownBlock
	}
	elBlock, kzgCommitsAll, hErr := a.queryElBlockNumberAndKzg(queryUrl)
	if hErr != nil {
		a.lg.Error("Failed to get execution block number", "beaconID", id, "err", err)
		return nil, hErr
	}
	a.lg.Info("BeaconID to execution block number", "beaconID", id, "elBlock", elBlock)

	// hashToIndex is used to determine correct blob index
	hashToIndex := make(map[common.Hash]int)
	for _, c := range kzgCommitsAll {
		var commit kzg4844.Commitment
		copy(commit[:], common.FromHex(c))
		vh := common.Hash(kzg4844.CalcBlobHashV1(sha256.New(), &commit))
		if hashIncluded(vh, hashes) {
			hashToIndex[vh] = len(hashToIndex)
		}
	}

	// get event logs on the block
	blockBN := big.NewInt(int64(elBlock))
	events, err := a.l1Source.FilterLogsByBlockRange(blockBN, blockBN, eth.PutBlobEvent)
	if err != nil {
		a.lg.Error("Failed to get events", "err", err)
		return nil, errServerError
	}

	blobsData := make([]string, len(hashToIndex))
	for i, event := range events {
		blobHash := event.Topics[3]
		a.lg.Info("Parsing event", "blobHash", blobHash, "event", fmt.Sprintf("%d of %d", i, len(events)))
		// parse event to get kv_index with queried index
		if index, ok := hashToIndex[blobHash]; ok {
			kvIndex := big.NewInt(0).SetBytes(event.Topics[1][:]).Uint64()
			a.lg.Info("Blobhash matched", "blobhash", blobHash, "index", index, "kvIndex", kvIndex)
			blobData, found, err := a.storageMgr.TryRead(kvIndex, int(a.storageMgr.MaxKvSize()), blobHash)
			if err != nil {
				a.lg.Error("Failed to read blob", "err", err)
				return nil, errServerError
			}
			if !found {
				a.lg.Info("Blob not found by storage manager", "kvIndex", kvIndex, "blobHash", blobHash)
				return nil, errBlobNotInES
			}
			blobsData[index] = "0x" + common.Bytes2Hex(blobData)
		}
	}
	// remove empty entries (not found)
	filteredBlobs := make([]string, 0, len(blobsData))
	for _, data := range blobsData {
		if data != "" {
			filteredBlobs = append(filteredBlobs, data)
		}
	}
	a.lg.Info("Query blobs done", "blobs", len(filteredBlobs))
	return &blobs.BeaconBlobs{Data: filteredBlobs}, nil
}

// Deprecated since Fusaka
func (a *API) queryBlobSidecars(id string, indices []uint64) (*BlobSidecars, *httpError) {
	// query EL block
	queryUrl, err := a.beaconClient.QueryUrlForV2BeaconBlock(id)
	if err != nil {
		a.lg.Error("Invalid beaconID", "beaconID", id, "err", err)
		return nil, errUnknownBlock
	}
	elBlock, kzgCommitsAll, hErr := a.queryElBlockNumberAndKzg(queryUrl)
	if hErr != nil {
		a.lg.Error("Failed to get execution block number", "beaconID", id, "err", err)
		return nil, hErr
	}
	a.lg.Info("BeaconID to execution block number", "beaconID", id, "elBlock", elBlock)

	blobsInBeacon := uint64(len(kzgCommitsAll))
	for _, index := range indices {
		if index >= blobsInBeacon {
			// beacon API will ignore invalid indices and return all blobs
			indices = nil
		}
	}

	// hashToIndex is used to determine correct blob index
	hashToIndex := make(map[common.Hash]Index)
	for i, c := range kzgCommitsAll {
		if indexIncluded(uint64(i), indices) {
			bh := gkzg.KZGToVersionedHash(gkzg.KZGCommitment(common.FromHex(c)))
			hashToIndex[common.Hash(bh)] = Index(i)
		}
	}

	// get event logs on the block
	blockBN := big.NewInt(int64(elBlock))
	events, err := a.l1Source.FilterLogsByBlockRange(blockBN, blockBN, eth.PutBlobEvent)
	if err != nil {
		a.lg.Error("Failed to get events", "err", err)
		return nil, errServerError
	}
	var res BlobSidecars
	for i, event := range events {
		blobHash := event.Topics[3]
		a.lg.Info("Parsing event", "blobHash", blobHash, "event", fmt.Sprintf("%d of %d", i, len(events)))
		// parse event to get kv_index with queried index
		if index, ok := hashToIndex[blobHash]; ok {
			kvIndex := big.NewInt(0).SetBytes(event.Topics[1][:]).Uint64()
			a.lg.Info("Blobhash matched", "blobhash", blobHash, "index", index, "kvIndex", kvIndex)
			sidecar, hErr := a.buildSidecar(kvIndex, common.FromHex(kzgCommitsAll[index]), blobHash)
			if hErr != nil {
				a.lg.Error("Failed to build sidecar", "err", hErr)
				return nil, hErr
			}
			sidecar.Index = index
			a.lg.Info("Sidecar built", "index", index, "sidecar", sidecar)
			res.Data = append(res.Data, sidecar)
		}
	}
	a.lg.Info("Query blob sidecars done", "blobs", len(res.Data))
	return &res, nil
}

func (a *API) queryElBlockNumberAndKzg(queryUrl string) (uint64, []string, *httpError) {
	resp, err := http.Get(queryUrl)
	if err != nil {
		return 0, nil, errServerError
	}
	defer resp.Body.Close()

	respObj := &struct {
		Data struct {
			Message struct {
				Body struct {
					ExecutionPayload struct {
						BlockNumber string `json:"block_number"`
					} `json:"execution_payload"`
					BlobKzgCommitments []string `json:"blob_kzg_commitments"`
				} `json:"body"`
			} `json:"message"`
		} `json:"data"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&respObj)
	if err != nil {
		return 0, nil, errUnknownBlock
	}
	body := respObj.Data.Message.Body
	elBlock, err := strconv.ParseUint(body.ExecutionPayload.BlockNumber, 10, 64)
	if err != nil {
		return 0, nil, errUnknownBlock
	}
	return elBlock, body.BlobKzgCommitments, nil
}

// Deprecated since Fusaka
func (a *API) buildSidecar(kvIndex uint64, kzgCommitment []byte, blobHash common.Hash) (*BlobSidecar, *httpError) {
	blobData, found, err := a.storageMgr.TryRead(kvIndex, int(a.storageMgr.MaxKvSize()), blobHash)
	if err != nil {
		a.lg.Error("Failed to read blob", "err", err)
		return nil, errServerError
	}
	if !found {
		a.lg.Info("Blob not found by storage manager", "kvIndex", kvIndex, "blobHash", blobHash)
		return nil, errBlobNotInES
	}
	blob := kzg4844.Blob(blobData)
	kzgProof, err := kzg4844.ComputeBlobProof(&blob, kzg4844.Commitment(kzgCommitment))
	if err != nil {
		a.lg.Error("Failed to get kzg proof", "err", err)
		return nil, errServerError
	}
	return &BlobSidecar{
		Blob:          [blobs.BlobLength]byte(blobData),
		KZGCommitment: [48]byte(kzgCommitment),
		KZGProof:      [48]byte(kzgProof),
	}, nil
}
