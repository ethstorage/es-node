// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
)

var (
	mineSig = crypto.Keccak256Hash([]byte(`mine(uint256,uint256,address,uint256,bytes32[],uint256[],bytes,bytes[],bytes[])`))
)

func NewL1MiningAPI(l1 *eth.PollingClient, rc *eth.RandaoClient, l1URL string, lg log.Logger) *l1MiningAPI {
	return &l1MiningAPI{l1, rc, l1URL, lg}
}

type l1MiningAPI struct {
	*eth.PollingClient
	rc    *eth.RandaoClient
	l1URL string
	lg    log.Logger
}

func (m *l1MiningAPI) GetMiningInfo(ctx context.Context, contract common.Address, shardIdx uint64) (*miningInfo, error) {
	uint256Type, _ := abi.NewType("uint256", "", nil)
	dataField, _ := abi.Arguments{{Type: uint256Type}}.Pack(new(big.Int).SetUint64(shardIdx))
	h := crypto.Keccak256Hash([]byte(`infos(uint256)`))
	calldata := append(h[0:4], dataField...)
	msg := ethereum.CallMsg{
		To:   &contract,
		Data: calldata,
	}
	bs, err := m.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, err
	}
	res, err := abi.Arguments{
		{Type: uint256Type},
		{Type: uint256Type},
		{Type: uint256Type},
	}.UnpackValues(bs)
	if err != nil {
		return nil, err
	}
	mi := &miningInfo{
		LastMineTime: res[0].(*big.Int).Uint64(),
		Difficulty:   res[1].(*big.Int),
		BlockMined:   res[2].(*big.Int),
	}
	return mi, nil
}

func (m *l1MiningAPI) GetDataHashes(ctx context.Context, contract common.Address, kvIdxes []uint64) ([]common.Hash, error) {
	metas, err := m.GetKvMetas(kvIdxes, rpc.LatestBlockNumber.Int64())
	if err != nil {
		m.lg.Error("Failed to get verioned hashs", "error", err)
		return nil, err
	}
	var hashes []common.Hash
	for i := 0; i < len(metas); i++ {
		var dhash common.Hash
		copy(dhash[:], metas[i][32-ethstorage.HashSizeInContract:32])
		m.lg.Info("Get data hash", "kvIndex", kvIdxes[i], "hash", dhash.Hex())
		hashes = append(hashes, dhash)
	}
	return hashes, nil
}

func (m *l1MiningAPI) ComposeCalldata(ctx context.Context, rst result) ([]byte, error) {
	headerRlp, err := m.getRandaoProof(ctx, rst.blockNumber)
	if err != nil {
		m.lg.Error("Failed to get randao proof", "error", err)
		return nil, err
	}
	uint256Type, _ := abi.NewType("uint256", "", nil)
	uint256Array, _ := abi.NewType("uint256[]", "", nil)
	addrType, _ := abi.NewType("address", "", nil)
	bytes32Array, _ := abi.NewType("bytes32[]", "", nil)
	bytesArray, _ := abi.NewType("bytes[]", "", nil)
	bytesType, _ := abi.NewType("bytes", "", nil)
	dataField, _ := abi.Arguments{
		{Type: uint256Type},
		{Type: uint256Type},
		{Type: addrType},
		{Type: uint256Type},
		{Type: bytes32Array},
		{Type: uint256Array},
		{Type: bytesType},
		{Type: bytesArray},
		{Type: bytesArray},
	}.Pack(
		rst.blockNumber,
		new(big.Int).SetUint64(rst.startShardId),
		rst.miner,
		new(big.Int).SetUint64(rst.nonce),
		rst.encodedData,
		rst.masks,
		headerRlp,
		rst.inclusiveProofs,
		rst.decodeProof,
	)
	calldata := append(mineSig[0:4], dataField...)
	return calldata, nil
}

func (m *l1MiningAPI) getRandaoProof(ctx context.Context, blockNumber *big.Int) ([]byte, error) {
	var caller interface {
		HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
	}
	if m.rc != nil {
		caller = m.rc
	} else {
		caller = m.Client
	}
	blockHeader, err := caller.HeaderByNumber(ctx, blockNumber)
	if err != nil {
		m.lg.Error("Failed to get block header", "number", blockNumber, "error", err)
		return nil, err
	}
	headerRlp, err := rlp.EncodeToBytes(blockHeader)
	if err != nil {
		m.lg.Error("Failed to encode block header in RLP", "error", err)
		return nil, err
	}
	return headerRlp, nil
}

func (m *l1MiningAPI) L1Info() (*big.Int, string) {
	return m.NetworkID, m.l1URL
}
