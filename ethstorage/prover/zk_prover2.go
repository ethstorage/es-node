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

// Generate ZK Proof for the given encoding key and sample index
func (p *ZKProver) GenerateZKProof(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error) {
	proof, publics, err := p.GenerateZKProofRaw(encodingKeys, sampleIdxs)
	if err != nil {
		return nil, nil, err
	}
	if len(publics) != 6 {
		return nil, nil, fmt.Errorf("publics length is %d", len(publics))
	}
	return proof, publics[4:], nil
}

// Generate ZK Proof for the given encoding keys and chunck indexes using snarkjs
func (p *ZKProver) GenerateZKProofRaw(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, []*big.Int, error) {
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
	publics, err := readPublics(publicInputs)
	if err != nil {
		p.lg.Error("Read publics failed", "error", err)
		return nil, nil, err
	}
	return proof, publics, nil
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

func readPublics(publicInputs string) ([]*big.Int, error) {
	var output []string
	if err := json.Unmarshal([]byte(publicInputs), &output); err != nil {
		return nil, err
	}
	var publics []*big.Int
	for _, v := range output {
		pub, ok := new(big.Int).SetString(v, 0)
		if !ok {
			return nil, fmt.Errorf("invalid public input %v", v)
		}
		publics = append(publics, pub)
	}
	return publics, nil
}

type InputPairV2 struct {
	EncodingKeyIn []string `json:"encodingKeyIn"`
	XIn           []string `json:"xIn"`
}
