// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/crate-crypto/go-proto-danksharding-crypto/eth"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethstorage/go-ethstorage/ethstorage/encoder"
	"github.com/iden3/go-rapidsnark/types"
)

type ZKProof struct {
	A G1Point `json:"A"`
	B G2Point `json:"B"`
	C G1Point `json:"C"`
}

type InputPair struct {
	EncodingKeyIn string `json:"encodingKeyIn"`
	XIn           string `json:"xIn"`
}

type InputPairV2 struct {
	EncodingKeyIn []string `json:"encodingKeyIn"`
	XIn           []string `json:"xIn"`
}

func toG1Point(s []string) (G1Point, error) {
	var x, y big.Int
	_, ok := x.SetString(s[0], 10)
	if !ok {
		return G1Point{}, fmt.Errorf("invalid number %s", s[0])
	}
	_, ok = y.SetString(s[1], 10)
	if !ok {
		return G1Point{}, fmt.Errorf("invalid number %s", s[1])
	}
	return G1Point{&x, &y}, nil
}

func toG2Point(s [][]string) (G2Point, error) {
	var x, y [2]*big.Int
	for i, vi := range s {
		for j, vj := range vi {
			z := new(big.Int)
			_, ok := z.SetString(vj, 10)
			if !ok {
				return G2Point{}, fmt.Errorf("invalid number %s", vj)
			}
			// swap so that it can be accepted by the verifier contract
			if i == 0 {
				x[1-j] = z
			} else {
				y[1-j] = z
			}
		}
	}
	return G2Point{x, y}, nil
}

type G1Point struct {
	X *big.Int
	Y *big.Int
}

func (p G1Point) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		X string `json:"X"`
		Y string `json:"Y"`
	}{
		X: p.X.String(),
		Y: p.Y.String(),
	})
}

func (p *G1Point) UnmarshalJSON(b []byte) error {
	var values struct {
		X string `json:"X"`
		Y string `json:"Y"`
	}
	err := json.Unmarshal(b, &values)
	if err != nil {
		fmt.Printf("Unmarshal %v\n", err)
		return err
	}
	p.X = new(big.Int)
	_, ok := p.X.SetString(values.X, 10)
	if !ok {
		return err
	}
	p.Y = new(big.Int)
	_, ok = p.Y.SetString(values.Y, 10)
	if !ok {
		return err
	}
	return nil
}

type G2Point struct {
	X [2]*big.Int `json:"X"`
	Y [2]*big.Int `json:"Y"`
}

func (p *G2Point) UnmarshalJSON(b []byte) error {
	var values struct {
		X [2]string `json:"X"`
		Y [2]string `json:"Y"`
	}
	err := json.Unmarshal(b, &values)
	if err != nil {
		fmt.Printf("Unmarshal %v\n", err)
		return err
	}
	for j, vj := range values.X {
		z := new(big.Int)
		_, ok := z.SetString(vj, 10)
		if !ok {
			return err
		}
		// swap so that it can be accepted by the verifier contract
		p.X[1-j] = z
	}
	for j, vj := range values.Y {
		z := new(big.Int)
		_, ok := z.SetString(vj, 10)
		if !ok {
			return err
		}
		// swap so that it can be accepted by the verifier contract
		p.Y[1-j] = z
	}
	return nil
}

func (p G2Point) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		X [2]string `json:"X"`
		Y [2]string `json:"Y"`
	}{
		X: [2]string{p.X[0].String(), p.X[1].String()},
		Y: [2]string{p.Y[0].String(), p.Y[1].String()},
	})
}

func readPublics(publicInputs []byte) ([]*big.Int, error) {
	var output []string
	if err := json.Unmarshal(publicInputs, &output); err != nil {
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

func readProof(proofRaw []byte) ([]byte, error) {
	var piOut = types.ProofData{}
	if err := json.Unmarshal(proofRaw, &piOut); err != nil {
		return nil, err
	}
	u2, _ := abi.NewType("uint256[2]", "", nil)
	u22, _ := abi.NewType("uint256[2][2]", "", nil)
	args := abi.Arguments{
		{Type: u2},
		{Type: u22},
		{Type: u2},
	}
	a, err := toG1Point(piOut.A[:2])
	if err != nil {
		return nil, err
	}
	b, err := toG2Point(piOut.B[:2])
	if err != nil {
		return nil, err
	}
	c, err := toG1Point(piOut.C[:2])
	if err != nil {
		return nil, err
	}
	values := []interface{}{[]*big.Int{a.X, a.Y}, [][]*big.Int{b.X[:], b.Y[:]}, []*big.Int{c.X, c.Y}}
	packed, err := args.Pack(values...)
	if err != nil {
		return nil, fmt.Errorf("%v, values: %v", err, values)
	}
	return packed, nil
}

func readMask(publicInputs []byte) (*big.Int, error) {
	var output []string
	if err := json.Unmarshal(publicInputs, &output); err != nil {
		return nil, err
	}
	if len(output) != 3 {
		return nil, fmt.Errorf("invalid public output")
	}
	mask, ok := new(big.Int).SetString(output[2], 0)
	if !ok {
		return nil, fmt.Errorf("invalid mask")
	}
	return mask, nil
}

func readProofFrom(proofFile string) ([]byte, error) {
	dat, err := os.ReadFile(proofFile)
	if err != nil {
		return nil, err
	}
	return readProof(dat)
}

func readPublicsFrom(publicFile string) ([]*big.Int, error) {
	dat, err := os.ReadFile(publicFile)
	if err != nil {
		return nil, err
	}
	return readPublics(dat)
}

func readMaskFrom(publicFile string) (*big.Int, error) {
	dat, err := os.ReadFile(publicFile)
	if err != nil {
		return nil, err
	}
	return readMask(dat)
}

// Generate public inputs (mode 2)
func GenerateInputs(encodingKeys []common.Hash, sampleIdxs []uint64) ([]byte, error) {
	var encodingKeyModStr, xInStr []string
	for i, sampleIdx := range sampleIdxs {
		if int(sampleIdx) >= eth.FieldElementsPerBlob {
			return nil, fmt.Errorf("chunk index out of scope: %d", sampleIdx)
		}
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
	return json.Marshal(inputObj)
}

// Generate public input (mode 1)
func GenerateInput(encodingKey common.Hash, sampleIdx uint64) ([]byte, error) {
	var b fr.Element
	var exp big.Int
	exp.Div(exp.Sub(fr.Modulus(), common.Big1), big.NewInt(int64(eth.FieldElementsPerBlob)))
	ru := b.Exp(*b.SetInt64(5), &exp)
	xIn := ru.Exp(*ru, big.NewInt(int64(sampleIdx)))
	encodingKeyMod := fr.Modulus().Mod(encodingKey.Big(), fr.Modulus())
	inputObj := InputPair{
		EncodingKeyIn: hexutil.Encode(encodingKeyMod.Bytes()),
		XIn:           xIn.String(),
	}
	return json.Marshal(inputObj)
}

func GenerateMask(encodingKey common.Hash, sampleIdx uint64) ([]byte, error) {
	if int(sampleIdx) >= eth.FieldElementsPerBlob {
		return nil, fmt.Errorf("sample index out of scope")
	}
	encodingKeyMod := fr.Modulus().Mod(encodingKey.Big(), fr.Modulus())
	masks, err := encoder.Encode(common.BigToHash(encodingKeyMod), eth.FieldElementsPerBlob*32)
	if err != nil {
		return nil, err
	}
	bytesIdx := sampleIdx * 32
	mask := masks[bytesIdx : bytesIdx+32]
	return mask, nil
}
