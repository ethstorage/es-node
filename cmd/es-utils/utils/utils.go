// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package utils

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/holiman/uint256"
)

var (
	log = esLog.NewLogger(esLog.DefaultCLIConfig())
)

func SendBlobTx(
	addr string,
	to common.Address,
	prv string,
	data []byte,
	needEncoding bool,
	nonce int64,
	value string,
	gasLimit uint64,
	gasPrice string,
	priorityGasPrice string,
	maxFeePerDataGas string,
	chainID string,
	calldata string,
) *types.Transaction {
	chainId, _ := new(big.Int).SetString(chainID, 0)

	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, addr)
	if err != nil {
		log.Crit("Failed to connect to the Ethereum client", "error", err)
	}

	h := crypto.Keccak256Hash([]byte(`upfrontPayment()`))
	callMsg := ethereum.CallMsg{
		To: &to,
		Data: h[:],
	}
	bs, err := client.CallContract(context.Background(), callMsg, new(big.Int).SetInt64(-2))
	if err != nil {
		log.Crit("Failed to get upfront fee", "error", err)
	}

	uint256Type, _ := abi.NewType("uint256", "", nil)
	res, err := abi.Arguments{{Type: uint256Type}}.UnpackValues(bs)
	if err != nil {
		log.Crit("Failed to unpack values", "error", err)
	}

	val, success := new(big.Int).SetString(value, 0)
	if !success {
		log.Crit("Invalid value param")
	}

	if res[0].(* big.Int).Cmp(val) == 1 {
		val = res[0].(* big.Int)
	}

	value256, overflow := uint256.FromBig(val)
	if overflow {
		log.Crit("Invalid value param", "error", err)
	}

	key, err := crypto.HexToECDSA(prv)
	if err != nil {
		log.Crit("Invalid private key", "error", err)
	}

	if nonce == -1 {
		pendingNonce, err := client.PendingNonceAt(ctx, crypto.PubkeyToAddress(key.PublicKey))
		if err != nil {
			log.Crit("Error getting nonce", "error", err)
		}
		nonce = int64(pendingNonce)
	}

	var gasPrice256 *uint256.Int
	if gasPrice == "" {
		val, err := client.SuggestGasPrice(ctx)
		if err != nil {
			log.Crit("Error getting suggested gas price", "error", err)
		}
		var nok bool
		gasPrice256, nok = uint256.FromBig(val)
		if nok {
			log.Crit("Gas price is too high!", "value", val.String())
		}
	} else {
		gasPrice256, err = DecodeUint256String(gasPrice)
		if err != nil {
			log.Crit("Invalid gas price", "error", err)
		}
	}

	priorityGasPrice256 := gasPrice256
	if priorityGasPrice != "" {
		priorityGasPrice256, err = DecodeUint256String(priorityGasPrice)
		if err != nil {
			log.Crit("Invalid priority gas price", "error", err)
		}
	}

	maxFeePerDataGas256, err := DecodeUint256String(maxFeePerDataGas)
	if err != nil {
		log.Crit("Invalid max_fee_per_data_gas", "error", err)
	}
	var blobs []kzg4844.Blob
	if needEncoding {
		blobs = EncodeBlobs(data)
	} else {
		blobs = ConvertToBlobs(data)
	}
	commitments, proofs, versionedHashes, err := ComputeBlobs(blobs)
	if err != nil {
		log.Crit("Failed to compute commitments", "error", err)
	}

	calldataBytes, err := common.ParseHexOrString(calldata)
	if err != nil {
		log.Crit("Failed to parse calldata", "error", err)
	}
	sideCar := &types.BlobTxSidecar{
		Blobs:       blobs,
		Commitments: commitments,
		Proofs:      proofs,
	}
	blobtx := &types.BlobTx{
		ChainID:    uint256.MustFromBig(chainId),
		Nonce:      uint64(nonce),
		GasTipCap:  priorityGasPrice256,
		GasFeeCap:  gasPrice256,
		Gas:        gasLimit,
		To:         to,
		Value:      value256,
		Data:       calldataBytes,
		BlobFeeCap: maxFeePerDataGas256,
		BlobHashes: versionedHashes,
		Sidecar:    sideCar,
	}
	tx := types.MustSignNewTx(key, types.NewCancunSigner(chainId), blobtx)
	err = client.SendTransaction(context.Background(), tx)
	if err != nil {
		log.Crit("Unable to send transaction", "error", err)
	}

	for {
		// receipt, err = client.TransactionReceipt(context.Background(), txWithBlobs.Transaction.Hash())
		txn, isPending, err := client.TransactionByHash(context.Background(), tx.Hash())
		if err != nil || isPending {
			time.Sleep(1 * time.Second)
		} else {
			tx = txn
			break
		}
	}

	log.Info("Transaction submitted.", "nonce", nonce, "hash", tx.Hash(), "blobs", len(blobs))
	return tx
}

