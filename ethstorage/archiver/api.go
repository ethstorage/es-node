// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/gorilla/mux"
	gkzg "github.com/protolambda/go-kzg/eth"
)

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
		Message: "Blob not stored in EthStorage",
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

// blobSidecarHandler implements the /eth/v1/beacon/blob_sidecars/{id} endpoint, but allows clients to fetch expired blobs.
func (a *API) blobSidecarHandler(w http.ResponseWriter, r *http.Request) {
	a.logger.Info("Blob archiver API request", "url", r.RequestURI)
	result := BlobSidecars{}
	start := time.Now()
	defer func(start time.Time, blobs int) {
		dur := time.Since(start)
		a.logger.Info("Blob archiver API request handled", "blobs", len(result.Data), "took(s)", dur.Seconds())
	}(start, len(result.Data))

	// query execution layer block number from beacon block id
	id := mux.Vars(r)["id"]
	elBlock, kzgCommits, err := a.beaconClient.QueryElBlockNumberAndKzg(id)
	if err != nil {
		a.logger.Error("Failed to get execution block number", "beaconID", id, "err", err)
		errUnknownBlock.write(w)
		return
	}
	a.logger.Info("BeaconID to execution block number", "beaconID", id, "elBlock", elBlock)
	fmt.Printf("QueryElBlockNumberAndKzg took %.1fs\n", time.Since(start).Seconds())

	// filter by indices if provided
	indices := r.URL.Query().Get("indices")
	filteredCommitments, hErr := filterCommitments(kzgCommits, indices)
	if hErr != nil {
		hErr.write(w)
		return
	}
	a.logger.Info("Filtered by indices", "indices", indices, "filtered", len(filteredCommitments))

	// hashToIndex is used to determine correct blob index
	hashToIndex := make(map[common.Hash]uint64)
	for i, c := range filteredCommitments {
		bh := gkzg.KZGToVersionedHash(gkzg.KZGCommitment(common.FromHex(c)))
		hashToIndex[common.Hash(bh)] = uint64(i)
		a.logger.Info("BlobhHash to index", "blobHash", common.Hash(bh), "index", i)
	}

	// get event logs on the block
	blockBN := big.NewInt(int64(elBlock))
	start1 := time.Now()
	events, err := a.l1Source.FilterLogsByBlockRange(blockBN, blockBN, eth.PutBlobEvent)
	if err != nil {
		a.logger.Error("Failed to get events", "err", err)
		errServerError.write(w)
		return
	}
	fmt.Printf("FilterLogsByBlockRange took %.1fs\n", time.Since(start1).Seconds())
	for i, event := range events {
		blobHash := event.Topics[3]
		a.logger.Info("Parsing event", "blobHash", blobHash, "event", fmt.Sprintf("%d of %d", i, len(events)))
		// parse event to get kv_index with queried index
		if index, ok := hashToIndex[blobHash]; ok {
			kvIndex := big.NewInt(0).SetBytes(event.Topics[1][:]).Uint64()
			a.logger.Info("BlobHash matched", "blobHash", blobHash, "index", index, "kvIndex", kvIndex)
			sidecar, hErr := a.buildSideCar(kvIndex, common.FromHex(kzgCommits[index]), blobHash)
			if hErr != nil {
				hErr.write(w)
				return
			}
			sidecar.Index = index
			a.logger.Info("Sidecar built", "index", index, "sidecar", sidecar)
			result.Data = append(result.Data, sidecar)
		}
	}

	if len(result.Data) == 0 {
		a.logger.Info("Not stored by EthStorage", "beaconID", id)
		errBlobNotInES.write(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	encodingErr := json.NewEncoder(w).Encode(result)
	if encodingErr != nil {
		a.logger.Error("Unable to encode blob sidecars to JSON", "err", encodingErr)
		errServerError.write(w)
		return
	}
}

func filterCommitments(commitments []string, indices string) ([]string, *httpError) {
	if indices == "" {
		return commitments, nil
	}
	splits := strings.Split(indices, ",")
	if len(splits) == 0 {
		return commitments, nil
	}
	filtered := make([]string, 0)
	for _, index := range splits {
		parsedInt, err := strconv.ParseUint(index, 10, 64)
		if err != nil {
			return nil, newIndicesError(index)
		}
		if parsedInt >= uint64(len(commitments)) {
			return nil, newOutOfRangeError(parsedInt, len(commitments))
		}
		filtered = append(filtered, commitments[parsedInt])
	}
	return filtered, nil
}

func (a *API) buildSideCar(kvIndex uint64, kzgCommitment []byte, blobHash common.Hash) (*BlobSidecar, *httpError) {
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		a.logger.Info("buildSideCar", "took(s)", dur.Seconds())
	}(start)

	blobData, found, err := a.storageMgr.TryRead(kvIndex, int(a.storageMgr.MaxKvSize()), blobHash)
	if err != nil {
		a.logger.Error("Failed to read blob", "err", err)
		return nil, errServerError
	}
	if !found {
		a.logger.Info("Blob not found", "kvIndex", kvIndex, "blobHash", blobHash)
		return nil, errBlobNotInES
	}
	fmt.Printf("storageMgr.TryRead took %.1fs\n", time.Since(start).Seconds())
	start1 := time.Now()
	kzgProof, err := kzg4844.ComputeBlobProof(kzg4844.Blob(blobData), kzg4844.Commitment(kzgCommitment))
	if err != nil {
		a.logger.Error("Failed to get kzg proof", "err", err)
		return nil, errServerError
	}
	fmt.Printf("ComputeBlobProof took %.1fs\n", time.Since(start1).Seconds())
	return &BlobSidecar{
		Blob:          [BlobLength]byte(blobData),
		KZGCommitment: [48]byte(kzgCommitment),
		KZGProof:      kzgProof,
	}, nil
}
