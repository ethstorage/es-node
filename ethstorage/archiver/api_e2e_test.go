// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/blobs"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
)

// End-to-end tests of the archiver API: a beacon node and an execution client
// are replaced by local fakes, and blobs are written to a real StorageManager
// backed by a temporary shard file. This exercises the whole path: beacon block
// parsing, PutBlob lookup by execution block hash, blob filtering, ordering and
// local reads.

const (
	e2eKvEntries = uint64(16)
	e2eKvSize    = uint64(blobs.BlobLength)
	e2eChunkSize = uint64(blobs.BlobLength)
	e2eSlot      = "9999"
)

var e2eContract = common.HexToAddress("0x000000000000000000000000000000003330001")

// e2eHarness wires an archiver API to fake beacon and L1 endpoints.
type e2eHarness struct {
	api        *API
	elLogs     []types.Log
	elErr      error
	getLogsReq []map[string]any
}

// makeBlob builds a recognisable blob whose KZG commitment and versioned hash
// are computed the same way the archiver computes them.
func makeBlob(t *testing.T, kvIndex uint64) ([]byte, kzg4844.Commitment, common.Hash) {
	t.Helper()
	raw := make([]byte, blobs.BlobLength)
	copy(raw[:20], e2eContract.Bytes())
	binary.BigEndian.PutUint64(raw[20:28], kvIndex)
	// Fill the rest with a per-KV pattern so tests can tell blobs apart. Every
	// 32 byte field element stays well below the BLS modulus.
	for i := 28; i < len(raw); i++ {
		raw[i] = byte(kvIndex)
	}
	blob := kzg4844.Blob(raw)
	commit, err := kzg4844.BlobToCommitment(&blob)
	if err != nil {
		t.Fatalf("BlobToCommitment failed: %v", err)
	}
	vh := common.Hash(kzg4844.CalcBlobHashV1(sha256.New(), &commit))
	return raw, commit, vh
}

// newStorageManager fills kv slots with blobs and returns the manager.
func newStorageManager(t *testing.T, kvIndexes []uint64, blobsData [][]byte, commits []common.Hash) *ethstorage.StorageManager {
	t.Helper()
	sm := ethstorage.NewShardManager(e2eContract, e2eKvSize, e2eKvEntries, e2eChunkSize)
	if err := sm.AddDataShard(0); err != nil {
		t.Fatalf("AddDataShard failed: %v", err)
	}
	fileName := filepath.Join(t.TempDir(), "archiver_e2e.dat")
	df, err := ethstorage.Create(fileName, 0, e2eKvEntries*(e2eKvSize/e2eChunkSize), 0, e2eKvSize, ethstorage.NO_ENCODE, common.Address{}, e2eChunkSize)
	if err != nil {
		t.Fatalf("Create data file failed: %v", err)
	}
	t.Cleanup(func() {
		if err := df.Close(); err != nil {
			t.Errorf("close data file failed: %v", err)
		}
	})
	if err := sm.AddDataFile(df); err != nil {
		t.Fatalf("AddDataFile failed: %v", err)
	}

	storageMgr := ethstorage.NewStorageManager(sm, &e2eL1Source{kvEntryCount: e2eKvEntries}, log.New("archiver_e2e"))
	storageMgr.DownloadThreadNum = 1
	if len(kvIndexes) > 0 {
		if err := storageMgr.DownloadFinished(100, kvIndexes, blobsData, commits); err != nil {
			t.Fatalf("DownloadFinished failed: %v", err)
		}
	}
	return storageMgr
}

type e2eL1Source struct {
	kvEntryCount uint64
}

func (l *e2eL1Source) GetKvMetas(kvIndices []uint64, blockNumber int64) ([][32]byte, error) {
	return make([][32]byte, len(kvIndices)), nil
}

func (l *e2eL1Source) GetStorageKvEntryCount(blockNumber int64) (uint64, error) {
	return l.kvEntryCount, nil
}

