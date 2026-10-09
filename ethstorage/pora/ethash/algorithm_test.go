package pora

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestHashimotoKeccak512Compatibility(t *testing.T) {
	// Canonical vector from go-ethereum v1.10.26 consensus/ethash/algorithm_test.go.
	cache := make([]uint32, 1024/4)
	generateCache(cache, 0, make([]byte, 32))
	dataset := make([]uint32, 32*1024/4)
	generateDataset(dataset, 0, cache)
	hash := hexutil.MustDecode("0xc9149cc0386e689d789a1c2f3d5d169a61a6218ed30e74414dc736e442ef3d1f")
	wantDigest := hexutil.MustDecode("0xe4073cffaef931d37117cefd9afd27ea0f1cad6a981dd2605c4a1ac97c519800")
	wantResult := hexutil.MustDecode("0xd3539235ee2e6f8db665c0a72169f55b7f6c605712330b778ec3944f0eb5a557")
	digest, result := hashimotoLight(32*1024, cache, hash, 0)
	require.Equal(t, wantDigest, digest)
	require.Equal(t, wantResult, result)
	digest, result = hashimotoFull(dataset, hash, 0)
	require.Equal(t, wantDigest, digest)
	require.Equal(t, wantResult, result)
}

func TestHashimotoForMaskKeccak512Compatibility(t *testing.T) {
	// Golden mask generated with the original implementation and go-ethereum v1.16.5.
	want := hexutil.MustDecode("0xd62b97181c47e017f78c0ef5695aec3d1d56d9895ba9c2ee139e01e4b740823f2" +
		"81d0b2f4a0c9e3522c86427b9df766b3df67c2602e0d143eff9d7e6741c9a85d" +
		"699e8af1c6ce8c0f7402ce7e97fbd881d320e5edb5294a113a2715337c7e0d9" +
		"28efe46a4ab14afc22a862ec39ee853b3d220a6c02f2e418effb2d7e74d8576a")
	mask := HashimotoForMask([]byte("EthStorage Keccak512 compatibility"), 32*1024, func(index uint32) []uint32 {
		data := make([]uint32, hashWords)
		for i := range data {
			data[i] = index*hashWords + uint32(i)
		}
		return data
	})
	require.Equal(t, want, mask)
}
