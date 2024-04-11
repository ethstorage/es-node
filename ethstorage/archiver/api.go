// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
	"github.com/gorilla/mux"
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

const (
	jsonAcceptType = "application/json"
	serverTimeout  = 60 * time.Second
)

var (
	errUnknownBlock = &httpError{
		Code:    http.StatusNotFound,
		Message: "Block not found",
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
		Message: fmt.Sprintf("invalid index: %d block contains %d blobs", input, blobCount),
	}
}

type API struct {
	beaconClient *eth.BeaconClient
	l1Source     *eth.PollingClient
	storageMgr   *ethstorage.StorageManager
	prover       *prover.KZGProver
	logger       log.Logger
}

func NewAPI(beaconClient *eth.BeaconClient, l1Source *eth.PollingClient, logger log.Logger) *API {
	result := &API{
		beaconClient: beaconClient,
		l1Source:     l1Source,
		prover:       prover.NewKZGProver(logger),
		logger:       logger,
	}

	return result
}

func isHash(s string) bool {
	if len(s) != 66 || !strings.HasPrefix(s, "0x") {
		return false
	}

	_, err := hexutil.Decode(s)
	return err == nil
}

func isSlot(id string) bool {
	_, err := strconv.ParseUint(id, 10, 64)
	return err == nil
}

func isKnownIdentifier(id string) bool {
	return slices.Contains([]string{"genesis", "finalized", "head"}, id)
}

// blobSidecarHandler implements the /eth/v1/beacon/blob_sidecars/{id} endpoint, using the underlying DataStoreReader
// to fetch blobs instead of the beacon node. This allows clients to fetch expired blobs.
func (a *API) blobSidecarHandler(w http.ResponseWriter, r *http.Request) {
	a.logger.Info("Blob sidecar request", "url", r.RequestURI)
	id := mux.Vars(r)["id"]
	elBlock, err := a.beaconClient.BeaconToExectutionBlockNumber(id)
	if err != nil {
		a.logger.Error("Failed to get execution block number", "beaconID", id, "err", err)
		errUnknownBlock.write(w)
		return
	}
	a.logger.Info("BeaconID to execution block number", "beaconID", id, "elBlock", elBlock)
	blockBN := big.NewInt(int64(elBlock))
	events, err := a.l1Source.FilterLogsByBlockRange(blockBN, blockBN, eth.PutBlobEvent)
	if err != nil {
		a.logger.Error("Failed to get events", "err", err)
		errServerError.write(w)
		return
	}

	result := BlobSidecars{}
	for _, event := range events {
		kvIndex := big.NewInt(0).SetBytes(event.Topics[1][:]).Uint64()
		hash := common.Hash{}
		copy(hash[:], event.Topics[3][:])
		blobData, found, err := a.storageMgr.TryRead(kvIndex, int(a.storageMgr.MaxKvSize()), hash)
		if err != nil {
			a.logger.Error("Failed to read blob", "err", err)
			errServerError.write(w)
			return
		}
		if !found {
			a.logger.Info("Blob not found", "kvIndex", kvIndex, "hash", hash)
			errUnknownBlock.write(w)
			return
		}

		kzgCommitment, kzgProof, err := a.prover.GetKZGCommitmentAndBlobKZGProof(blobData)
		if err != nil {
			a.logger.Error("Failed to get KZG commitment and proof", "err", err)
			errServerError.write(w)
			return
		}

		var blb [BlobLength]byte
		for i := 0; i < len(blobData); i++ {
			blb[i] = blobData[i]
		}
		sidecarOut := &BlobSidecar{
			Blob:          [BlobLength]byte(blb),
			KZGCommitment: kzgCommitment,
			KZGProof:      kzgProof,
		}
		result.Data = append(result.Data, sidecarOut)
	}

	filteredBlobSidecars, hErr := filterBlobs(result.Data, r.URL.Query().Get("indices"))
	if hErr != nil {
		hErr.write(w)
		return
	}
	a.logger.Info("Blob sidecar request handled", "blobs", len(filteredBlobSidecars))
	result.Data = filteredBlobSidecars

	w.Header().Set("Content-Type", jsonAcceptType)
	encodingErr := json.NewEncoder(w).Encode(result)
	if encodingErr != nil {
		a.logger.Error("unable to encode blob sidecars to JSON", "err", encodingErr)
		errServerError.write(w)
		return
	}
}

// filterBlobs filters the blobs based on the indices query provided.
// If no indices are provided, all blobs are returned. If invalid indices are provided, an error is returned.
func filterBlobs(blobs []*BlobSidecar, indices string) ([]*BlobSidecar, *httpError) {
	if indices == "" {
		return blobs, nil
	}

	splits := strings.Split(indices, ",")
	if len(splits) == 0 {
		return blobs, nil
	}

	indicesMap := map[string]struct{}{}
	for _, index := range splits {
		parsedInt, err := strconv.ParseUint(index, 10, 64)
		if err != nil {
			return nil, newIndicesError(index)
		}

		if parsedInt >= uint64(len(blobs)) {
			return nil, newOutOfRangeError(parsedInt, len(blobs))
		}

		blobIndex := strconv.FormatUint(parsedInt, 10)
		indicesMap[blobIndex] = struct{}{}
	}

	filteredBlobs := make([]*BlobSidecar, 0)
	for _, blob := range blobs {
		blobIndex := strconv.FormatUint(blob.Index, 10)
		if _, ok := indicesMap[blobIndex]; ok {
			filteredBlobs = append(filteredBlobs, blob)
		}
	}

	return filteredBlobs, nil
}
