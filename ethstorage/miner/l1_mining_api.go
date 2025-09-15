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
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"golang.org/x/mod/semver"
)

const (
	gasBufferRatio         = 1.2
	versionMineRoleEnabled = "v0.1.2"
)

var (
	mineSig           = crypto.Keccak256Hash([]byte(`mine(uint256,uint256,address,uint256,bytes32[],uint256[],bytes,bytes[],bytes[])`))
	ether             = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	txFeeCapDefaultL2 = new(big.Int).Mul(big.NewInt(1000), ether)
	gasPriceOracleL2  = common.HexToAddress("0x420000000000000000000000000000000000000F")
)

func NewL1MiningAPI(l1 *eth.PollingClient, rc *eth.RandaoClient, lg log.Logger) *l1MiningAPI {
	return &l1MiningAPI{l1, rc, lg}
}

type l1MiningAPI struct {
	*eth.PollingClient
	rc *eth.RandaoClient
	lg log.Logger
}

// Pre-check if the miner has been whitelisted before actually mining
func (m *l1MiningAPI) CheckMinerRole(ctx context.Context, contract, miner common.Address) error {
	version, err := m.GetContractVersion()
	if err != nil {
		return fmt.Errorf("failed to get contract version: %w", err)
	}
	m.lg.Info("Storage Contract version", "version", version)
	if semver.Compare(version, versionMineRoleEnabled) == -1 {
		return nil
	}
	enforced, err := m.PollingClient.ReadContractField("enforceMinerRole", nil)
	if err != nil {
		return fmt.Errorf("failed to query enforceMinerRole(): %w", err)
	}
	if new(big.Int).SetBytes(enforced).Uint64() == 1 {
		m.lg.Info("Miner role enforced")

		addrType, _ := abi.NewType("address", "", nil)
		data, _ := abi.Arguments{{Type: addrType}}.Pack(miner)
		sig := crypto.Keccak256Hash([]byte(`hasMinerRole(address)`))
		calldata := append(sig[:4], data...)
		result, err := m.CallContract(ctx, ethereum.CallMsg{To: &contract, Data: calldata}, nil)
		if err != nil {
			return fmt.Errorf("failed to query hasMinerRole(): %w", err)
		}
		hasMinerRole := new(big.Int).SetBytes(result).Uint64()
		if hasMinerRole == 1 {
			m.lg.Info("Miner role granted", "miner", miner)
			return nil
		}
		return fmt.Errorf("miner role not granted to: %s", miner)
	}
	m.lg.Info("Miner role not enforced")
	return nil
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
		m.lg.Error("Failed to get versioned hashs", "error", err)
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
	calldata, err := m.composeCalldata(ctx, rst)
	if err != nil {
		m.lg.Error("Failed to compose calldata", "error", err)
		return common.Hash{}, err
	}
	m.lg.Info("Composed calldata", "calldata", hexutil.Encode(calldata))

	tip, gasFeeCap, useConfig, err := m.suggestGasPrices(ctx, cfg)
	if err != nil {
		m.lg.Error("Failed to suggest gas prices", "error", err)
		return common.Hash{}, err
	}

	// if gasFeeCap is higher than the configured max, drop the mined result
	if cfg.MaxGasPrice != nil && cfg.MaxGasPrice.Cmp(common.Big0) > 0 && gasFeeCap.Cmp(cfg.MaxGasPrice) == 1 {
		errMessage := fmt.Sprintf("gas price %s Gwei exceeds the configured max %s Gwei", fmtGwei(gasFeeCap), fmtGwei(cfg.MaxGasPrice))
		m.lg.Warn("Mining tx dropped", "error", errMessage)
		return common.Hash{}, errDropped{reason: errMessage}
	}

	estimatedGas, err := m.EstimateGas(ctx, ethereum.CallMsg{
		From:      cfg.SignerAddr,
		To:        &contract,
		GasTipCap: tip,
		GasFeeCap: gasFeeCap,
		Value:     common.Big0,
		Data:      calldata,
	})
	if err != nil {
		errMessage := parseErr(err)
		m.lg.Error("Estimate gas failed", "error", errMessage)
		return common.Hash{}, fmt.Errorf("failed to estimate gas: %v", errMessage)
	}
	m.lg.Info("Estimated gas done", "gas", estimatedGas)

	nonce, err := m.PendingNonceAt(ctx, cfg.SignerAddr)
	if err != nil {
		m.lg.Error("Query nonce failed", "error", err.Error())
		return common.Hash{}, err
	}
	m.lg.Debug("Query nonce done", "nonce", nonce)
	safeGas := uint64(float64(estimatedGas) * gasBufferRatio)

	unsignedTx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   m.NetworkID,
		Nonce:     nonce,
		GasTipCap: tip,
		GasFeeCap: gasFeeCap,
		Gas:       safeGas,
		To:        &contract,
		Value:     common.Big0,
		Data:      calldata,
	})
	gasFeeCapChecked, err := checkGasPrice(ctx, m, unsignedTx, rst, cfg.MinimumProfit, gasFeeCap, tip, estimatedGas, safeGas, m.rc != nil, useConfig, m.lg)
	if err != nil {
		return common.Hash{}, err
	}

	sign := cfg.SignerFnFactory(m.NetworkID)
	signedTx, err := sign(ctx, cfg.SignerAddr, unsignedTx)
	if err != nil {
		m.lg.Error("Sign tx error", "error", err)
		return common.Hash{}, err
	}
	err = m.SendTransaction(ctx, signedTx)
	if err != nil {
		m.lg.Error("Send tx failed", "txNonce", nonce, "gasFeeCap", gasFeeCapChecked, "error", err)
		return common.Hash{}, err
	}
	m.lg.Info("Submit mined result done", "shard", rst.startShardId, "block", rst.blockNumber,
		"nonce", rst.nonce, "txSigner", cfg.SignerAddr.Hex(), "hash", signedTx.Hash().Hex())
	return signedTx.Hash(), nil
}

