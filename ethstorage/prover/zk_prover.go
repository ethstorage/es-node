// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/crate-crypto/go-proto-danksharding-crypto/eth"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
)

const (
	snarkLibDir      = "snarkjs"
	snarkBuildDir    = "snarkbuild"
	witnessGenerator = "generate_witness.js"
	inputName        = "input_blob_poseidon.json"
	wasmName         = "blob_poseidon.wasm"
	wtnsName         = "witness_blob_poseidon.wtns"
	proofName        = "proof_blob_poseidon.json"
	publicName       = "public_blob_poseidon.json"
)

type ZKProver struct {
	dir, zkeyFile string
	lg            log.Logger
	cleanup       bool
}

func NewZKProver(workingDir, zkeyFile string, lg log.Logger) *ZKProver {
	return newZKProver(workingDir, zkeyFile, true, lg)
}

func NewZKProverInternal(workingDir, zkeyFile string, lg log.Logger) *ZKProver {
	return newZKProver(workingDir, zkeyFile, false, lg)
}

func newZKProver(workingDir, zkeyFile string, cleanup bool, lg log.Logger) *ZKProver {
	path := workingDir
	if path == "" {
		path, _ = filepath.Abs("./")
	}
	libDir := filepath.Join(path, snarkLibDir)
	if _, err := os.Stat(libDir); errors.Is(err, os.ErrNotExist) {
		lg.Crit("Init ZK prover failed", "error", "snark lib does not exist", "dir", libDir)
	}
	return &ZKProver{
		dir:      path,
		zkeyFile: zkeyFile,
		cleanup:  cleanup,
		lg:       lg,
	}
}

// Generate ZK Proof for the given encoding keys and chunck indexes using snarkjs
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
		p.lg.Info("Generate zk proof done", "sampleIdx", sampleIdxs, "took(sec)", dur.Seconds())
	}(start)

	tempDir := crypto.Keccak256Hash([]byte(fmt.Sprint(encodingKeys, sampleIdxs)))
	p.lg.Debug("Generate zk proof", "path", common.Bytes2Hex(tempDir[:]))
	buildDir := filepath.Join(p.dir, snarkBuildDir, common.Bytes2Hex(tempDir[:]))
	if _, err := os.Stat(buildDir); err == nil {
		os.RemoveAll(buildDir)
	}
	err := os.Mkdir(buildDir, os.ModePerm)
	if err != nil {
		p.lg.Error("Generate zk proof failed", "mkdir", buildDir, "error", err)
		return nil, nil, err
	}
	defer func() {
		if p.cleanup {
			e := os.RemoveAll(buildDir)
			if e != nil {
				p.lg.Warn("Remove folder error", "dir", buildDir, "error", e)
			}
		}
	}()

	libDir := filepath.Join(p.dir, snarkLibDir)
	// 1. Generate input
	inputFile := filepath.Join(buildDir, inputName)
	file, err := os.OpenFile(inputFile, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		p.lg.Error("Create input file failed", "error", err)
		return nil, nil, err
	}
	defer file.Close()

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
	err = json.NewEncoder(file).Encode(inputObj)
	if err != nil {
		p.lg.Error("Write input file failed", "error", err)
		return nil, nil, err
	}
	p.lg.Debug("Generate zk proof", "input", inputObj)

	// 2. Generate witness
	wtnsFile := filepath.Join(buildDir, wtnsName)
	cmd := exec.Command("node",
		filepath.Join(libDir, witnessGenerator),
		filepath.Join(libDir, wasmName),
		inputFile,
		wtnsFile,
	)
	cmd.Dir = libDir
	out, err := cmd.Output()
	if err != nil {
		p.lg.Error("Generate witness failed", "error", err, "cmd", cmd.String(), "output", string(out))
		return nil, nil, err
	}
	p.lg.Debug("Generate witness done")

	// 3. Generate proof
	proofFile := filepath.Join(buildDir, proofName)
	publicFile := filepath.Join(buildDir, publicName)
	cmd = exec.Command("snarkjs", "groth16", "prove",
		filepath.Join(libDir, p.zkeyFile),
		wtnsFile,
		proofFile,
		publicFile,
	)
	cmd.Dir = libDir
	out, err = cmd.Output()
	if err != nil {
		p.lg.Error("Generate proof failed", "error", err, "cmd", cmd.String(), "output", string(out))
		return nil, nil, err
	}
	p.lg.Debug("Generate proof done")

	// 4. Read proof and masks
	proof, err := readProof(proofFile)
	if err != nil {
		p.lg.Error("Parse proof failed", "error", err)
		return nil, nil, err
	}
	masks, err := readMasks(publicFile)
	if err != nil {
		p.lg.Error("Read mask failed", "error", err)
		return nil, nil, err
	}
	return proof, masks, nil
}