// newE2EAPI starts a fake beacon node serving one block of the given version,
// and a fake execution client answering eth_getLogs, and wires them into an API.
func newE2EAPI(t *testing.T, storageMgr *ethstorage.StorageManager, version string, commitments []string, elLogs []types.Log, elErr error) *e2eHarness {
	t.Helper()
	h := &e2eHarness{
		elLogs: elLogs,
		elErr:  elErr,
	}

	beacon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/eth/v1/beacon/genesis"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"genesis_time": "1000"}})
		case strings.Contains(r.URL.Path, "/eth/v2/beacon/blocks/"):
			_, _ = w.Write([]byte(beaconBlockJSON(version, e2eSlot, testBlockHash, commitments, version == beaconBlockVersionGloas)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(beacon.Close)

	l1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("bad JSON-RPC request: %v", err)
			return
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		switch req.Method {
		case "net_version":
			resp["result"] = "1"
		case "eth_getLogs":
			var query map[string]any
			if len(req.Params) > 0 {
				_ = json.Unmarshal(req.Params[0], &query)
			}
			h.getLogsReq = append(h.getLogsReq, query)
			if elErr != nil {
				resp["error"] = map[string]any{"code": -32000, "message": elErr.Error()}
			} else {
				resp["result"] = elLogs
			}
		default:
			resp["error"] = map[string]any{"code": -32601, "message": "unsupported in test: " + req.Method}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(l1.Close)

	beaconClient, err := eth.NewBeaconClient(beacon.URL, 12)
	if err != nil {
		t.Fatalf("NewBeaconClient failed: %v", err)
	}
	ec, err := ethclient.Dial(l1.URL)
	if err != nil {
		t.Fatalf("dial fake L1 failed: %v", err)
	}
	l1Client := eth.NewClient(context.Background(), ec, true, e2eContract, 1,
		func() (*types.Header, error) { return nil, errors.New("head polling disabled in test") },
		log.New("archiver_e2e_l1"))
	t.Cleanup(l1Client.Close)

	h.api = NewAPI(storageMgr, beaconClient, l1Client, log.New("archiver_e2e_api"))
	return h
}

// putBlobLog builds the PutBlob log an execution client would return for a blob
// stored at kvIndex with the given versioned hash.
func putBlobLog(kvIndex uint64, blobHash common.Hash, logIndex uint) types.Log {
	// PutBlob emits kvIndex in topic 1 and leaves topic 2 empty; the versioned
	// blob hash is topic 3.
	var emptyTopic common.Hash
	topics := []common.Hash{
		crypto.Keccak256Hash([]byte(eth.PutBlobEvent)),
		common.BigToHash(big.NewInt(int64(kvIndex))),
		emptyTopic,
		blobHash,
	}
	return types.Log{
		Address:     e2eContract,
		Topics:      topics,
		Data:        []byte{},
		BlockNumber: 100,
		BlockHash:   common.HexToHash(testBlockHash),
		TxHash:      common.Hash{byte(logIndex) + 1},
		Index:       logIndex,
	}
}

// TestQueryBlobsOrderAndFilter stores three blobs at shuffled kv indices and
// checks that queryBlobs returns them in beacon commitment order, filtered to
// the requested versioned hashes.
func TestQueryBlobsOrderAndFilter(t *testing.T) {
	// commitment order -> kv index mapping is deliberately shuffled
	kvIndexes := []uint64{5, 1, 3}
	raw := make([][]byte, 3)
	commits := make([]kzg4844.Commitment, 3)
	hashes := make([]common.Hash, 3)
	for i, kv := range kvIndexes {
		raw[i], commits[i], hashes[i] = makeBlob(t, kv)
	}

	commitHex := make([]string, 3)
	for i := range commits {
		commitHex[i] = "0x" + common.Bytes2Hex(commits[i][:])
	}

	// The execution client reports the logs in an order unrelated to the
	// commitment order.
	elLogs := []types.Log{
		putBlobLog(kvIndexes[1], hashes[1], 0),
		putBlobLog(kvIndexes[2], hashes[2], 1),
		putBlobLog(kvIndexes[0], hashes[0], 2),
	}

	storageMgr := newStorageManager(t, kvIndexes, raw, hashes)
	h := newE2EAPI(t, storageMgr, beaconBlockVersionGloas, commitHex, elLogs, nil)

	// ask for blob 2 and blob 0 only, out of order
	res, hErr := h.api.queryBlobs(context.Background(), e2eSlot, []common.Hash{hashes[2], hashes[0]})
	if hErr != nil {
		t.Fatalf("queryBlobs failed: %d %s", hErr.Code, hErr.Message)
	}
	if len(res.Data) != 2 {
		t.Fatalf("got %d blobs, want 2", len(res.Data))
	}
	// commitment order wins over both the request order and the EL log order
	wantOrder := []int{0, 2}
	for n, want := range wantOrder {
		got, err := hexToBlob(res.Data[n])
		if err != nil {
			t.Fatalf("blob %d not decodable: %v", n, err)
		}
		if !equalBytes(got, raw[want]) {
			t.Errorf("blob %d is kv %d, want kv %d", n, blobKvIndex(got), kvIndexes[want])
		}
	}

	// the archiver must query the execution block by hash, not by number
	assertGetLogsByHash(t, h.getLogsReq)
}

// TestQueryBlobSidecarsGloas checks the deprecated sidecar endpoint on a Gloas
// block: it must resolve the execution block from the bid and return sidecars
// with the commitment and proof matching the block.
func TestQueryBlobSidecarsGloas(t *testing.T) {
	kvIndexes := []uint64{2, 7}
	raw := make([][]byte, 2)
	commits := make([]kzg4844.Commitment, 2)
	hashes := make([]common.Hash, 2)
	for i, kv := range kvIndexes {
		raw[i], commits[i], hashes[i] = makeBlob(t, kv)
	}
	commitHex := []string{"0x" + common.Bytes2Hex(commits[0][:]), "0x" + common.Bytes2Hex(commits[1][:])}
	elLogs := []types.Log{putBlobLog(kvIndexes[0], hashes[0], 0), putBlobLog(kvIndexes[1], hashes[1], 1)}

	storageMgr := newStorageManager(t, kvIndexes, raw, hashes)
	h := newE2EAPI(t, storageMgr, beaconBlockVersionGloas, commitHex, elLogs, nil)

	res, hErr := h.api.queryBlobSidecars(context.Background(), e2eSlot, []uint64{1})
	if hErr != nil {
		t.Fatalf("queryBlobSidecars failed: %d %s", hErr.Code, hErr.Message)
	}
	if len(res.Data) != 1 {
		t.Fatalf("got %d sidecars, want 1", len(res.Data))
	}
	sidecar := res.Data[0]
	if sidecar.Index != 1 {
		t.Errorf("sidecar index = %d, want 1", sidecar.Index)
	}
	if !equalBytes(sidecar.KZGCommitment[:], commits[1][:]) {
		t.Error("sidecar commitment does not match the block commitment")
	}
	if !equalBytes(sidecar.Blob[:], raw[1]) {
		t.Error("sidecar blob does not match stored data")
	}
	sidecarBlob := kzg4844.Blob(raw[1])
	proof, err := kzg4844.ComputeBlobProof(&sidecarBlob, commits[1])
	if err != nil {
		t.Fatal(err)
	}
	if !equalBytes(sidecar.KZGProof[:], proof[:]) {
		t.Error("sidecar proof mismatch")
	}
	assertGetLogsByHash(t, h.getLogsReq)
}

// TestQueryBlobsStorageMissing covers a block that exists on both the beacon
// chain and L1, but whose blob is not stored locally.
func TestQueryBlobsStorageMissing(t *testing.T) {
	kv := uint64(4)
	raw, commit, hash := makeBlob(t, kv)
	commitHex := []string{"0x" + common.Bytes2Hex(commit[:])}
	elLogs := []types.Log{putBlobLog(kv, hash, 0)}

	// storage manager exists but nothing was written into it
	storageMgr := newStorageManager(t, nil, nil, nil)
	h := newE2EAPI(t, storageMgr, beaconBlockVersionDeneb, commitHex, elLogs, nil)

	_, hErr := h.api.queryBlobs(context.Background(), e2eSlot, []common.Hash{hash})
	if hErr == nil {
		t.Fatal("expected an error for a blob that is not stored")
	}
	if hErr.Code != http.StatusNotFound || hErr.Message != errBlobNotInES.Message {
		t.Errorf("error = %d %q, want 404 %q", hErr.Code, hErr.Message, errBlobNotInES.Message)
	}
	_ = raw
}

// TestQueryBlobsExecutionBlockNotAvailable covers a Gloas bid whose execution
// block is not known to the L1 client yet: the request must be retryable, not a
// final "block not found".
func TestQueryBlobsExecutionBlockNotAvailable(t *testing.T) {
	kv := uint64(4)
	_, commit, hash := makeBlob(t, kv)
	commitHex := []string{"0x" + common.Bytes2Hex(commit[:])}

	storageMgr := newStorageManager(t, nil, nil, nil)
	h := newE2EAPI(t, storageMgr, beaconBlockVersionGloas, commitHex, nil, errors.New("unknown block"))

	_, hErr := h.api.queryBlobs(context.Background(), e2eSlot, []common.Hash{hash})
	if hErr == nil {
		t.Fatal("expected an error when the execution block is unavailable")
	}
	if hErr.Code != http.StatusServiceUnavailable {
		t.Errorf("error = %d %q, want %d %q", hErr.Code, hErr.Message,
			errExecutionBlockNotAvailable.Code, errExecutionBlockNotAvailable.Message)
	}
	if hErr.Code == http.StatusNotFound {
		t.Error("an unavailable execution block must not be reported as a missing block")
	}
}

// TestQueryBlobsBeaconBlockNotFound covers a slot the beacon node does not know.
func TestQueryBlobsBeaconBlockNotFound(t *testing.T) {
	storageMgr := newStorageManager(t, nil, nil, nil)
	h := newE2EAPI(t, storageMgr, beaconBlockVersionDeneb, nil, nil, nil)
	// make the beacon answer 404 for the block
	h.api.beaconClient = newNotFoundBeaconClient(t)

	_, hErr := h.api.queryBlobs(context.Background(), "123456", nil)
	if hErr == nil {
		t.Fatal("expected an error for an unknown slot")
	}
	if hErr.Code != http.StatusNotFound || hErr.Message != errUnknownBlock.Message {
		t.Errorf("error = %d %q, want 404 %q", hErr.Code, hErr.Message, errUnknownBlock.Message)
	}
}

// TestQueryBlobsUnsupportedVersion makes sure a block the archiver cannot
// decode is never reported as "block not found".
func TestQueryBlobsUnsupportedVersion(t *testing.T) {
	storageMgr := newStorageManager(t, nil, nil, nil)
	h := newE2EAPI(t, storageMgr, "prague", nil, nil, nil)

	_, hErr := h.api.queryBlobs(context.Background(), e2eSlot, nil)
	if hErr == nil {
		t.Fatal("expected an error for an unsupported beacon version")
	}
	if hErr.Code != errUnsupportedBeaconVersion.Code {
		t.Errorf("error = %d %q, want %d %q", hErr.Code, hErr.Message,
			errUnsupportedBeaconVersion.Code, errUnsupportedBeaconVersion.Message)
	}
}

func newNotFoundBeaconClient(t *testing.T) *eth.BeaconClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/genesis") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"genesis_time": "1000"}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"Block not found"}`))
	}))
	t.Cleanup(srv.Close)
	client, err := eth.NewBeaconClient(srv.URL, 12)
	if err != nil {
		t.Fatalf("NewBeaconClient failed: %v", err)
	}
	return client
}

