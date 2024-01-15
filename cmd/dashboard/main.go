// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"context"
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
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	ethRPC "github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
)

const (
	dbKey = "FetchStatus"
	step  = 300
	epoch = 12 * time.Second
)

var (
	listenAddrFlag  = flag.String("address", "0.0.0.0", "Listener address")
	portFlag        = flag.Int("port", 8300, "Listener port for the devp2p connection")
	rpcURLFlag      = flag.String("rpcurl", "http://65.109.115.36:8545", "L1 RPC URL")
	l1ContractFlag  = flag.String("l1contract", "", "Storage contract address on l1")
	startNumberFlag = flag.Uint64("startnumber", 1, "The block number start to filter contract event")
	dataPath        = flag.String("datadir", "./es-data", "Data directory for the databases")
	logFlag         = flag.Int("loglevel", 3, "Log level to use for Ethereum and the faucet")
)

type miningEvent struct {
	ShardId      uint64
	LastMineTime uint64
	Difficulty   *big.Int
	BlockMined   *big.Int
	Miner        common.Address
	GasFee       uint64
	Reward       uint64
}

type dashboard struct {
	ctx        context.Context
	l1Source   *eth.PollingClient
	m          metrics.Metricer
	l1Contract common.Address
	kvEntries  uint64

	maxShardIdx uint64
	startBlock  uint64
	endBlock    uint64
	db          ethdb.Database
	logger      log.Logger
}

func newDashboard(rpcURL string, l1Contract common.Address) (*dashboard, error) {
	var (
		m      = metrics.NewMetrics("dashboard")
		logger = log.New("app", "Dashboard")
		ctx    = context.Background()
	)

	l1, err := eth.Dial(rpcURL, l1Contract, logger)
	if err != nil {
		log.Crit("Failed to create L1 source", "err", err)
	}

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

	start := *startNumberFlag
	if status, _ := db.Get([]byte(dbKey)); status != nil {
		start = new(big.Int).SetBytes(status).Uint64()
	}

	if start == 0 {
		block, err := l1.BlockByNumber(ctx, new(big.Int).SetInt64(ethRPC.LatestBlockNumber.Int64()))
		if err != nil {
			log.Crit("Failed to fetch start block", "err", err)
		}
		start = block.NumberU64()
		if start == 0 {
			log.Crit("Start block should not be 0")
		}
	}

	shardEntryBits, err := readUintFromContract(ctx, l1.Client, l1Contract, "shardEntryBits")
	if err != nil {
		return nil, err
	}

	return &dashboard{
		ctx:        ctx,
		l1Source:   l1,
		m:          m,
		l1Contract: l1Contract,
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
	lastKVIndex, err := d.l1Source.GetStorageLastBlobIdx(int64(sig.Number))
	if err != nil {
		log.Warn("Refresh contract metrics (last kv index) failed", "err", err.Error())
		return
	}
	maxShardIdx := lastKVIndex / d.kvEntries
	d.m.SetLastKVIndexAndMaxShardId(sig.Number, lastKVIndex, maxShardIdx)
	d.logger.Info("RefreshBlobMetrics", "block number", sig.Number, "lastKvIndex", lastKVIndex, "maxShardIdx", maxShardIdx)
	d.maxShardIdx = maxShardIdx
	if sig.Number > d.endBlock {
		d.endBlock = sig.Number
	}
}

func (d *dashboard) RefreshMiningMetrics() {
	if d.startBlock > d.endBlock {
		return
	}

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
		d.m.SetMiningInfo(event.ShardId, event.Difficulty.Uint64(), event.LastMineTime, event.BlockMined.Uint64(), event.Miner, event.GasFee, event.Reward)
		d.logger.Info("Refresh mining info", "blockMined", event.BlockMined, "lastMineTime", event.LastMineTime,
			"Difficulty", event.Difficulty, "Miner", event.Miner, "GasFee", event.GasFee, "Reward", event.Reward)
	}
	d.startBlock = next
	d.db.Put([]byte(dbKey), new(big.Int).SetUint64(d.startBlock).Bytes())
}