func ConvertToBlobs(data []byte) []kzg4844.Blob {
	blobs := []kzg4844.Blob{}
	blobIndex := 0
	for i := 0; i < len(data); i += params.BlobTxFieldElementsPerBlob * 32 {
		max := i + params.BlobTxFieldElementsPerBlob*32
		if max > len(data) {
			max = len(data)
		}
		blobs = append(blobs, kzg4844.Blob{})
		copy(blobs[blobIndex][:], data[i:max])
		blobIndex++
	}
	return blobs
}

func ComputeBlobs(blobs []kzg4844.Blob) ([]kzg4844.Commitment, []kzg4844.Proof, []common.Hash, error) {
	var (
		commits         []kzg4844.Commitment
		proofs          []kzg4844.Proof
		versionedHashes []common.Hash
	)
	for _, blob := range blobs {
		commit, err := kzg4844.BlobToCommitment(blob)
		if err != nil {
			return nil, nil, nil, err
		}
		commits = append(commits, commit)

		proof, err := kzg4844.ComputeBlobProof(blob, commit)
		if err != nil {
			return nil, nil, nil, err
		}
		proofs = append(proofs, proof)

		versionedHashes = append(versionedHashes, kZGToVersionedHash(commit))
	}
	return commits, proofs, versionedHashes, nil
}

var blobCommitmentVersionKZG uint8 = 0x01

// kZGToVersionedHash implements kzg_to_versioned_hash from EIP-4844
func kZGToVersionedHash(kzg kzg4844.Commitment) common.Hash {
	h := sha256.Sum256(kzg[:])
	h[0] = blobCommitmentVersionKZG

	return h
}

func EncodeBlobs(data []byte) []kzg4844.Blob {
	blobs := []kzg4844.Blob{{}}
	blobIndex := 0
	fieldIndex := -1
	for i := 0; i < len(data); i += 31 {
		fieldIndex++
		if fieldIndex == params.BlobTxFieldElementsPerBlob {
			blobs = append(blobs, kzg4844.Blob{})
			blobIndex++
			fieldIndex = 0
		}
		max := i + 31
		if max > len(data) {
			max = len(data)
		}
		copy(blobs[blobIndex][fieldIndex*32+1:], data[i:max])
	}
	return blobs
}

func DecodeBlob(blob []byte) []byte {
	if len(blob) != params.BlobTxFieldElementsPerBlob*32 {
		panic("Invalid blob encoding")
	}
	var data []byte

	// XXX: the following removes trailing 0s in each field element (see EncodeBlobs), which could be unexpected for certain blobs
	j := 0
	for i := 0; i < params.BlobTxFieldElementsPerBlob; i++ {
		data = append(data, blob[j+1:j+32]...)
		j += 32
	}

	i := len(data) - 1
	for ; i >= 0; i-- {
		if data[i] != 0x00 {
			break
		}
	}
	data = data[:i+1]
	return data
}

func DecodeUint256String(hexOrDecimal string) (*uint256.Int, error) {
	var base = 10
	if strings.HasPrefix(hexOrDecimal, "0x") {
		base = 16
	}
	b, ok := new(big.Int).SetString(hexOrDecimal, base)
	if !ok {
		return nil, fmt.Errorf("invalid value")
	}
	val256, nok := uint256.FromBig(b)
	if nok {
		return nil, fmt.Errorf("value is too big")
	}
	return val256, nil
}