// Generate ZK Proof for the given encoding key and chunck index using snarkjs
func (p *ZKProver) GenerateZKProofPerSample(encodingKey common.Hash, sampleIdx uint64) ([]byte, *big.Int, error) {
	p.lg.Debug("Generate zk proof", "encodingKey", encodingKey.Hex(), "sampleIdx", sampleIdx)
	if int(sampleIdx) >= eth.FieldElementsPerBlob {
		return nil, nil, fmt.Errorf("chunk index out of scope: %d", sampleIdx)
	}
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		p.lg.Info("Generate zk proof", "sampleIdx", sampleIdx, "took(sec)", dur.Seconds())
	}(start)
	buildDir := filepath.Join(p.dir, snarkBuildDir, strings.Join([]string{
		encodingKey.Hex(),
		fmt.Sprint(sampleIdx),
	}, "-"))
	if _, err := os.Stat(buildDir); err == nil {
		os.RemoveAll(buildDir)
	}
	err := os.Mkdir(buildDir, os.ModePerm)
	if err != nil {
		p.lg.Error("Generate zk proof failed", "mkdir", buildDir, "error", err)
		return nil, nil, err
	}
	defer func() {
		if p.cleanup {
			e := os.RemoveAll(buildDir)
			if e != nil {
				p.lg.Warn("Remove folder error", "dir", buildDir, "error", e)
			}
		}
	}()

	libDir := filepath.Join(p.dir, snarkLibDir)
	// 1. Generate input
	inputFile := filepath.Join(buildDir, inputName)
	file, err := os.OpenFile(inputFile, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		p.lg.Error("Create input file failed", "error", err)
		return nil, nil, err
	}
	defer file.Close()

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
	err = json.NewEncoder(file).Encode(inputObj)
	if err != nil {
		p.lg.Error("Write input file failed", "error", err)
		return nil, nil, err
	}
	p.lg.Debug("Generate zk proof", "input", inputObj)

	// 2. Generate witness
	wtnsFile := filepath.Join(buildDir, wtnsName)
	cmd := exec.Command("node",
		filepath.Join(libDir, witnessGenerator),
		filepath.Join(libDir, wasmName),
		inputFile,
		wtnsFile,
	)
	cmd.Dir = libDir
	out, err := cmd.Output()
	if err != nil {
		p.lg.Error("Generate witness failed", "error", err, "cmd", cmd.String(), "output", string(out))
		return nil, nil, err
	}
	p.lg.Debug("Generate witness done")

	// 3. Generate proof
	proofFile := filepath.Join(buildDir, proofName)
	publicFile := filepath.Join(buildDir, publicName)
	cmd = exec.Command("snarkjs", "groth16", "prove",
		filepath.Join(libDir, p.zkeyFile),
		wtnsFile,
		proofFile,
		publicFile,
	)
	cmd.Dir = libDir
	out, err = cmd.Output()
	if err != nil {
		p.lg.Error("Generate proof failed", "error", err, "cmd", cmd.String(), "output", string(out))
		return nil, nil, err
	}
	p.lg.Debug("Generate proof done")

	// 4. Read proof and mask
	proof, err := readProof(proofFile)
	if err != nil {
		p.lg.Error("Parse proof failed", "error", err)
		return nil, nil, err
	}
	mask, err := readMask(publicFile)
	if err != nil {
		p.lg.Error("Read mask failed", "error", err)
		return nil, nil, err
	}
	return proof, mask, nil
}

func readProof(proofFile string) ([]byte, error) {
	dat, err := os.ReadFile(proofFile)
	if err != nil {
		return nil, err
	}
	var piOut = pi{}
	err = json.Unmarshal(dat, &piOut)
	if err != nil {
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
	b, err := toG2Point(piOut.B[0:2])
	if err != nil {
		return nil, err
	}
	c, err := toG1Point(piOut.C[0:2])
	if err != nil {
		return nil, err
	}
	values := []interface{}{[]*big.Int{a.X, a.Y}, [][]*big.Int{b.X[:], b.Y[:]}, []*big.Int{c.X, c.Y}}
	encoded, err := args.Pack(values...)
	if err != nil {
		return nil, fmt.Errorf("%v, values: %v", err, values)
	}
	return encoded, nil
}

func readMasks(publicFile string) ([]*big.Int, error) {
	var masks []*big.Int
	f, err := os.Open(publicFile)
	if err != nil {
		return masks, err
	}
	defer f.Close()
	var output []string
	var decoder = json.NewDecoder(f)

	if err = decoder.Decode(&output); err != nil {
		return masks, err
	}
	for _, v := range output {
		mask, ok := new(big.Int).SetString(v, 0)
		if !ok {
			return masks, fmt.Errorf("invalid mask %v", v)
		}
		masks = append(masks, mask)
	}
	return masks, nil
}

func readMask(publicFile string) (*big.Int, error) {
	f, err := os.Open(publicFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var output []string
	var decoder = json.NewDecoder(f)
	err = decoder.Decode(&output)
	if err != nil {
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

func toG2Point(s [][2]string) (G2Point, error) {
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

type ZKProof struct {
	A G1Point `json:"A"`
	B G2Point `json:"B"`
	C G1Point `json:"C"`
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

// original proof structure produced by snarkjs
type pi struct {
	A        [3]string    `json:"pi_a"`
	B        [3][2]string `json:"pi_b"`
	C        [3]string    `json:"pi_c"`
	Protocal string       `json:"protocol"`
	Curve    string       `json:"curve"`
}

// input structure used by snarkjs
type InputPairV2 struct {
	EncodingKeyIn []string `json:"encodingKeyIn"`
	XIn           []string `json:"xIn"`
}

type InputPair struct {
	EncodingKeyIn string `json:"encodingKeyIn"`
	XIn           string `json:"xIn"`
}
