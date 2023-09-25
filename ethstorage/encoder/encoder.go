// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package encoder

import (
	"errors"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/fft"
	"github.com/ethereum/go-ethereum/common"
	"github.com/iden3/go-iden3-crypto/constants"
	"github.com/iden3/go-iden3-crypto/ff"
	"github.com/iden3/go-iden3-crypto/poseidon"
)

func Encode(hash common.Hash, size int) ([]byte, error) {
	if size%64 != 0 {
		return nil, errors.New("size must be a multiple of 64")
	}

	initialState := big.NewInt(0)

	k := big.NewInt(0).SetBytes(hash.Bytes())

	// TODO: simple hash to point mapping
	for k.Cmp(constants.Q) != -1 {
		k = k.Sub(k, constants.Q)
	}

	elements := make([]*ff.Element, 0)
	for i := 0; i < size/64; i++ {
		k1 := new(big.Int).Set(k)
		k1 = k1.Add(k1, big.NewInt(int64(i)))
		k2 := new(big.Int).Set(k)
		k2 = k2.Add(k2, big.NewInt(int64(i+1)))
		hs, _ := poseidon.HashState(initialState, []*big.Int{k1, k2})
		elements = append(elements, hs[0], hs[1])
	}

	pol := make([]fr.Element, len(elements))
	for i := 0; i < len(elements); i++ {
		bs := elements[i].Bytes()
		pol[i].SetBytes(bs[:])
	}

	domainWithPrecompute := fft.NewDomain(uint64(len(elements)))
	domainWithPrecompute.FFT(pol, fft.DIF)

	fft.BitReverse(pol)

	returnData := make([]byte, 0)
	for _, e := range pol {
		bs := e.Bytes()
		returnData = append(returnData, bs[:]...)
	}

	return returnData, nil
}
