// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	ethRPC "github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
)

const (
	dbKey_Prefix_LastBlock = "lastBlock"
	step                   = 500
	epoch                  = 12 * time.Second
	l1Type                 = "l1"
	l2Type                 = "l2"
)

var divisor = new(big.Int).SetUint64(10000000000)

var (
	configFileFlag = flag.String("config", "config.json", "File contain the config params to init dashboard")
	listenAddrFlag = flag.String("address", "0.0.0.0", "Listener address")
	portFlag       = flag.Int("port", 8300, "Listener port for the devp2p connection")
	dataPath       = flag.String("datadir", "./es-data", "Data directory for the databases")
	logFlag        = flag.Int("loglevel", 3, "Log level to use for Ethereum and the faucet")
)

type miningEvent struct {
	TxHash       common.Hash
	ShardId      uint64
	LastMineTime uint64
	Difficulty   *big.Int
	BlockMined   *big.Int
	Miner        common.Address
	GasFee       *big.Int
	Reward       *big.Int
}

type Param struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Rpc         string `json:"rpc"`
	Contract    string `json:"contract"`
	StartNumber uint64 `json:"startNumber"`
}

type dashboard struct {
	ctx         context.Context
	sourceType  string
	source      *eth.PollingClient
	m           metrics.Metricer
	contract    common.Address
	kvEntries   uint64
	maxShardIdx uint64
	startBlock  uint64
	endBlock    uint64
	db          ethdb.Database
	logger      log.Logger
}

func newDashboard(param *Param, db ethdb.Database, m metrics.Metricer) (*dashboard, error) {
	var (
		logger   = log.New("app", "Dashboard")
		ctx      = context.Background()
		contract = common.HexToAddress(param.Contract)
	)

	if param.Type != l2Type && param.Type != l1Type {
		log.Crit("Invalid source type for param", "name", param.Name)
	}

	source, err := eth.Dial(param.Rpc, contract, 12, logger)
	if err != nil {
		log.Crit("Failed to create L1 source", "err", err)
	}

	start := param.StartNumber
	if status, _ := db.Get([]byte(fmt.Sprintf("%s-%s", dbKey_Prefix_LastBlock, contract.Hex()))); status != nil {
		start = new(big.Int).SetBytes(status).Uint64()
	}

	if start == 0 {
		block, err := source.BlockByNumber(ctx, new(big.Int).SetInt64(ethRPC.LatestBlockNumber.Int64()))
		if err != nil {
			log.Crit("Failed to fetch start block", "err", err)
		}
		start = block.NumberU64()
		if start == 0 {
			log.Crit("Start block should not be 0")
		}
	}

	result, err := source.ReadContractField("shardEntryBits", nil)
	if err != nil {
		return nil, err
	}
	shardEntryBits := new(big.Int).SetBytes(result).Uint64()

	return &dashboard{
		ctx:        ctx,
		sourceType: param.Type,
		source:     source,
		m:          m,
		contract:   contract,
		kvEntries:  1 << shardEntryBits,
		db:         db,
		startBlock: start,
		endBlock:   start - 1,
		logger:     logger,
	}, nil
}

func (d *dashboard) RefreshMetrics(ctx context.Context, sig eth.L1BlockRef) {
	d.RefreshBlobsMetrics(sig)
	d.RefreshMiningMetrics()
}

func (d *dashboard) RefreshBlobsMetrics(sig eth.L1BlockRef) {
	lastKVIndex, err := d.source.GetStorageLastBlobIdx(int64(sig.Number))
	if err != nil {
		log.Warn("Refresh contract metrics (last kv index) failed", "err", err.Error())
		return
	}
	maxShardIdx := lastKVIndex / d.kvEntries
	d.m.SetLastKVIndexAndMaxShardId(d.contract, sig.Number, lastKVIndex, maxShardIdx)
	d.logger.Info("RefreshBlobMetrics", "contract", d.contract, "blockNumber", sig.Number, "lastKvIndex", lastKVIndex, "maxShardIdx", maxShardIdx)
	d.maxShardIdx = maxShardIdx
	if sig.Number > d.endBlock {
		d.endBlock = sig.Number
	}
}

func (d *dashboard) RefreshMiningMetrics() {
	for d.startBlock <= d.endBlock {
		start, end := d.startBlock, d.endBlock
		if end > start+step {
			end = start + step
		}

		events, next, err := d.FetchMiningEvents(start, end)
		if err != nil {
			log.Warn("FetchMiningEvents fail", "start", start, "end", end, "err", err.Error())
			return
		}

		for _, event := range events {
			d.m.SetMiningInfo(d.contract, event.ShardId, event.Difficulty.Uint64(), event.LastMineTime,
				event.BlockMined.Uint64(), event.Miner, event.GasFee.Uint64(), event.Reward.Uint64())
			d.logger.Info("Refresh mining info", "contract", d.contract, "txHash", event.TxHash.Hex(),
				"blockMined", event.BlockMined, "lastMineTime", event.LastMineTime, "difficulty", event.Difficulty,
				"miner", event.Miner, "gasFee", event.GasFee, "reward", event.Reward)
		}
		d.startBlock = next
	}
	d.db.Put([]byte(fmt.Sprintf("%s-%s", dbKey_Prefix_LastBlock, d.contract.Hex())), new(big.Int).SetUint64(d.startBlock).Bytes())
}

