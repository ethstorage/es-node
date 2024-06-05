// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
)

const (
	gasBufferRatio = 1.2
)

var (
	mineSig = crypto.Keccak256Hash([]byte(`mine(uint256,uint256,address,uint256,bytes32[],uint256[],bytes,bytes[],bytes[])`))
)

func NewL1MiningAPI(l1 *eth.PollingClient, l1URL string, lg log.Logger) *l1MiningAPI {
	return &l1MiningAPI{l1, l1URL, lg}
}

type l1MiningAPI struct {
	*eth.PollingClient
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
	blockHeader, err := m.HeaderByNumber(ctx, rst.blockNumber)
	if err != nil {
		m.lg.Error("Failed to get block header", "error", err)
		return nil, err
	}
	headerRlp, err := rlp.EncodeToBytes(blockHeader)
	if err != nil {
		m.lg.Error("Failed to encode block header", "error", err)
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

func (m *l1MiningAPI) SuggestGasPrices(ctx context.Context, cfg Config) (*big.Int, *big.Int, *big.Int, error) {
	tip := cfg.PriorityGasPrice
	if tip == nil || tip.Cmp(common.Big0) == 0 {
		suggested, err := m.SuggestGasTipCap(ctx)
		if err != nil {
			m.lg.Error("Query gas tip cap failed", "error", err.Error())
			suggested = common.Big0
		}
		tip = suggested
		m.lg.Info("Query gas tip cap done", "gasTipGap", tip)
	}
	gasFeeCap := cfg.GasPrice
	predictedGasPrice := gasFeeCap
	if gasFeeCap == nil || gasFeeCap.Cmp(common.Big0) == 0 {
		blockHeader, err := m.HeaderByNumber(ctx, nil)
		if err != nil {
			m.lg.Error("Failed to get block header", "error", err)
			return nil, nil, nil, err
		}
		m.lg.Info("Query base fee done", "baseFee", blockHeader.BaseFee)
		// Doubling the base fee to ensure the transaction will remain marketable for six consecutive 100% full blocks.
		gasFeeCap = new(big.Int).Add(tip, new(big.Int).Mul(blockHeader.BaseFee, common.Big2))
		//	Use `tip + base fee` as predicted gas price that will be used to evaluate the mining profit
		predictedGasPrice = new(big.Int).Add(tip, blockHeader.BaseFee)
		m.lg.Info("Compute gas fee cap done", "gasFeeCap", gasFeeCap, "predictedGasPrice", predictedGasPrice)
	}
	return tip, gasFeeCap, predictedGasPrice, nil
}

func (m *l1MiningAPI) L1RPCURL() string {
	return m.l1URL
}
