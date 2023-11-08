package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	ethRPC "github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
)

var (
	listenAddrFlag = flag.String("address", "0.0.0.0", "Listener address")
	portFlag       = flag.Int("port", 8300, "Listener port for the devp2p connection")
	rpcURLFlag     = flag.String("rpcurl", "http://65.108.236.27:8545", "L1 RPC URL")
	l1ContractFlag = flag.String("l1contract", "", "Storage contract address on l1")
	logFlag        = flag.Int("loglevel", 3, "Log level to use for Ethereum and the faucet")
)

type dashboard struct {
	ctx        context.Context
	l1Source   *eth.PollingClient
	m          metrics.Metricer
	l1Contract common.Address
	kvEntries  uint64
	logger     log.Logger
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
		logger:     logger,
	}, nil
}

func (d *dashboard) RefreshContractMetrics(ctx context.Context, sig eth.L1BlockRef) {
	lastKVIndex, err := d.l1Source.GetStorageLastBlobIdx(int64(sig.Number))
	if err != nil {
		log.Warn("Refresh contract metrics (last kv index) failed", "err", err.Error())
		return
	}
	maxShardIdx := lastKVIndex / d.kvEntries
	d.m.SetLastKVIndexAndMaxShardId(sig.Number, lastKVIndex, maxShardIdx)
	d.logger.Info("RefreshContractMetrics", "lastKvIndex", lastKVIndex, "maxShardIdx", maxShardIdx)

	l1api := miner.NewL1MiningAPI(d.l1Source, d.logger)
	for sid := uint64(0); sid <= maxShardIdx; sid++ {
		info, err := l1api.GetMiningInfo(ctx, d.l1Contract, sid)
		if err != nil {
			log.Warn("Get mining info for metrics fail", "shardId", sid, "err", err.Error())
			continue
		}
		d.m.SetMiningInfo(sid, info.Difficulty.Uint64(), info.LastMineTime, info.BlockMined.Uint64())
		d.logger.Info("Refresh mining info", "blockMined", info.BlockMined, "lastMineTime", info.LastMineTime, "Difficulty", info.Difficulty)
	}
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
	l1LatestBlockSub := eth.PollBlockChanges(d.ctx, d.logger, d.l1Source, d.RefreshContractMetrics, ethRPC.LatestBlockNumber,
		10*time.Second, time.Second*10)
	defer l1LatestBlockSub.Unsubscribe()

	if err := d.m.Serve(d.ctx, *listenAddrFlag, *portFlag); err != nil {
		log.Crit("Error starting metrics server", "err", err)
	}
}
