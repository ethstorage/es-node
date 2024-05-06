// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"context"
	"fmt"
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

const (
	gasBufferRatio    = 1.2
	rewardDenominator = 10000
)

var (
	mineSig = crypto.Keccak256Hash([]byte(`mine(uint256,uint256,address,uint256,bytes32[],uint256[],bytes,bytes[],bytes[])`))
)

func NewL1MiningAPI(l1 *eth.PollingClient, lg log.Logger) *l1MiningAPI {
	return &l1MiningAPI{l1, lg}
}

type l1MiningAPI struct {
	*eth.PollingClient
	lg log.Logger
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

func (m *l1MiningAPI) SubmitMinedResult(ctx context.Context, contract common.Address, rst result, cfg Config) (common.Hash, error) {
	m.lg.Debug("Submit mined result", "shard", rst.startShardId, "block", rst.blockNumber, "nonce", rst.nonce)
	blockHeader, err := m.HeaderByNumber(ctx, rst.blockNumber)
	if err != nil {
		m.lg.Error("Failed to get block header", "error", err)
		return common.Hash{}, err
	}
	headerRlp, err := rlp.EncodeToBytes(blockHeader)
	if err != nil {
		m.lg.Error("Failed to encode block header", "error", err)
		return common.Hash{}, err
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

	gasPrice := cfg.GasPrice
	if gasPrice == nil || gasPrice.Cmp(common.Big0) == 0 {
		suggested, err := m.SuggestGasPrice(ctx)
		if err != nil {
			m.lg.Error("Query gas price failed", "error", err.Error())
			return common.Hash{}, err
		}
		gasPrice = suggested
		m.lg.Info("Query gas price done", "gasPrice", gasPrice)
	}
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
	estimatedGas, err := m.EstimateGas(ctx, ethereum.CallMsg{
		From:      cfg.SignerAddr,
		To:        &contract,
		GasTipCap: tip,
		GasFeeCap: gasPrice,
		Value:     common.Big0,
		Data:      calldata,
	})
	if err != nil {
		m.lg.Error("Estimate gas failed", "error", err.Error())
		return common.Hash{}, fmt.Errorf("failed to estimate gas: %w", err)
	}
	m.lg.Info("Estimated gas done", "gas", estimatedGas)
	cost := new(big.Int).Mul(new(big.Int).SetUint64(estimatedGas), gasPrice)
	reward, err := m.estimateReward(ctx, cfg, contract, rst.startShardId, rst.blockNumber)
	if err != nil {
		m.lg.Error("Calculate reward failed", "error", err.Error())
		return common.Hash{}, err
	}
	profit := new(big.Int).Sub(reward, cost)
	m.lg.Info("Estimated reward and cost (in ether)", "reward", weiToEther(reward), "cost", weiToEther(cost), "profit", weiToEther(profit))
	if profit.Cmp(cfg.MinimumProfit) == -1 {
		m.lg.Warn("Will drop the tx: the profit will not meet expectation",
			"profitEstimated", weiToEther(profit),
			"minimumProfit", weiToEther(cfg.MinimumProfit),
		)
		return common.Hash{}, errDropped
	}

	chainID, err := m.NetworkID(ctx)
	if err != nil {
		m.lg.Error("Get chainID failed", "error", err.Error())
		return common.Hash{}, err
	}
	sign := cfg.SignerFnFactory(chainID)
	nonce, err := m.NonceAt(ctx, cfg.SignerAddr, big.NewInt(rpc.LatestBlockNumber.Int64()))
	if err != nil {
		m.lg.Error("Query nonce failed", "error", err.Error())
		return common.Hash{}, err
	}
	m.lg.Debug("Query nonce done", "nonce", nonce)
	gas := uint64(float64(estimatedGas) * gasBufferRatio)
	rawTx := &types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasTipCap: tip,
		GasFeeCap: gasPrice,
		Gas:       gas,
		To:        &contract,
		Value:     common.Big0,
		Data:      calldata,
	}
	signedTx, err := sign(ctx, cfg.SignerAddr, types.NewTx(rawTx))
	if err != nil {
		m.lg.Error("Sign tx error", "error", err)
		return common.Hash{}, err
	}
	err = m.SendTransaction(ctx, signedTx)
	if err != nil {
		m.lg.Error("Send tx failed", "txNonce", nonce, "gasPrice", gasPrice, "error", err)
		return common.Hash{}, err
	}
	m.lg.Info("Submit mined result done", "shard", rst.startShardId, "block", rst.blockNumber,
		"nonce", rst.nonce, "txSigner", cfg.SignerAddr.Hex(), "hash", signedTx.Hash().Hex())
	return signedTx.Hash(), nil
}

// TODO: implement `miningReward()` in the contract to replace this impl
func (m *l1MiningAPI) estimateReward(ctx context.Context, cfg Config, contract common.Address, shard uint64, block *big.Int) (*big.Int, error) {

	lastKv, err := m.PollingClient.GetStorageLastBlobIdx(rpc.LatestBlockNumber.Int64())
	if err != nil {
		m.lg.Error("Failed to get lastKvIdx", "error", err)
		return nil, err
	}
	info, err := m.GetMiningInfo(ctx, contract, shard)
	if err != nil {
		m.lg.Error("Failed to get es mining info", "error", err.Error())
		return nil, err
	}
	lastMineTime := info.LastMineTime

	plmt, err := m.ReadContractField("prepaidLastMineTime", nil)
	if err != nil {
		m.lg.Error("Failed to read prepaidLastMineTime", "error", err.Error())
		return nil, err
	}
	prepaidLastMineTime := new(big.Int).SetBytes(plmt).Uint64()

	var lastShard uint64
	if lastKv > 0 {
		lastShard = (lastKv - 1) / cfg.ShardEntry
	}
	curBlock, err := m.HeaderByNumber(ctx, big.NewInt(rpc.LatestBlockNumber.Int64()))
	if err != nil {
		m.lg.Error("Failed to get latest block", "error", err.Error())
		return nil, err
	}

	minedTs := curBlock.Time - (new(big.Int).Sub(curBlock.Number, block).Uint64())*12
	reward := big.NewInt(0)
	if shard < lastShard {
		basePayment := new(big.Int).Mul(cfg.StorageCost, new(big.Int).SetUint64(cfg.ShardEntry))
		reward = paymentIn(basePayment, cfg.DcfFactor, lastMineTime, minedTs, cfg.StartTime)
	} else if shard == lastShard {
		basePayment := new(big.Int).Mul(cfg.StorageCost, new(big.Int).SetUint64(lastKv%cfg.ShardEntry))
		reward = paymentIn(basePayment, cfg.DcfFactor, lastMineTime, minedTs, cfg.StartTime)
		// Additional prepaid for the last shard
		if prepaidLastMineTime < minedTs {
			additionalReward := paymentIn(cfg.PrepaidAmount, cfg.DcfFactor, prepaidLastMineTime, minedTs, cfg.StartTime)
			reward = new(big.Int).Add(reward, additionalReward)
		}
	}
	minerReward := new(big.Int).Div(
		new(big.Int).Mul(new(big.Int).SetUint64(rewardDenominator-cfg.TreasuryShare), reward),
		new(big.Int).SetUint64(rewardDenominator),
	)
	return minerReward, nil
}

func paymentIn(x, dcfFactor *big.Int, fromTs, toTs, startTime uint64) *big.Int {
	return new(big.Int).Rsh(
		new(big.Int).Mul(
			x,
			new(big.Int).Sub(
				pow(dcfFactor, fromTs-startTime),
				pow(dcfFactor, toTs-startTime),
			)),
		128,
	)
}

func pow(fp *big.Int, n uint64) *big.Int {
	v := new(big.Int).Lsh(big.NewInt(1), 128)
	for n != 0 {
		if (n & 1) == 1 {
			v = new(big.Int).Rsh(new(big.Int).Mul(v, fp), 128)
		}
		fp = new(big.Int).Rsh(new(big.Int).Mul(fp, fp), 128)
		n = n / 2
	}
	return v
}
