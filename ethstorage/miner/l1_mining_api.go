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
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
)

const gasBufferRatio = 1.2

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
	uint256Type, _ := abi.NewType("uint256", "", nil)
	addrType, _ := abi.NewType("address", "", nil)
	bytes32Array, _ := abi.NewType("bytes32[]", "", nil)
	bytesArray, _ := abi.NewType("bytes[]", "", nil)
	dataField, _ := abi.Arguments{
		{Type: uint256Type},
		{Type: uint256Type},
		{Type: addrType},
		{Type: uint256Type},
		{Type: bytes32Array},
		{Type: bytesArray},
	}.Pack(
		rst.blockNumber,
		new(big.Int).SetUint64(rst.startShardId),
		rst.miner,
		new(big.Int).SetUint64(rst.nonce),
		rst.encodedData,
		rst.proofs,
	)
	h := crypto.Keccak256Hash([]byte(`mine(uint256,uint256,address,uint256,bytes32[],bytes[])`))
	calldata := append(h[0:4], dataField...)

	gasPrice := cfg.GasPrice
	if gasPrice == nil {
		gasPrice, err := m.SuggestGasPrice(ctx)
		if err != nil {
			m.lg.Error("Query gas price failed", "error", err.Error())
			return common.Hash{}, err
		}
		m.lg.Debug("Query gas price done", "gasPrice", gasPrice)
	}
	tip := cfg.PriorityGasPrice
	if tip == nil {
		tip, err := m.SuggestGasTipCap(ctx)
		if err != nil {
			m.lg.Error("Query gas tip cap failed", "error", err.Error())
			tip = common.Big0
		}
		m.lg.Debug("Query gas tip cap done", "gasTipGap", tip)
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
	m.lg.Info("Estimated cost done", "cost(ether)", cost.Div(cost, big.NewInt(1e18)))
	reward, err := m.calculateReward(ctx, contract, rst.startShardId, rst.blockNumber)
	if err != nil {
		m.lg.Error("Calculate reward failed", "error", err.Error())
		return common.Hash{}, err
	}
	m.lg.Info("Estimated reward done", "reward(ether)", reward.Div(reward, big.NewInt(1e18)))
	if new(big.Int).Sub(reward, cost).Cmp(cfg.MinimumProfit) == -1 {
		return common.Hash{}, fmt.Errorf("reward is less than cost")
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
		m.lg.Error("Send tx failed", "error", err)
		return common.Hash{}, err
	}
	m.lg.Info("Submit mined result done", "shard", rst.startShardId, "block", rst.blockNumber,
		"nonce", rst.nonce, "txSigner", cfg.SignerAddr.Hex(), "hash", signedTx.Hash().Hex())
	return signedTx.Hash(), nil
}

func (m *l1MiningAPI) calculateReward(ctx context.Context, contract common.Address, shard uint64, block *big.Int) (*big.Int, error) {
	// load storage configurations from contract
	dcff, err := m.ReadContractField("dcfFactor")
	if err != nil {
		m.lg.Error("Failed to read dcfFactor", "error", err.Error())
		return nil, err
	}
	base := new(big.Int).Exp(big.NewInt(2), big.NewInt(128), nil)
	dcf := new(big.Rat).SetFrac(new(big.Int).SetBytes(dcff), base)
	fmt.Println("dcf in sec", dcf.FloatString(10))
	// dcf := new(big.Float).SetRat(dcfr)

	st, err := m.ReadContractField("startTime")
	if err != nil {
		m.lg.Error("Failed to read startTime", "error", err.Error())
		return nil, err
	}
	startTime := new(big.Int).SetBytes(st).Uint64()

	seb, err := m.ReadContractField("shardEntryBits")
	if err != nil {
		m.lg.Error("Failed to read shardEntryBits", "error", err.Error())
		return nil, err
	}
	shardEntryBits := new(big.Int).SetBytes(seb).Uint64()
	shardEntry := new(big.Int).Lsh(big.NewInt(1), uint(shardEntryBits))

	sc, err := m.ReadContractField("storageCost")
	if err != nil {
		m.lg.Error("Failed to read storageCost", "error", err.Error())
		return nil, err
	}
	storageCost := new(big.Int).SetBytes(sc)

	pa, err := m.ReadContractField("prepaidAmount")
	if err != nil {
		m.lg.Error("Failed to read prepaidAmount", "error", err.Error())
		return nil, err
	}
	prepaidAmount := new(big.Int).SetBytes(pa)

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

	plmt, err := m.ReadContractField("prepaidLastMineTime")
	if err != nil {
		m.lg.Error("Failed to read prepaidLastMineTime", "error", err.Error())
		return nil, err
	}
	prepaidLastMineTime := new(big.Int).SetBytes(plmt).Uint64()

	var lastShard uint64
	if lastKv > 0 {
		lastShard = (lastKv - 1) >> shardEntryBits
	}
	curBlock, err := m.HeaderByNumber(ctx, big.NewInt(rpc.LatestBlockNumber.Int64()))
	if err != nil {
		m.lg.Error("Failed to get latest block", "error", err.Error())
		return nil, err
	}
	// most optimistic block number to confirm the tx is latest block + 1
	minedTs := curBlock.Time - (new(big.Int).Sub(curBlock.Number, block).Uint64()+1)*12

	var reward *big.Int
	if shard < lastShard {
		basePayment := new(big.Int).Mul(storageCost, shardEntry)
		reward = paymentIn(basePayment, dcf, lastMineTime, minedTs, startTime)
	} else if shard == lastShard {
		basePayment := new(big.Int).Mul(storageCost, new(big.Int).Mod(new(big.Int).SetUint64(lastKv), shardEntry))
		reward = paymentIn(basePayment, dcf, lastMineTime, minedTs, startTime)
		// Additional prepaid for the last shard
		if prepaidLastMineTime < minedTs {
			additionalReward := paymentIn(prepaidAmount, dcf, prepaidLastMineTime, minedTs, startTime)
			reward = new(big.Int).Add(reward, additionalReward)
		}
	}
	return reward, nil
}

func paymentIn(x *big.Int, dcfFactor *big.Rat, fromTs, toTs, startTime uint64) *big.Int {
	pow0 := pow(dcfFactor, fromTs-startTime)
	pow1 := pow(dcfFactor, toTs-startTime)
	delta := new(big.Rat).Sub(pow0, pow1)
	return new(big.Int).Div(new(big.Int).Mul(x, delta.Num()), delta.Denom())
}

func pow(a *big.Rat, e uint64) *big.Rat {
	aen := new(big.Int).Exp(a.Num(), new(big.Int).SetUint64(e), nil)
	aed := new(big.Int).Exp(a.Denom(), new(big.Int).SetUint64(e), nil)
	return new(big.Rat).SetFrac(aen, aed)
}
