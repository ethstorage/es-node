// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
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
	result := &API{
		storageMgr:   storageMgr,
		beaconClient: beaconClient,
		l1Source:     l1Source,
		logger:       logger,
	}

	return result
}

func (a *API) queryBlobSidecars(id string, indices []uint64) (*BlobSidecars, *httpError) {
	// query EL block
	elBlock, kzgCommitsAll, err := a.beaconClient.QueryElBlockNumberAndKzg(id)
	if err != nil {
		a.logger.Error("Failed to get execution block number", "beaconID", id, "err", err)
		return nil, errUnknownBlock
	}
	a.logger.Info("BeaconID to execution block number", "beaconID", id, "elBlock", elBlock)

	kzgCommits := kzgCommitsAll
	if indices != nil {
		// filter by indices if provided
		var filtered []string
		for _, index := range indices {
			if index >= uint64(len(kzgCommitsAll)) {
				return nil, newOutOfRangeError(index, len(kzgCommitsAll))
			}
			filtered = append(filtered, kzgCommitsAll[index])
		}
		a.logger.Info("Filtered by indices", "indices", indices, "filtered", len(filtered))
		kzgCommits = filtered
	}

	// hashToIndex is used to determine correct blob index
	hashToIndex := make(map[common.Hash]uint64)
	for i, c := range kzgCommits {
		bh := gkzg.KZGToVersionedHash(gkzg.KZGCommitment(common.FromHex(c)))
		hashToIndex[common.Hash(bh)] = uint64(i)
		a.logger.Info("BlobhHash to index", "blobHash", common.Hash(bh), "index", i)
	}

	// get event logs on the block
	blockBN := big.NewInt(int64(elBlock))
	events, err := a.l1Source.FilterLogsByBlockRange(blockBN, blockBN, eth.PutBlobEvent)
	if err != nil {
		a.logger.Error("Failed to get events", "err", err)
		return nil, errServerError
	}
	var result BlobSidecars
	for i, event := range events {
		blobHash := event.Topics[3]
		a.logger.Info("Parsing event", "blobHash", blobHash, "event", fmt.Sprintf("%d of %d", i, len(events)))
		// parse event to get kv_index with queried index
		if index, ok := hashToIndex[blobHash]; ok {
			kvIndex := big.NewInt(0).SetBytes(event.Topics[1][:]).Uint64()
			a.logger.Info("BlobHash matched", "blobHash", blobHash, "index", index, "kvIndex", kvIndex)
			sidecar, hErr := a.buildSidecar(kvIndex, common.FromHex(kzgCommitsAll[index]), blobHash)
			if hErr != nil {
				a.logger.Error("Failed to build sidecar", "err", hErr)
				return nil, hErr
			}
			sidecar.Index = index
			a.logger.Info("Sidecar built", "index", index, "sidecar", sidecar)
			result.Data = append(result.Data, sidecar)
		}
	}
	a.logger.Info("Query blob sidecars done", "blobs", len(result.Data))
	return &result, nil
}

func (a *API) buildSidecar(kvIndex uint64, kzgCommitment []byte, blobHash common.Hash) (*BlobSidecar, *httpError) {
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		a.logger.Info("buildSidecar", "took(s)", dur.Seconds())
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

type httpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e httpError) write(w http.ResponseWriter) {
	w.WriteHeader(e.Code)
	_ = json.NewEncoder(w).Encode(e)
}

func (e httpError) Error() string {
	return e.Message
}

var (
	errUnknownBlock = &httpError{
		Code:    http.StatusNotFound,
		Message: "Block not found",
	}
	errBlobNotInES = &httpError{
		Code:    http.StatusNotFound,
		Message: "Blob not found in EthStorage",
	}
	errServerError = &httpError{
		Code:    http.StatusInternalServerError,
		Message: "Internal server error",
	}
)

func newBlockIdError(input string) *httpError {
	return &httpError{
		Code:    http.StatusBadRequest,
		Message: fmt.Sprintf("invalid block id: %s", input),
	}
}

func newIndicesError(input string) *httpError {
	return &httpError{
		Code:    http.StatusBadRequest,
		Message: fmt.Sprintf("invalid index input: %s", input),
	}
}

func newOutOfRangeError(input uint64, blobCount int) *httpError {
	return &httpError{
		Code:    http.StatusBadRequest,
		Message: fmt.Sprintf("invalid index: %d - block contains %d blobs", input, blobCount),
	}
}