func assertGetLogsByHash(t *testing.T, requests []map[string]any) {
	t.Helper()
	if len(requests) != 1 {
		t.Fatalf("expected exactly one eth_getLogs call, got %d", len(requests))
	}
	query := requests[0]
	if query["blockHash"] != common.HexToHash(testBlockHash).Hex() {
		t.Errorf("eth_getLogs blockHash = %v, want %s", query["blockHash"], testBlockHash)
	}
	for _, forbidden := range []string{"fromBlock", "toBlock"} {
		if _, ok := query[forbidden]; ok {
			t.Errorf("eth_getLogs must not set %s when querying by block hash", forbidden)
		}
	}
	topic := crypto.Keccak256Hash([]byte(eth.PutBlobEvent)).Hex()
	topics, ok := query["topics"].([]any)
	if !ok || len(topics) != 1 {
		t.Fatalf("eth_getLogs topics = %v, want a single topic list", query["topics"])
	}
	first, ok := topics[0].([]any)
	if !ok || len(first) != 1 || first[0] != topic {
		t.Errorf("eth_getLogs topics[0] = %v, want [%s]", topics[0], topic)
	}
}

func hexToBlob(s string) ([]byte, error) {
	if !strings.HasPrefix(s, "0x") {
		return nil, fmt.Errorf("expected 0x prefixed hex, got %d chars", len(s))
	}
	return common.FromHex(s), nil
}

func blobKvIndex(blob []byte) uint64 {
	if len(blob) < 28 {
		return 0
	}
	return binary.BigEndian.Uint64(blob[20:28])
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
