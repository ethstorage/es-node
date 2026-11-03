// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package eth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
)

var testContract = common.HexToAddress("0x000000000000000000000000000000003330001")

// fakeRPC is a minimal JSON-RPC server that records every request it serves so
// that tests can assert on the exact parameters the client sent.
type fakeRPC struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []rpcRequest
}

type rpcRequest struct {
	Method string            `json:"-"`
	Params []json.RawMessage `json:"-"`
}

func (f *fakeRPC) byMethod(method string) []rpcRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []rpcRequest
	for _, req := range f.requests {
		if req.Method == method {
			out = append(out, req)
		}
	}
	return out
}

func newFakeRPC(t *testing.T, handle func(method string, params []json.RawMessage) (any, error)) *fakeRPC {
	t.Helper()
	f := &fakeRPC{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			JSONRPC string            `json:"jsonrpc"`
			ID      json.RawMessage   `json:"id"`
			Method  string            `json:"method"`
			Params  []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode JSON-RPC request: %v", err)
			return
		}
		f.mu.Lock()
		f.requests = append(f.requests, rpcRequest{Method: req.Method, Params: req.Params})
		f.mu.Unlock()

		var res any
		var err error
		if req.Method == "net_version" {
			// PollingClient.NewClient fails hard without a usable network id.
			res = "1"
		} else {
			res, err = handle(req.Method, req.Params)
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if err != nil {
			resp["error"] = map[string]any{"code": -32000, "message": err.Error()}
		} else {
			resp["result"] = res
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("failed to encode JSON-RPC response: %v", err)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

// newTestPollingClient builds a PollingClient against a fake RPC endpoint. The
// head poller is given a stub queryHeader so tests do not have to emulate
// eth_getBlockByNumber. pollRate must be non-zero: the poll loop only re-arms
// (and so only reacts to context cancellation on Close) when a next poll is
// scheduled.
func newTestPollingClient(t *testing.T, rpc *fakeRPC) *PollingClient {
	t.Helper()
	ec, err := ethclient.Dial(rpc.server.URL)
	if err != nil {
		t.Fatalf("failed to dial fake RPC: %v", err)
	}
	client := NewClient(
		context.Background(), ec, true, testContract, 1,
		func() (*types.Header, error) { return nil, errors.New("head polling disabled in test") },
		log.New("polling_client_test"),
	)
	t.Cleanup(client.Close)
	return client
}

// TestFilterLogsByBlockHashQuery asserts the eth_getLogs request built by
// FilterLogsByBlockHash: it filters on the EthStorage contract and the event
// topic, queries the block by hash, and (per EIP-234) never combines blockHash
// with fromBlock/toBlock.
func TestFilterLogsByBlockHashQuery(t *testing.T) {
	blockHash := common.HexToHash("0x2c8c22f8ba515dcf791d6100936e7e36168575a0e604acc0ddab4c6c0f9f0f4c")
	topic := crypto.Keccak256Hash([]byte(PutBlobEvent))

	rpc := newFakeRPC(t, func(method string, params []json.RawMessage) (any, error) {
		return []any{}, nil
	})
	client := newTestPollingClient(t, rpc)

	logs, err := client.FilterLogsByBlockHash(context.Background(), blockHash, PutBlobEvent)
	if err != nil {
		t.Fatalf("FilterLogsByBlockHash failed: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected no logs, got %d", len(logs))
	}

	calls := rpc.byMethod("eth_getLogs")
	if len(calls) != 1 {
		t.Fatalf("expected exactly one eth_getLogs call, got %d", len(calls))
	}
	if len(calls[0].Params) != 1 {
		t.Fatalf("expected a single filter argument, got %d", len(calls[0].Params))
	}

	var query map[string]any
	if err := json.Unmarshal(calls[0].Params[0], &query); err != nil {
		t.Fatalf("failed to decode filter argument: %v", err)
	}

	if got, want := query["blockHash"], blockHash.Hex(); got != want {
		t.Errorf("blockHash = %v, want %v", got, want)
	}
	// EIP-234: blockHash and fromBlock/toBlock are mutually exclusive. A server
	// that received both would reject the request outright.
	for _, forbidden := range []string{"fromBlock", "toBlock"} {
		if _, ok := query[forbidden]; ok {
			t.Errorf("filter must not set %s together with blockHash: %v", forbidden, query)
		}
	}

	addr, ok := query["address"].([]any)
	if !ok || len(addr) != 1 || addr[0] != testContract.Hex() {
		t.Errorf("address = %v, want [%s]", query["address"], testContract.Hex())
	}

	topics, ok := query["topics"].([]any)
	if !ok || len(topics) != 1 {
		t.Fatalf("topics = %v, want a single topic list", query["topics"])
	}
	first, ok := topics[0].([]any)
	if !ok || len(first) != 1 || first[0] != topic.Hex() {
		t.Errorf("topics[0] = %v, want [%s]", topics[0], topic.Hex())
	}
}

// TestFilterLogsByBlockHashPropagatesUnknownBlock makes sure the "unknown
// block" answer an execution client gives for an unknown block hash reaches
// the caller verbatim, so the archiver can tell it apart from a transport
// failure.
func TestFilterLogsByBlockHashPropagatesUnknownBlock(t *testing.T) {
	rpc := newFakeRPC(t, func(method string, params []json.RawMessage) (any, error) {
		return nil, errors.New("unknown block")
	})
	client := newTestPollingClient(t, rpc)

	_, err := client.FilterLogsByBlockHash(context.Background(), common.Hash{1}, PutBlobEvent)
	if err == nil {
		t.Fatal("expected an error for an unknown block hash")
	}
	if got := err.Error(); got != "unknown block" {
		t.Errorf("error = %q, want %q", got, "unknown block")
	}
}

// TestFilterLogsByBlockRangeStillUsesBlockRange guards the existing range
// query against the blockHash change: it must keep sending fromBlock/toBlock
// and no blockHash.
func TestFilterLogsByBlockRangeStillUsesBlockRange(t *testing.T) {
	rpc := newFakeRPC(t, func(method string, params []json.RawMessage) (any, error) {
		return []any{}, nil
	})
	client := newTestPollingClient(t, rpc)

	if _, err := client.FilterLogsByBlockRange(big.NewInt(1), big.NewInt(2), PutBlobEvent); err != nil {
		t.Fatalf("FilterLogsByBlockRange failed: %v", err)
	}

	calls := rpc.byMethod("eth_getLogs")
	if len(calls) != 1 {
		t.Fatalf("expected exactly one eth_getLogs call, got %d", len(calls))
	}
	var query map[string]any
	if err := json.Unmarshal(calls[0].Params[0], &query); err != nil {
		t.Fatalf("failed to decode filter argument: %v", err)
	}
	if _, ok := query["blockHash"]; ok {
		t.Errorf("range query must not set blockHash: %v", query)
	}
	if query["fromBlock"] != "0x1" || query["toBlock"] != "0x2" {
		t.Errorf("fromBlock/toBlock = %v/%v, want 0x1/0x2", query["fromBlock"], query["toBlock"])
	}
	if fmt.Sprint(query["address"]) != fmt.Sprint([]any{testContract.Hex()}) {
		t.Errorf("address = %v, want %s", query["address"], testContract.Hex())
	}
}
