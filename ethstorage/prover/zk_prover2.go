// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/crate-crypto/go-proto-danksharding-crypto/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Generate ZK Proof for the given encoding keys and sample indexes
func (p *ZKProver) GenerateZKProof(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error) {
	for i, idx := range sampleIdxs {
		p.lg.Debug("Generate zk proof", "encodingKey", encodingKeys[i], "sampleIdx", sampleIdxs[i])
		if int(idx) >= eth.FieldElementsPerBlob {
			return nil, nil, fmt.Errorf("chunk index out of scope: %d", idx)
		}
	}
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		p.lg.Info("Generate zk proof done", "sampleIdx", sampleIdxs, "timeUsed(s)", dur.Seconds())
	}(start)

	inputBytes, err := p.GenerateInputs(encodingKeys, sampleIdxs)
	if err != nil {
		p.lg.Error("Generate inputs failed", "error", err)
		return nil, nil, err
	}
	proof, publicInputs, err := p.prove(inputBytes)
	if err != nil {
		p.lg.Error("Prove failed", "error", err)
		return nil, nil, err
	}
	masks, err := readMasks(publicInputs)
	if err != nil {
		p.lg.Error("Read masks failed", "error", err)
		return nil, nil, err
	}
	p.lg.Debug("Generate zk proof", "masks", masks)
	return proof, masks, nil
}

func (p *ZKProver) GenerateInputs(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, error) {
	var encodingKeyModStr, xInStr []string
	for i, sampleIdx := range sampleIdxs {
		var b fr.Element
		var exp big.Int
		exp.Div(exp.Sub(fr.Modulus(), common.Big1), big.NewInt(int64(eth.FieldElementsPerBlob)))
		ru := b.Exp(*b.SetInt64(5), &exp)
		xIn := ru.Exp(*ru, new(big.Int).SetUint64(sampleIdx))
		xInStr = append(xInStr, xIn.String())
		encodingKeyMod := fr.Modulus().Mod(encodingKeys[i].Big(), fr.Modulus())
		encodingKeyModStr = append(encodingKeyModStr, hexutil.Encode(encodingKeyMod.Bytes()))
	}
	inputObj := InputPairV2{
		EncodingKeyIn: encodingKeyModStr,
		XIn:           xInStr,
	}
	p.lg.Debug("Generate zk proof", "input", inputObj)
	return json.Marshal(inputObj)
}

func readMasks(publicInputs string) ([]*big.Int, error) {
	var output []string
	if err := json.Unmarshal([]byte(publicInputs), &output); err != nil {
		return nil, err
	}
	var masks []*big.Int
	for _, v := range output {
		mask, ok := new(big.Int).SetString(v, 0)
		if !ok {
			return masks, fmt.Errorf("invalid mask %v", v)
		}
		masks = append(masks, mask)
	}
	return masks, nil
}

type InputPairV2 struct {
	EncodingKeyIn []string `json:"encodingKeyIn"`
	XIn           []string `json:"xIn"`
}