func (m *l1MiningAPI) GetL1Fee(ctx context.Context, unsignedTx *types.Transaction) (*big.Int, error) {
	unsignedBin, err := unsignedTx.MarshalBinary()
	if err != nil {
		return nil, err
	}
	bytesType, _ := abi.NewType("bytes", "", nil)
	uint256Type, _ := abi.NewType("uint256", "", nil)
	dataField, _ := abi.Arguments{{Type: bytesType}}.Pack(unsignedBin)
	h := crypto.Keccak256Hash([]byte(`getL1Fee(bytes)`))
	bs, err := m.CallContract(ctx, ethereum.CallMsg{
		To:   &gasPriceOracleL2,
		Data: append(h[0:4], dataField...),
	}, nil)
	if err != nil {
		return nil, err
	}
	res, err := abi.Arguments{{Type: uint256Type}}.UnpackValues(bs)
	if err != nil {
		return nil, err
	}
	return res[0].(*big.Int), nil
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

func (m *l1MiningAPI) composeCalldata(ctx context.Context, rst result) ([]byte, error) {
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

func (m *l1MiningAPI) suggestGasPrices(ctx context.Context, cfg Config) (*big.Int, *big.Int, bool, error) {
	gasFeeCap := cfg.GasPrice
	tip := cfg.PriorityGasPrice
	useConfig := true
	if gasFeeCap == nil || gasFeeCap.Cmp(common.Big0) == 0 {
		useConfig = false
		blockHeader, err := m.HeaderByNumber(ctx, nil)
		if err != nil {
			m.lg.Error("Failed to get block header", "error", err)
			return nil, nil, false, err
		}
		m.lg.Info("Query baseFee done", "baseFee", blockHeader.BaseFee, "fromBlock", blockHeader.Number)
		if tip == nil || tip.Cmp(common.Big0) == 0 {
			suggested, err := m.SuggestGasTipCap(ctx)
			if err != nil {
				m.lg.Error("Query gas tip cap failed", "error", err.Error())
				suggested = common.Big0
			}
			tip = suggested
			m.lg.Info("Query gas tip cap done", "gasTipGap", tip)
		}
		// Use (tip + 2*baseFee) to avoid `max fee per gas less than block base fee` when estimate gas
		// It ensures the tx to be marketable for six consecutive 100% full blocks.
		gasFeeCap = new(big.Int).Add(new(big.Int).Mul(blockHeader.BaseFee, big.NewInt(2)), tip)
		m.lg.Info("Suggested gas fee cap (tip + 2*baseFee)", "gasFeeCap", gasFeeCap)
	} else {
		m.lg.Info("Using configured gas price", "gasFeeCap", gasFeeCap, "tip", tip)
	}
	return tip, gasFeeCap, useConfig, nil
}

type GasPriceChecker interface {
	GetL1Fee(ctx context.Context, tx *types.Transaction) (*big.Int, error)
	GetMiningReward(shardID uint64, timestamp uint64) (*big.Int, error)
}

// Adjust the gas price based on the estimated rewards, costs, and default tx fee cap.
func checkGasPrice(
	ctx context.Context,
	checker GasPriceChecker,
	unsignedTx *types.Transaction,
	rst result,
	minProfit, gasFeeCap, tip *big.Int,
	estimatedGas, safeGas uint64,
	useL2, useConfig bool,
	lg log.Logger,
) (*big.Int, error) {
	extraCost := new(big.Int)
	// Add L1 data fee as tx cost when es-node is deployed on an L2
	if useL2 {
		l1fee, err := checker.GetL1Fee(ctx, unsignedTx)
		if err != nil {
			lg.Warn("Failed to get L1 fee", "error", err)
		} else {
			lg.Info("Get L1 fee done", "l1Fee", l1fee)
			extraCost = l1fee
		}
	}
	reward, err := checker.GetMiningReward(rst.startShardId, rst.timestamp)
	if err != nil {
		lg.Warn("Query mining reward failed", "error", err.Error())
	}
	gasFeeCapChecked := gasFeeCap
	if reward != nil {
		lg.Info("Query mining reward done", "reward", reward)
		costCap := new(big.Int).Sub(reward, minProfit)
		txCostCap := new(big.Int).Sub(costCap, extraCost)
		lg.Debug("Tx cost cap", "txCostCap", txCostCap)
		profitableGasFeeCap := new(big.Int).Div(txCostCap, new(big.Int).SetUint64(estimatedGas))
		lg.Info("Minimum profitable gas fee cap", "gasFeeCap", profitableGasFeeCap)

		// gasFeeCap = 2*baseFee + tip
		baseFee := new(big.Int).Div(new(big.Int).Sub(gasFeeCap, tip), big.NewInt(2))
		// baseFee + tip would be the cheapest and most likely used for tx inclusion
		gasPrice := new(big.Int).Add(baseFee, tip)
		// Drop the tx if baseFee + tip is already higher than the profitable gas fee cap
		if gasPrice.Cmp(profitableGasFeeCap) == 1 {
			profit := new(big.Int).Sub(reward, new(big.Int).Mul(new(big.Int).SetUint64(estimatedGas), gasFeeCap))
			profit = new(big.Int).Sub(profit, extraCost)
			droppedMsg := fmt.Sprintf("not enough profit (in ether):\r\n reward %s - gas cost %s - extra cost %s = profit %s < min profit %s",
				fmtEth(reward), fmtEth(new(big.Int).Mul(new(big.Int).SetUint64(estimatedGas), gasFeeCap)), fmtEth(extraCost), fmtEth(profit), fmtEth(minProfit))
			return nil, errDropped{reason: droppedMsg}
		}
		// Cap the gas fee to be profitable
		if gasFeeCap.Cmp(profitableGasFeeCap) == 1 && !useConfig {
			gasFeeCapChecked = profitableGasFeeCap
			lg.Info("Using profitable gas fee cap", "gasFeeCap", gasFeeCapChecked)
		}
	}

	// Check gas fee against tx fee cap (e.g., --rpc.txfeecap as in geth) early to avoid tx fail
	txFeeCapDefault := ether
	if useL2 {
		txFeeCapDefault = txFeeCapDefaultL2
	}
	txFee := new(big.Int).Mul(new(big.Int).SetUint64(safeGas), gasFeeCapChecked)
	lg.Debug("Estimated tx fee on the safe side", "safeGas", safeGas, "txFee", txFee)
	if txFee.Cmp(txFeeCapDefault) == 1 {
		gasFeeCapChecked = new(big.Int).Div(txFeeCapDefault, new(big.Int).SetUint64(safeGas))
		lg.Warn("Tx fee exceeds the configured cap, lower the gasFeeCap", "txFee", fmtEth(txFee), "gasFeeCapUpdated", gasFeeCapChecked)
	}
	return gasFeeCapChecked, nil
}
