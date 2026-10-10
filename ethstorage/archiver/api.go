// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/blobs"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	gkzg "github.com/protolambda/go-kzg/eth"
)

const (
	// beaconBlockFetchTimeout bounds a single upstream beacon node request.
	beaconBlockFetchTimeout = 30 * time.Second
	// maxBeaconBlockResponseSize caps how much of a beacon block response we
	// read into memory (a full mainnet block is well under this).
	maxBeaconBlockResponseSize = 64 << 20
	// beaconBlockVersionDeneb .. beaconBlockVersionGloas are the consensus
	// versions whose blocks the archiver knows how to decode.
	beaconBlockVersionDeneb   = "deneb"
	beaconBlockVersionElectra = "electra"
	beaconBlockVersionFulu    = "fulu"
	beaconBlockVersionGloas   = "gloas"
)

// beaconBlock is the execution layer view of a beacon block: the execution
// block hash and the KZG commitments of its blobs, in the order they appear
// in the block. Execution block number is deliberately not part of it: Gloas
// blocks carry a signed execution payload bid instead of an execution payload,
// and the bid has no block number (see consensus-specs gloas/beacon-chain.md).
type beaconBlock struct {
	version       string
	slot          uint64
	executionHash common.Hash
	kzgCommitsAll []string
}

