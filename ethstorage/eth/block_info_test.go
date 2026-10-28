package eth

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"
)

func TestInfoToL1BlockRefCanonicalHeader(t *testing.T) {
	for _, tc := range []struct {
		file   string
		fields int
	}{
		{"testdata/sepolia-11700000.json", 21},
		{"testdata/sepolia-11866124.json", 23},
	} {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(tc.file)
			require.NoError(t, err)
			var response struct{ Result json.RawMessage }
			require.NoError(t, json.Unmarshal(data, &response))
			var canonical struct {
				Hash       common.Hash
				ParentHash common.Hash
				Number     hexutil.Uint64
				Timestamp  hexutil.Uint64
			}
			require.NoError(t, json.Unmarshal(response.Result, &canonical))
			var header types.Header
			require.NoError(t, json.Unmarshal(response.Result, &header))

			require.Equal(t, canonical.Hash, header.Hash())
			require.Equal(t, L1BlockRef{
				Hash:       canonical.Hash,
				Number:     uint64(canonical.Number),
				ParentHash: canonical.ParentHash,
				Time:       uint64(canonical.Timestamp),
			}, InfoToL1BlockRef(&header))

			encoded, err := (headerBlockInfo{&header}).HeaderRLP()
			require.NoError(t, err)
			require.Equal(t, canonical.Hash, crypto.Keccak256Hash(encoded))
			var fields []rlp.RawValue
			require.NoError(t, rlp.DecodeBytes(encoded, &fields))
			require.Len(t, fields, tc.fields)
			copied, err := (blockInfo{types.NewBlockWithHeader(&header)}).HeaderRLP()
			require.NoError(t, err)
			require.Equal(t, encoded, copied)
		})
	}
}