// upload blobs and call putBlobs(bytes32[] memory keys)
// and returns the kv indexes and the data hashes
func UploadBlobs(
	pc *eth.PollingClient,
	rpc, private, chainID string,
	contractAddr common.Address,
	data []byte,
	needEncoding bool,
	value string) ([]uint64, []common.Hash, error) {
	key, err := crypto.HexToECDSA(private)
	if err != nil {
		log.Error("Invalid private key", "err", err)
		return nil, nil, err
	}
	signer := crypto.PubkeyToAddress(key.PublicKey)
	var keys []common.Hash

	var blobs []kzg4844.Blob
	if needEncoding {
		blobs = EncodeBlobs(data)
	} else {
		blobs = ConvertToBlobs(data)
	}
	for i, blob := range blobs {
		keys = append(keys, genKey(signer, i, blob[:]))
	}
	bytes32Array, _ := abi.NewType("bytes32[]", "", nil)
	dataField, _ := abi.Arguments{{Type: bytes32Array}}.Pack(keys)
	h := crypto.Keccak256Hash([]byte("putBlobs(bytes32[])"))
	calldata := "0x" + common.Bytes2Hex(append(h[0:4], dataField...))
	tx := SendBlobTx(
		rpc,
		contractAddr,
		private,
		data,
		needEncoding,
		-1,
		value,
		1000000,
		"",
		"400000000",
		"4000000000",
		chainID,
		calldata,
	)
	log.Info("SendBlobTx done.", "txHash", tx.Hash())
	resultCh := make(chan *types.Receipt, 1)
	errorCh := make(chan error, 1)
	revert := fmt.Errorf("revert")
	go func() {
		receipt, err := bind.WaitMined(context.Background(), pc.Client, tx)
		if err != nil {
			log.Error("Get transaction receipt err", "error", err)
			errorCh <- err
		}
		if receipt.Status == 0 {
			log.Error("Blob transaction reverted")
			errorCh <- revert
			return
		}
		log.Info("Blob transaction confirmed successfully", "txHash", tx.Hash())
		resultCh <- receipt
	}()

	select {
	// try to get data hash from events first
	case receipt := <-resultCh:
		log.Info("receipt returned", "gasUsed", receipt.GasUsed)
		var dataHashs []common.Hash
		var kvIndexes []uint64
		for i := range receipt.Logs {
			eventTopics := receipt.Logs[i].Topics
			kvIndex := new(big.Int).SetBytes(eventTopics[1][:]).Uint64()
			dataHash := eventTopics[3]
			dataHashs = append(dataHashs, dataHash)
			kvIndexes = append(kvIndexes, kvIndex)
		}
		return kvIndexes, dataHashs, nil
	case err := <-errorCh:
		log.Error("Get transaction receipt err", "error", err)
		if err == revert {
			return nil, nil, err
		}
	case <-time.After(5 * time.Second):
		log.Info("Timed out for receipt, query contract for data hash...")
	}
	// if wait receipt timed out or failed, query contract for data hash
	return getKvInfo(pc, contractAddr, len(blobs))
}

func getKvInfo(pc *eth.PollingClient, contractAddr common.Address, blobLen int) ([]uint64, []common.Hash, error) {
	lastIdx, err := pc.GetStorageLastBlobIdx(rpc.LatestBlockNumber.Int64())
	if err != nil {
		return nil, nil, err
	}
	var kvIndices []uint64
	for i := lastIdx - uint64(blobLen); i < lastIdx; i++ {
		kvIndices = append(kvIndices, i)
	}
	metas, err := pc.GetKvMetas(kvIndices, rpc.LatestBlockNumber.Int64())
	if err != nil {
		log.Error("Failed to get verioned hashs", "error", err)
		return nil, nil, err
	}
	if len(metas) != len(kvIndices) {
		return nil, nil, errors.New("invalid params lens")
	}
	var hashes []common.Hash
	for i := 0; i < len(metas); i++ {
		var dhash common.Hash
		copy(dhash[:], metas[i][32-ethstorage.HashSizeInContract:32])
		log.Info("Get data hash", "kvIndex", kvIndices[i], "hash", dhash.Hex())
		hashes = append(hashes, dhash)
	}
	return kvIndices, hashes, nil
}

func genKey(addr common.Address, blobIndex int, data []byte) common.Hash {
	keySource := addr.Bytes()
	keySource = append(keySource, big.NewInt(time.Now().UnixNano()).Bytes()...)
	keySource = append(keySource, data...)
	keySource = append(keySource, byte(blobIndex))
	return crypto.Keccak256Hash(keySource)
}
