package miner

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadRandaoBlock(t *testing.T, file string) (json.RawMessage, common.Hash, *big.Int) {
	t.Helper()
	data, err := os.ReadFile("../eth/testdata/" + file)
	require.NoError(t, err)
	var response struct{ Result json.RawMessage }
	require.NoError(t, json.Unmarshal(data, &response))
	var canonical struct {
		Hash   common.Hash
		Number hexutil.Big
	}
	require.NoError(t, json.Unmarshal(response.Result, &canonical))
	return response.Result, canonical.Hash, (*big.Int)(&canonical.Number)
}

func randaoHeaderClient(t *testing.T, block json.RawMessage, calls *atomic.Uint32) *ethclient.Client {
	t.Helper()
	var canonical struct{ Number hexutil.Uint64 }
	require.NoError(t, json.Unmarshal(block, &canonical))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage
			Method string
			Params json.RawMessage
		}
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var result interface{}
		switch request.Method {
		case "net_version":
			result = "11155111"
		case "eth_getBlockByNumber":
			calls.Add(1)
			assert.JSONEq(t, `["`+hexutil.EncodeUint64(uint64(canonical.Number))+`",false]`, string(request.Params))
			result = block
		default:
			t.Errorf("unexpected RPC method: %s", request.Method)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0", "id": request.ID, "result": result,
		}))
	}))
	t.Cleanup(server.Close)
	client, err := ethclient.DialContext(t.Context(), server.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client
}

func TestGetRandaoProofCanonicalHeader(t *testing.T) {
	files := []string{"sepolia-11700000.json", "sepolia-11866124.json"}
	for i, file := range files {
		for _, source := range []string{"default", "independent"} {
			t.Run(file+"/"+source, func(t *testing.T) {
				block, canonicalHash, number := loadRandaoBlock(t, file)
				primaryBlock := block
				if source == "independent" {
					primaryBlock, _, _ = loadRandaoBlock(t, files[1-i])
				}
				var primaryCalls, randaoCalls atomic.Uint32
				primary := randaoHeaderClient(t, primaryBlock, &primaryCalls)
				logger := log.DefaultLogger()
				polling := eth.NewClient(t.Context(), primary, false, common.Address{}, 0, nil, logger)
				var randao *eth.RandaoClient
				if source == "independent" {
					client := randaoHeaderClient(t, block, &randaoCalls)
					randao = eth.NewRandaoClient(t.Context(), polling, client)
				}
				mining := NewL1MiningAPI(polling, randao, logger)
				proof, err := mining.getRandaoProof(t.Context(), number)
				require.NoError(t, err)
				require.Equal(t, canonicalHash, crypto.Keccak256Hash(proof))
				var header types.Header
				require.NoError(t, rlp.DecodeBytes(proof, &header))
				require.Equal(t, number, header.Number)

				calldata, err := mining.composeCalldata(t.Context(), result{
					blockNumber: number,
					miner:       common.HexToAddress("0x1234"),
				})
				require.NoError(t, err)
				var arguments abi.Arguments
				for _, name := range []string{"uint256", "uint256", "address", "uint256", "bytes32[]", "uint256[]", "bytes", "bytes[]", "bytes[]"} {
					typ, err := abi.NewType(name, "", nil)
					require.NoError(t, err)
					arguments = append(arguments, abi.Argument{Type: typ})
				}
				values, err := arguments.Unpack(calldata[4:])
				require.NoError(t, err)
				require.Equal(t, number, values[0])
				require.Equal(t, proof, values[6])
				if source == "independent" {
					require.Zero(t, primaryCalls.Load())
					require.Equal(t, uint32(2), randaoCalls.Load())
				} else {
					require.Equal(t, uint32(2), primaryCalls.Load())
				}
			})
		}
	}
}
