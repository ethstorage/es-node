package archiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/attestantio/go-eth2-client/api"
	"github.com/attestantio/go-eth2-client/spec/deneb"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
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
	retriever    *Retriever
	beaconClient *eth.BeaconClient
	logger       log.Logger
}

func NewAPI(beaconClient *eth.BeaconClient, retriever *Retriever, logger log.Logger) *API {
	result := &API{
		beaconClient: beaconClient,
		retriever:    retriever,
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

// toBeaconBlockHash converts a string that can be a slot, hash or identifier to a beacon block hash.
func (a *API) toBeaconBlockHash(id string) (common.Hash, *httpError) {
	if isHash(id) {
		return common.HexToHash(id), nil
	} else if isSlot(id) || isKnownIdentifier(id) {
		result, err := a.beaconClient.BeaconBlockRootHash(context.Background(), id)
		if err != nil {
			var apiErr *api.Error
			if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
				return common.Hash{}, errUnknownBlock
			}

			return common.Hash{}, errServerError
		}

		return result, nil
	} else {
		return common.Hash{}, newBlockIdError(id)
	}
}

// blobSidecarHandler implements the /eth/v1/beacon/blob_sidecars/{id} endpoint, using the underlying DataStoreReader
// to fetch blobs instead of the beacon node. This allows clients to fetch expired blobs.
func (a *API) blobSidecarHandler(w http.ResponseWriter, r *http.Request) {
	a.logger.Info("Blob sidecar request", "url", r.RequestURI)
	id := mux.Vars(r)["id"]
	beaconBlockHash, err := a.toBeaconBlockHash(id)
	if err != nil {
		a.logger.Error("Failed to get beacon block root hash", "beaconID", id, "err", err)
		err.write(w)
		return
	}
	a.logger.Info("BeaconID to beacon root hash", "beaconID", id, "beaconRoot", beaconBlockHash)
	result, storageErr := a.retriever.GetBlobSidecars(beaconBlockHash)
	if storageErr != nil {
		if errors.Is(storageErr, ethereum.NotFound) {
			a.logger.Info("Blob not found", "beaconRoot", beaconBlockHash)
			errUnknownBlock.write(w)
		} else {
			a.logger.Info("Unexpected error fetching blobs", "err", storageErr, "beaconRoot", beaconBlockHash.String())
			errServerError.write(w)
		}
		return
	}

	filteredBlobSidecars, err := filterBlobs(result.Data, r.URL.Query().Get("indices"))
	if err != nil {
		err.write(w)
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
func filterBlobs(blobs []*deneb.BlobSidecar, indices string) ([]*deneb.BlobSidecar, *httpError) {
	if indices == "" {
		return blobs, nil
	}

	splits := strings.Split(indices, ",")
	if len(splits) == 0 {
		return blobs, nil
	}

	indicesMap := map[deneb.BlobIndex]struct{}{}
	for _, index := range splits {
		parsedInt, err := strconv.ParseUint(index, 10, 64)
		if err != nil {
			return nil, newIndicesError(index)
		}

		if parsedInt >= uint64(len(blobs)) {
			return nil, newOutOfRangeError(parsedInt, len(blobs))
		}

		blobIndex := deneb.BlobIndex(parsedInt)
		indicesMap[blobIndex] = struct{}{}
	}

	filteredBlobs := make([]*deneb.BlobSidecar, 0)
	for _, blob := range blobs {
		if _, ok := indicesMap[blob.Index]; ok {
			filteredBlobs = append(filteredBlobs, blob)
		}
	}

	return filteredBlobs, nil
}