func (d *dashboard) FetchMiningEvents(start, end uint64) ([]*miningEvent, uint64, error) {
	logs, err := d.l1Source.FilterLogsByBlockRange(new(big.Int).SetUint64(start), new(big.Int).SetUint64(end), eth.MinedBlockEvent)
	if err != nil {
		return nil, start, fmt.Errorf("FilterLogsByBlockRange: %s", err.Error())
	}

	events := make([]*miningEvent, 0)
	for _, l := range logs {
		tx, err := d.GetTransactionByHash(l.TxHash)
		if err != nil {
			return nil, start, fmt.Errorf("GetTransactionByHash fail, tx hash: %s, error: %s", l.TxHash.Hex(), err.Error())
		}

		events = append(events, &miningEvent{
			ShardId:      new(big.Int).SetBytes(l.Topics[1].Bytes()).Uint64(),
			Difficulty:   new(big.Int).SetBytes(l.Topics[2].Bytes()),
			BlockMined:   new(big.Int).SetBytes(l.Topics[3].Bytes()),
			LastMineTime: new(big.Int).SetBytes(l.Data[:32]).Uint64(),
			Miner:        common.BytesToAddress(l.Data[:32][44:64]),
			Reward:       new(big.Int).SetBytes(l.Data[64:96]).Uint64() / 10000000000,
			GasFee:       tx.Gas() * tx.GasPrice().Uint64() / 10000000000,
		})
	}
	return events, end + 1, nil
}

func (d *dashboard) GetTransactionByHash(hash common.Hash) (*types.Transaction, error) {
	tx, _, err := d.l1Source.TransactionByHash(context.Background(), hash)
	if err != nil {
		return nil, err
	}
	if len(tx.Data()) < 100 {
		return nil, fmt.Errorf("invalid data len for tx %d", len(tx.Data()))
	}
	return tx, nil
}

func (d *dashboard) InitMetrics() error {
	lastMineTimeVal, err := d.l1Source.ReadContractField("prepaidLastMineTime", new(big.Int).SetUint64(d.startBlock))
	if err != nil {
		return err
	}
	minDiffVal, err := d.l1Source.ReadContractField("minimumDiff", new(big.Int).SetUint64(d.startBlock))
	if err != nil {
		return err
	}
	d.m.SetMiningInfo(0, new(big.Int).SetBytes(minDiffVal).Uint64(), new(big.Int).SetBytes(lastMineTimeVal).Uint64(),
		0, common.Address{}, 0, 0)
	return nil
}

func readSlotFromContract(ctx context.Context, client *ethclient.Client, l1Contract common.Address, fieldName string) ([]byte, error) {
	h := crypto.Keccak256Hash([]byte(fieldName + "()"))
	msg := ethereum.CallMsg{
		To:   &l1Contract,
		Data: h[0:4],
	}
	bs, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s from contract: %v", fieldName, err)
	}
	return bs, nil
}

func readUintFromContract(ctx context.Context, client *ethclient.Client, l1Contract common.Address, fieldName string) (uint64, error) {
	bs, err := readSlotFromContract(ctx, client, l1Contract, fieldName)
	if err != nil {
		return 0, err
	}
	value := new(big.Int).SetBytes(bs).Uint64()
	log.Info("Read uint from contract", "field", fieldName, "value", value)
	return value, nil
}

func main() {
	// Parse the flags and set up the logger to print everything requested
	flag.Parse()
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(*logFlag), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	if *portFlag < 0 || *portFlag > math.MaxUint16 {
		log.Crit("Invalid port")
	}
	d, err := newDashboard(*rpcURLFlag, common.HexToAddress(*l1ContractFlag))
	if err != nil {
		log.Crit("New dashboard fail", "err", err)
	}
	err = d.InitMetrics()
	if err != nil {
		log.Crit("Init metrics value fail", "err", err.Error())
	}
	l1LatestBlockSub := eth.PollBlockChanges(d.ctx, d.logger, d.l1Source, d.RefreshMetrics, ethRPC.LatestBlockNumber, epoch, epoch)
	defer l1LatestBlockSub.Unsubscribe()

	if err := d.m.Serve(d.ctx, *listenAddrFlag, *portFlag); err != nil {
		log.Crit("Error starting metrics server", "err", err)
	}
}
