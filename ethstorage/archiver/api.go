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
	if hErr := validateBlockID(id); hErr != nil {
		return nil, hErr
	}
	// query EL block
	queryUrl, err := a.beaconClient.QueryUrlForV2BeaconBlock(id)
	if err != nil {
		a.logger.Error("Invalid beaconID", "beaconID", id, "err", err)
		return nil, errUnknownBlock
	}
	elBlock, kzgCommitsAll, hErr := queryElBlockNumberAndKzg(queryUrl)
	if hErr != nil {
		a.logger.Error("Failed to get execution block number", "beaconID", id, "err", err)
		return nil, hErr
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
	var res BlobSidecars
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
			res.Data = append(res.Data, sidecar)
		}
	}
	a.logger.Info("Query blob sidecars done", "blobs", len(res.Data))
	return &res, nil
}

func queryElBlockNumberAndKzg(queryUrl string) (uint64, []string, *httpError) {
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

func validateBlockID(id string) *httpError {
	if isHash(id) || isSlot(id) || isKnownIdentifier(id) {
		return nil
	}
	return newBlockIdError(id)
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