func (d *dashboard) FetchMiningEvents(start, end uint64) ([]*miningEvent, uint64, error) {
	logs, err := d.source.FilterLogsByBlockRange(new(big.Int).SetUint64(start), new(big.Int).SetUint64(end), eth.MinedBlockEvent)
	if err != nil {
		return nil, start, fmt.Errorf("FilterLogsByBlockRange: %s", err.Error())
	}

	events := make([]*miningEvent, 0)
	for _, l := range logs {
		var event *miningEvent
		if d.sourceType == l1Type {
			event, err = d.GetL1TransactionReceiptByHash(l)
		} else if d.sourceType == l2Type {
			event, err = d.GetL2TransactionReceiptByHash(l)
		}
		if err != nil {
			return nil, start, fmt.Errorf("GetTransactionByHash fail, tx hash: %s, error: %s", l.TxHash.Hex(), err.Error())
		}

		events = append(events, event)
	}
	return events, end + 1, nil
}

func (d *dashboard) GetL1TransactionReceiptByHash(l types.Log) (*miningEvent, error) {
	receipt, err := d.source.TransactionReceipt(context.Background(), l.TxHash)
	if err != nil {
		return nil, err
	}
	if receipt.Status == types.ReceiptStatusFailed {
		return nil, fmt.Errorf("tx successfully published but reverted")
	}

	return &miningEvent{
		TxHash:       l.TxHash,
		ShardId:      new(big.Int).SetBytes(l.Topics[1].Bytes()).Uint64(),
		Difficulty:   new(big.Int).SetBytes(l.Topics[2].Bytes()),
		BlockMined:   new(big.Int).SetBytes(l.Topics[3].Bytes()),
		LastMineTime: new(big.Int).SetBytes(l.Data[:32]).Uint64(),
		Miner:        common.BytesToAddress(l.Data[44:64]),
		Reward:       new(big.Int).Div(new(big.Int).SetBytes(l.Data[64:96]), divisor),
		GasFee:       new(big.Int).Div(new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice), divisor),
	}, nil
}

func (d *dashboard) GetL2TransactionReceiptByHash(l types.Log) (*miningEvent, error) {
	var r *L2Receipt
	err := d.source.Client.Client().CallContext(context.Background(), &r, "eth_getTransactionReceipt", l.TxHash)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ethereum.NotFound
	}

	if r.Status == types.ReceiptStatusFailed {
		return nil, fmt.Errorf("tx successfully published but reverted")
	}

	return &miningEvent{
		TxHash:       l.TxHash,
		ShardId:      new(big.Int).SetBytes(l.Topics[1].Bytes()).Uint64(),
		Difficulty:   new(big.Int).SetBytes(l.Topics[2].Bytes()),
		BlockMined:   new(big.Int).SetBytes(l.Topics[3].Bytes()),
		LastMineTime: new(big.Int).SetBytes(l.Data[:32]).Uint64(),
		Miner:        common.BytesToAddress(l.Data[44:64]),
		Reward:       new(big.Int).Div(new(big.Int).SetBytes(l.Data[64:96]), divisor),
		GasFee: new(big.Int).Div(new(big.Int).Add(
			new(big.Int).Mul(new(big.Int).SetUint64(r.GasUsed), r.EffectiveGasPrice), r.L1Fee), divisor),
	}, nil
}

func (d *dashboard) InitMetrics() error {
	lastMineTimeVal, err := d.source.ReadContractField("prepaidLastMineTime", new(big.Int).SetUint64(d.startBlock))
	if err != nil {
		return err
	}
	minDiffVal, err := d.source.ReadContractField("minimumDiff", new(big.Int).SetUint64(d.startBlock))
	if err != nil {
		return err
	}
	d.m.SetMiningInfo(d.contract, 0, new(big.Int).SetBytes(minDiffVal).Uint64(),
		new(big.Int).SetBytes(lastMineTimeVal).Uint64(), 0, common.Address{}, 0, 0)
	return nil
}

func LoadConfig(ruleFile string) []*Param {
	file, err := os.Open(ruleFile)
	if err != nil {
		log.Crit("Failed to load rule file", "ruleFile", ruleFile, "err", err)
	}
	defer file.Close()

	var params []*Param
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&params); err != nil {
		log.Crit("Failed to decode rule file", "ruleFile", ruleFile, "err", err)
	}

	return params
}

func main() {
	// Parse the flags and set up the logger to print everything requested
	flag.Parse()
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(*logFlag), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	if *portFlag < 0 || *portFlag > math.MaxUint16 {
		log.Crit("Invalid port")
	}

	m := metrics.NewMetrics("dashboard")
	params := LoadConfig(*configFileFlag)
	db, err := rawdb.Open(rawdb.OpenOptions{
		Type:              "leveldb",
		Directory:         *dataPath,
		AncientsDirectory: filepath.Join(*dataPath, "ancient"),
		Namespace:         "es-data/db/dashboard/",
		Cache:             2048,
		Handles:           8196,
		ReadOnly:          false,
	})
	if err != nil {
		log.Crit("Failed to create db", "err", err)
	}

	for _, param := range params {
		d, err := newDashboard(param, db, m)
		if err != nil {
			log.Crit("New dashboard fail", "err", err)
		}
		err = d.InitMetrics()
		if err != nil {
			log.Crit("Init metrics value fail", "err", err.Error())
		}

		d.RefreshMiningMetrics()
		l1LatestBlockSub := eth.PollBlockChanges(d.ctx, d.logger, d.source, d.RefreshMetrics, ethRPC.LatestBlockNumber, epoch, epoch)
		defer l1LatestBlockSub.Unsubscribe()
	}

	if err := m.Serve(context.Background(), *listenAddrFlag, *portFlag); err != nil {
		log.Crit("Error starting metrics server", "err", err)
	}
}
