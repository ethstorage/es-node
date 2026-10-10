// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// A valid 32 byte execution block hash, plus 48 byte KZG commitments built from
// a seed byte so their length is guaranteed.
const testBlockHash = "0x1111111111111111111111111111111111111111111111111111111111111111"

func commitmentHex(seed byte) string {
	b := make([]byte, 48)
	for i := range b {
		b[i] = seed + byte(i)
	}
	return "0x" + common.Bytes2Hex(b)
}

var (
	testCommit0 = commitmentHex(0x89)
	testCommit1 = commitmentHex(0xa1)
	testCommit2 = commitmentHex(0x90)
)

// beaconBlockJSON builds a /eth/v2/beacon/blocks response body the way a beacon
// node would serve it for the given consensus version.
func beaconBlockJSON(version, slot, elBlockHash string, commitments []string, useBid bool) string {
	commitsJSON := "null"
	if commitments != nil {
		quoted := make([]string, len(commitments))
		for i, c := range commitments {
			quoted[i] = fmt.Sprintf("%q", c)
		}
		commitsJSON = "[" + strings.Join(quoted, ",") + "]"
	}

	var body string
	if useBid {
		body = fmt.Sprintf(`{"signed_execution_payload_bid":{"message":{`+
			`"block_hash":%q,"blob_kzg_commitments":%s}}}`, elBlockHash, commitsJSON)
	} else {
		body = fmt.Sprintf(`{"execution_payload":{"block_hash":%q},"blob_kzg_commitments":%s}`,
			elBlockHash, commitsJSON)
	}
	return fmt.Sprintf(`{"version":%q,"data":{"message":{"slot":%q,"body":%s}}}`, version, slot, body)
}

func TestParseBeaconBlockVersions(t *testing.T) {
	cases := []struct {
		name        string
		version     string
		useBid      bool
		commitments []string
	}{
		{"deneb", beaconBlockVersionDeneb, false, []string{testCommit0}},
		{"electra", beaconBlockVersionElectra, false, []string{testCommit0, testCommit1, testCommit2}},
		{"fulu", beaconBlockVersionFulu, false, []string{testCommit1}},
		// Gloas blocks have no execution_payload at all: everything lives in
		// the signed bid.
		{"gloas", beaconBlockVersionGloas, true, []string{testCommit0, testCommit1}},
		{"gloas without blobs", beaconBlockVersionGloas, true, []string{}},
		{"deneb without blobs", beaconBlockVersionDeneb, false, nil},
		{"version case insensitive", "Gloas", true, []string{testCommit0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := beaconBlockJSON(tc.version, "1234", testBlockHash, tc.commitments, tc.useBid)
			block, hErr := parseBeaconBlock([]byte(body))
			if hErr != nil {
				t.Fatalf("parseBeaconBlock failed: %d %s", hErr.Code, hErr.Message)
			}
			if block.executionHash != common.HexToHash(testBlockHash) {
				t.Errorf("executionHash = %s, want %s", block.executionHash, testBlockHash)
			}
			if block.slot != 1234 {
				t.Errorf("slot = %d, want 1234", block.slot)
			}
			want := tc.commitments
			if want == nil {
				want = []string{}
			}
			if len(block.kzgCommitsAll) != len(want) {
				t.Fatalf("got %d commitments, want %d", len(block.kzgCommitsAll), len(want))
			}
			// Commitments must keep the order they appear in the block: the
			// archiver relies on it to map blobs to their index.
			for i, c := range want {
				if block.kzgCommitsAll[i] != c {
					t.Errorf("commitment %d = %s, want %s", i, block.kzgCommitsAll[i], c)
				}
			}
		})
	}
}

func TestParseBeaconBlockErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want *httpError
	}{
		{
			name: "unknown version",
			body: beaconBlockJSON("prague", "1", testBlockHash, nil, false),
			want: errUnsupportedBeaconVersion,
		},
		{
			name: "missing version",
			body: `{"data":{"message":{"slot":"1","body":{"execution_payload":{"block_hash":"` + testBlockHash + `"}}}}}`,
			want: errUpstreamError,
		},
		{
			name: "deneb without execution payload",
			body: beaconBlockJSON("deneb", "1", testBlockHash, []string{testCommit0}, true),
			want: errUpstreamError,
		},
		{
			name: "gloas without signed bid",
			body: beaconBlockJSON("gloas", "1", testBlockHash, []string{testCommit0}, false),
			want: errUpstreamError,
		},
		{
			name: "gloas bid without block hash",
			body: `{"version":"gloas","data":{"message":{"slot":"1","body":{"signed_execution_payload_bid":{"message":` +
				`{"blob_kzg_commitments":[` + fmt.Sprintf("%q", testCommit0) + `]}}}}}}`,
			want: errUpstreamError,
		},
		{
			name: "execution payload without block hash",
			body: `{"version":"deneb","data":{"message":{"slot":"1","body":{"blob_kzg_commitments":null}}}}`,
			want: errUpstreamError,
		},
		{
			name: "missing slot",
			body: `{"version":"deneb","data":{"message":{"body":{"execution_payload":{"block_hash":"` + testBlockHash + `"}}}}}`,
			want: errUpstreamError,
		},
		{
			name: "block hash too short",
			body: beaconBlockJSON("deneb", "1", "0x111111111111111111111111111111111111111111111111111111111111", nil, false),
			want: errUpstreamError,
		},
		{
			name: "commitment too short",
			body: beaconBlockJSON("deneb", "1", testBlockHash, []string{"0x11111111111111111111"}, false),
			want: errUpstreamError,
		},
		{
			name: "commitment not hex",
			body: beaconBlockJSON("deneb", "1", testBlockHash, []string{"notahex"}, false),
			want: errUpstreamError,
		},
		{
			name: "invalid json",
			body: `{"version":"deneb",`,
			want: errUpstreamError,
		},
		{
			name: "empty body",
			body: ``,
			want: errUpstreamError,
		},
		{
			name: "null data",
			body: `{"version":"deneb","data":null}`,
			want: errUpstreamError,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block, hErr := parseBeaconBlock([]byte(tc.body))
			if hErr == nil {
				t.Fatalf("expected an error, got block %+v", block)
			}
			if block != nil {
				t.Errorf("expected no block alongside the error")
			}
			if hErr.Code != tc.want.Code || hErr.Message != tc.want.Message {
				t.Errorf("error = %d %q, want %d %q", hErr.Code, hErr.Message, tc.want.Code, tc.want.Message)
			}
			// A malformed or unsupported block must never look like a block
			// that does not exist: callers would stop retrying.
			if hErr.Code == http.StatusNotFound {
				t.Errorf("classification error: %d %q must not be 404", hErr.Code, hErr.Message)
			}
		})
	}
}

func TestFetchBeaconBlockStatusCodes(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   *httpError
	}{
		{
			name:   "beacon 404 is block not found",
			status: http.StatusNotFound,
			body:   `{"code":404,"message":"Block not found"}`,
			want:   errUnknownBlock,
		},
		{
			name:   "beacon 500 is upstream error",
			status: http.StatusInternalServerError,
			body:   `{"code":500,"message":"Internal Server Error"}`,
			want:   errUpstreamError,
		},
		{
			name:   "beacon 503 is upstream error",
			status: http.StatusServiceUnavailable,
			body:   `{"code":503,"message":"beacon node is syncing"}`,
			want:   errUpstreamError,
		},
		{
			name:   "beacon 200 with garbage is upstream error",
			status: http.StatusOK,
			body:   `<html>proxy error</html>`,
			want:   errUpstreamError,
		},
		{
			name:   "beacon 200 with unsupported version",
			status: http.StatusOK,
			body:   beaconBlockJSON("prague", "1", testBlockHash, nil, false),
			want:   errUnsupportedBeaconVersion,
		},
		{
			name:   "beacon 200 gloas block",
			status: http.StatusOK,
			body:   beaconBlockJSON("gloas", "9", testBlockHash, []string{testCommit0}, true),
			want:   nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			block, hErr := fetchBeaconBlock(t.Context(), srv.Client(), srv.URL)
			if tc.want == nil {
				if hErr != nil {
					t.Fatalf("unexpected error: %d %s", hErr.Code, hErr.Message)
				}
				if block.executionHash != common.HexToHash(testBlockHash) {
					t.Errorf("executionHash = %s, want %s", block.executionHash, testBlockHash)
				}
				return
			}
			if hErr == nil {
				t.Fatalf("expected %d %q, got block %+v", tc.want.Code, tc.want.Message, block)
			}
			if hErr.Code != tc.want.Code || hErr.Message != tc.want.Message {
				t.Errorf("error = %d %q, want %d %q", hErr.Code, hErr.Message, tc.want.Code, tc.want.Message)
			}
		})
	}
}

func TestIsUnknownBlockErr(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{err: fmt.Errorf("unknown block"), want: true},
		{err: fmt.Errorf(`{"code":-32000,"message":"unknown block"}`), want: true},
		{err: fmt.Errorf("Block not found"), want: true},
		{err: fmt.Errorf("connection refused"), want: false},
		{err: fmt.Errorf("context deadline exceeded"), want: false},
		{err: nil, want: false},
	}
	for _, tc := range cases {
		if got := isUnknownBlockErr(tc.err); got != tc.want {
			t.Errorf("isUnknownBlockErr(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

// TestBeaconBlockResponseShape makes sure the decode struct keeps up with the
// JSON the beacon nodes actually serve for each version.
func TestBeaconBlockResponseShape(t *testing.T) {
	var resp beaconBlockResponse
	if err := json.Unmarshal([]byte(beaconBlockJSON("gloas", "7", testBlockHash, []string{testCommit0}, true)), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Message.Body.ExecutionPayload != nil {
		t.Error("gloas body must not decode an execution payload")
	}
	if resp.Data.Message.Body.SignedExecutionPayloadBid == nil {
		t.Fatal("gloas bid not decoded")
	}
	if *resp.Data.Message.Body.SignedExecutionPayloadBid.Message.BlockHash != testBlockHash {
		t.Error("bid block hash not decoded")
	}
}
