// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	gkzg "github.com/protolambda/go-kzg/eth"
)

type API struct {
	beaconClient *eth.BeaconClient
	l1Source     *eth.PollingClient
	storageMgr   *ethstorage.StorageManager
	logger       log.Logger
}

func NewAPI(storageMgr *ethstorage.StorageManager, beaconClient *eth.BeaconClient, l1Source *eth.PollingClient, logger log.Logger) *API {
	return &API{
		storageMgr:   storageMgr,
		beaconClient: beaconClient,
		l1Source:     l1Source,
		logger:       logger,
	}
}

func (a *API) queryBlobSidecars(id string, indices []uint64) (*BlobSidecars, *httpError) {
	// query EL block
	queryUrl, err := a.beaconClient.QueryUrlForV2BeaconBlock(id)
	if err != nil {
		a.logger.Error("Invalid beaconID", "beaconID", id, "err", err)
		return nil, errUnknownBlock
	}
	elBlock, kzgCommitsAll, hErr := a.queryElBlockNumberAndKzg(queryUrl)
	if hErr != nil {
		a.logger.Error("Failed to get execution block number", "beaconID", id, "err", err)
		return nil, hErr
	}
	a.logger.Info("BeaconID to execution block number", "beaconID", id, "elBlock", elBlock)

	blobsInBeacon := len(kzgCommitsAll)
	if indices != nil {
		for _, index := range indices {
			if int(index) >= blobsInBeacon {
				return nil, newOutOfRangeError(index, blobsInBeacon)
			}
		}
	}

	// hashToIndex is used to determine correct blob index
	hashToIndex := make(map[common.Hash]uint64)
	for i, c := range kzgCommitsAll {
		if indexIncluded(uint64(i), indices) {
			bh := gkzg.KZGToVersionedHash(gkzg.KZGCommitment(common.FromHex(c)))
			hashToIndex[common.Hash(bh)] = uint64(i)
		}
	}

	// get event logs on the block
	blockBN := big.NewInt(int64(elBlock))
	events, err := a.l1Source.FilterLogsByBlockRange(blockBN, blockBN, eth.PutBlobEvent)
	if err != nil {
		a.logger.Error("Failed to get events", "err", err)
		return nil, errServerError
	}
	var res BlobSidecars
	for i, event := range events {
		blobHash := event.Topics[3]
		a.logger.Info("Parsing event", "blobHash", blobHash, "event", fmt.Sprintf("%d of %d", i, len(events)))
		// parse event to get kv_index with queried index
		if index, ok := hashToIndex[blobHash]; ok {
			kvIndex := big.NewInt(0).SetBytes(event.Topics[1][:]).Uint64()
			a.logger.Info("Blobhash matched", "blobhash", blobHash, "index", index, "kvIndex", kvIndex)
			sidecar, hErr := a.buildSidecar(kvIndex, common.FromHex(kzgCommitsAll[index]), blobHash)
			if hErr != nil {
				a.logger.Error("Failed to build sidecar", "err", hErr)
				return nil, hErr
			}
			sidecar.Index = index
			a.logger.Info("Sidecar built", "index", index, "sidecar", sidecar)
			res.Data = append(res.Data, sidecar)
		}
	}
	a.logger.Info("Query blob sidecars done", "blobs", len(res.Data))
	return &res, nil
}

func (a *API) queryElBlockNumberAndKzg(queryUrl string) (uint64, []string, *httpError) {
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		a.logger.Info("Query el block number and kzg", "took(s)", dur.Seconds())
	}(start)

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

func (a *API) buildSidecar(kvIndex uint64, kzgCommitment []byte, blobHash common.Hash) (*BlobSidecar, *httpError) {
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		a.logger.Info("Build sidecar", "took(s)", dur.Seconds())
	}(start)

	blobData, found, err := a.storageMgr.TryRead(kvIndex, int(a.storageMgr.MaxKvSize()), blobHash)
	if err != nil {
		a.logger.Error("Failed to read blob", "err", err)
		return nil, errServerError
	}
	if !found {
		a.logger.Info("Blob not found by storage manager", "kvIndex", kvIndex, "blobHash", blobHash)
		return nil, errBlobNotInES
	}

	kzgProof, err := kzg4844.ComputeBlobProof(kzg4844.Blob(blobData), kzg4844.Commitment(kzgCommitment))
	if err != nil {
		a.logger.Error("Failed to get kzg proof", "err", err)
		return nil, errServerError
	}
	return &BlobSidecar{
		Blob:          [BlobLength]byte(blobData),
		KZGCommitment: [48]byte(kzgCommitment),
		KZGProof:      kzgProof,
	}, nil
}