// beaconBlockResponse mirrors the /eth/v2/beacon/blocks/{id} envelope for every
// supported consensus version. Fields are pointers so that a missing field can
// be told apart from an empty one.
type beaconBlockResponse struct {
	Version *string `json:"version"`
	Data    *struct {
		Message *struct {
			Slot *string `json:"slot"`
			Body *struct {
				// Deneb, Electra and Fulu
				ExecutionPayload *struct {
					BlockHash *string `json:"block_hash"`
				} `json:"execution_payload"`
				BlobKzgCommitments *[]string `json:"blob_kzg_commitments"`
				// Gloas (EIP-7732): execution payload and blob commitments moved
				// into the signed bid
				SignedExecutionPayloadBid *struct {
					Message *struct {
						BlockHash          *string   `json:"block_hash"`
						BlobKzgCommitments *[]string `json:"blob_kzg_commitments"`
					} `json:"message"`
				} `json:"signed_execution_payload_bid"`
			} `json:"body"`
		} `json:"message"`
	} `json:"data"`
}

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
func (a *API) queryBlobs(ctx context.Context, id string, hashes []common.Hash) (*blobs.BeaconBlobs, *httpError) {
	block, hErr := a.queryBeaconBlock(ctx, id)
	if hErr != nil {
		return nil, hErr
	}

	// hashToIndex is used to determine correct blob index
	hashToIndex := make(map[common.Hash]int)
	for _, c := range block.kzgCommitsAll {
		var commit kzg4844.Commitment
		copy(commit[:], common.FromHex(c))
		vh := common.Hash(kzg4844.CalcBlobHashV1(sha256.New(), &commit))
		if hashIncluded(vh, hashes) {
			hashToIndex[vh] = len(hashToIndex)
		}
	}

	// get event logs on the execution block
	events, hErr := a.queryPutBlobEvents(ctx, id, block)
	if hErr != nil {
		return nil, hErr
	}

	blobsData := make([]string, len(hashToIndex))
	for i, event := range events {
		blobHash := event.Topics[3]
		a.lg.Info("Parsing event", "blobHash", blobHash, "event", fmt.Sprintf("%d of %d", i, len(events)))
		// parse event to get kv_index with queried index
		if index, ok := hashToIndex[blobHash]; ok {
			kvIndex := big.NewInt(0).SetBytes(event.Topics[1][:]).Uint64()
			a.lg.Info("Blobhash matched", "blobhash", blobHash, "index", index, "kvIndex", kvIndex)
			blobData, hErr := a.readBlob(kvIndex, blobHash)
			if hErr != nil {
				return nil, hErr
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
	a.lg.Info("Query blobs done", "beaconID", id, "blobs", len(filteredBlobs))
	return &blobs.BeaconBlobs{Data: filteredBlobs}, nil
}

// Deprecated since Fusaka
func (a *API) queryBlobSidecars(ctx context.Context, id string, indices []uint64) (*BlobSidecars, *httpError) {
	block, hErr := a.queryBeaconBlock(ctx, id)
	if hErr != nil {
		return nil, hErr
	}
	kzgCommitsAll := block.kzgCommitsAll

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

	// get event logs on the execution block
	events, hErr := a.queryPutBlobEvents(ctx, id, block)
	if hErr != nil {
		return nil, hErr
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
	a.lg.Info("Query blob sidecars done", "beaconID", id, "blobs", len(res.Data))
	return &res, nil
}

// queryBeaconBlock fetches a beacon block and reduces it to the execution block
// hash plus its ordered KZG commitments, whatever the consensus version is.
func (a *API) queryBeaconBlock(ctx context.Context, id string) (*beaconBlock, *httpError) {
	// /eth/v2/beacon/blocks
	queryUrl, err := a.beaconClient.QueryUrlForV2BeaconBlock(id)
	if err != nil {
		a.lg.Error("Invalid beaconID", "beaconID", id, "err", err)
		return nil, errUnknownBlock
	}
	// fetchBeaconBlock bounds the request with its own context timeout, so the
	// shared client needs no timeout of its own.
	block, hErr := fetchBeaconBlock(ctx, http.DefaultClient, queryUrl)
	if hErr != nil {
		a.lg.Error("Failed to get beacon block", "beaconID", id, "err", hErr)
		return nil, hErr
	}
	a.lg.Info("BeaconID to execution block", "beaconID", id, "version", block.version,
		"slot", block.slot, "elBlockHash", block.executionHash, "blobsInBlock", len(block.kzgCommitsAll))
	return block, nil
}

// queryPutBlobEvents returns the PutBlob events of the execution block, ordered
// as the EL reports them. Querying by block hash (EIP-234) rather than by block
// number keeps the events aligned with the commitments of the beacon block even
// during reorgs.
func (a *API) queryPutBlobEvents(ctx context.Context, id string, block *beaconBlock) ([]types.Log, *httpError) {
	events, err := a.l1Source.FilterLogsByBlockHash(ctx, block.executionHash, eth.PutBlobEvent)
	if err != nil {
		if isUnknownBlockErr(err) {
			// the execution block the bid points to is not available on the EL yet
			a.lg.Warn("Execution block not available on L1", "beaconID", id,
				"version", block.version, "elBlockHash", block.executionHash, "err", err)
			return nil, errExecutionBlockNotAvailable
		}
		a.lg.Error("Failed to get events", "beaconID", id, "elBlockHash", block.executionHash, "err", err)
		return nil, errServerError
	}
	return events, nil
}

// fetchBeaconBlock requests a /eth/v2/beacon/blocks/{id} response and decodes it
// into a beaconBlock. Error classification:
//   - 404 from the beacon node: the block does not exist
//   - transport errors, timeouts, >=500, invalid JSON: upstream error (5xx)
//   - unknown version, or a known version missing required fields: upstream
//     error (5xx), never a silent "block not found"
func fetchBeaconBlock(ctx context.Context, client *http.Client, queryUrl string) (*beaconBlock, *httpError) {
	ctx, cancel := context.WithTimeout(ctx, beaconBlockFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryUrl, nil)
	if err != nil {
		return nil, errServerError
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errUpstreamError
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errUnknownBlock
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errUpstreamError
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBeaconBlockResponseSize))
	if err != nil {
		return nil, errUpstreamError
	}
	return parseBeaconBlock(body)
}

// parseBeaconBlock decodes a /eth/v2/beacon/blocks/{id} JSON body. It is split
// out from the HTTP fetch so that fixtures of every consensus version can be
// tested without a network.
func parseBeaconBlock(body []byte) (*beaconBlock, *httpError) {
	respObj := &beaconBlockResponse{}
	if err := json.Unmarshal(body, respObj); err != nil {
		return nil, errUpstreamError
	}
	if respObj.Data == nil || respObj.Data.Message == nil || respObj.Data.Message.Body == nil {
		return nil, errUpstreamError
	}
	version := strings.ToLower(strings.TrimSpace(deref(respObj.Version)))
	if version == "" {
		// the beacon node did not tell us which version this block is
		return nil, errUpstreamError
	}
	bodyObj := respObj.Data.Message.Body
	slot, hErr := parseSlot(respObj.Data.Message.Slot)
	if hErr != nil {
		return nil, hErr
	}

	var (
		blockHashStr *string
		commits      *[]string
	)
	switch version {
	case beaconBlockVersionDeneb, beaconBlockVersionElectra, beaconBlockVersionFulu:
		if bodyObj.ExecutionPayload == nil {
			return nil, errUpstreamError
		}
		blockHashStr = bodyObj.ExecutionPayload.BlockHash
		commits = bodyObj.BlobKzgCommitments
	case beaconBlockVersionGloas:
		// Gloas has no execution_payload in the body: the execution block hash
		// and the commitments live in the signed bid's message.
		if bodyObj.SignedExecutionPayloadBid == nil || bodyObj.SignedExecutionPayloadBid.Message == nil {
			return nil, errUpstreamError
		}
		blockHashStr = bodyObj.SignedExecutionPayloadBid.Message.BlockHash
		commits = bodyObj.SignedExecutionPayloadBid.Message.BlobKzgCommitments
	default:
		return nil, errUnsupportedBeaconVersion
	}

	if blockHashStr == nil {
		return nil, errUpstreamError
	}
	executionHash, err := decodeBlockHash(*blockHashStr)
	if err != nil {
		return nil, errUpstreamError
	}
	// blob_kzg_commitments is required by the beacon API schema for every
	// supported version: a block without blobs serves an explicit empty array,
	// so a missing or null field means the upstream response is malformed.
	// Treating it as "no blobs" would report a valid stored blob as not found.
	if commits == nil {
		return nil, errUpstreamError
	}
	kzgCommitsAll := *commits
	for _, c := range kzgCommitsAll {
		if _, err := decodeKZGCommitment(c); err != nil {
			return nil, errUpstreamError
		}
	}
	return &beaconBlock{
		version:       version,
		slot:          slot,
		executionHash: executionHash,
		kzgCommitsAll: kzgCommitsAll,
	}, nil
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func parseSlot(slot *string) (uint64, *httpError) {
	if slot == nil {
		return 0, errUpstreamError
	}
	v, err := strconv.ParseUint(*slot, 10, 64)
	if err != nil {
		return 0, errUpstreamError
	}
	return v, nil
}

func decodeBlockHash(s string) (common.Hash, error) {
	b, err := decodeHexFixed(s, common.HashLength)
	if err != nil {
		return common.Hash{}, err
	}
	return common.BytesToHash(b), nil
}

func decodeKZGCommitment(s string) ([]byte, error) {
	return decodeHexFixed(s, len(kzg4844.Commitment{}))
}

func decodeHexFixed(s string, size int) ([]byte, error) {
	if !strings.HasPrefix(s, "0x") {
		return nil, fmt.Errorf("expected 0x prefixed hex string, got %q", s)
	}
	b, err := hexutil.Decode(s)
	if err != nil {
		return nil, err
	}
	if len(b) != size {
		return nil, fmt.Errorf("expected %d bytes, got %d", size, len(b))
	}
	return b, nil
}

// readBlob reads a blob from EthStorage local storage and classifies failures:
// a commit mismatch (the slot holds different data, or nothing at all) means
// the blob is simply not stored here, while an I/O failure is a server error.
func (a *API) readBlob(kvIndex uint64, blobHash common.Hash) ([]byte, *httpError) {
	blobData, found, err := a.storageMgr.TryRead(kvIndex, int(a.storageMgr.MaxKvSize()), blobHash)
	if err != nil {
		var mismatch *ethstorage.CommitMismatchError
		if errors.As(err, &mismatch) {
			a.lg.Info("Blob commit mismatch in storage", "kvIndex", kvIndex, "blobHash", blobHash, "err", err)
			return nil, errBlobNotInES
		}
		a.lg.Error("Failed to read blob", "err", err)
		return nil, errServerError
	}
	if !found {
		a.lg.Info("Blob not found by storage manager", "kvIndex", kvIndex, "blobHash", blobHash)
		return nil, errBlobNotInES
	}
	return blobData, nil
}

// Deprecated since Fusaka
func (a *API) buildSidecar(kvIndex uint64, kzgCommitment []byte, blobHash common.Hash) (*BlobSidecar, *httpError) {
	blobData, hErr := a.readBlob(kvIndex, blobHash)
	if hErr != nil {
		return nil, hErr
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
