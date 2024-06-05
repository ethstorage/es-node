// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"context"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
)

type L1API interface {
	TransactionByHash(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error)
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	GetMiningInfo(ctx context.Context, contract common.Address, shardIdx uint64) (*miningInfo, error)
	GetDataHashes(ctx context.Context, contract common.Address, kvIdxes []uint64) ([]common.Hash, error)
	GetMiningReward(shard uint64, blockNumber int64) (*big.Int, error)
	ComposeCalldata(ctx context.Context, rst result) ([]byte, error)
	SuggestGasPrices(ctx context.Context, cfg Config) (*big.Int, *big.Int, *big.Int, error)
	BlockNumber(ctx context.Context) (uint64, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
	L1RPCURL() string
}

type MiningProver interface {
	GetStorageProof(encodedKVs [][]byte, encodingKey []common.Hash, sampleIdxInKv []uint64) ([]*big.Int, [][]byte, [][]byte, error)
}

type miningInfo struct {
	LastMineTime uint64
	Difficulty   *big.Int
	BlockMined   *big.Int
}

func (a *miningInfo) String() string {
	return fmt.Sprintf(
		"LastMineTime: %d, Difficulty: %s, BlockMined: %s",
		a.LastMineTime,
		a.Difficulty.String(),
		a.BlockMined.String(),
	)
}

// Miner creates blocks and searches for proof-of-work values.
type Miner struct {
	feed        *event.Feed
	worker      *worker
	exitCh      chan struct{}
	startCh     chan struct{}
	stopCh      chan struct{}
	ChainHeadCh chan eth.L1BlockRef
	wg          sync.WaitGroup
	lg          log.Logger
}

func New(config *Config, db ethdb.Database, storageMgr *ethstorage.StorageManager, api L1API, prover MiningProver, feed *event.Feed, lg log.Logger) *Miner {
	chainHeadCh := make(chan eth.L1BlockRef, chainHeadChanSize)
	miner := &Miner{
		feed:        feed,
		ChainHeadCh: chainHeadCh,
		exitCh:      make(chan struct{}),
		startCh:     make(chan struct{}),
		stopCh:      make(chan struct{}),
		lg:          lg,
		worker:      newWorker(*config, db, storageMgr, api, chainHeadCh, prover, lg),
	}
	miner.wg.Add(1)
	go miner.update()
	return miner
}

// update keeps track of the downloader events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
func (miner *Miner) update() {
	// Subscribe es SyncDone event
	syncEventCh := make(chan protocol.EthStorageSyncDone)
	sub := miner.feed.Subscribe(syncEventCh)
	defer func() {
		sub.Unsubscribe()
		close(syncEventCh)
		miner.wg.Done()
	}()

	shouldStart := false
	canStart := false

	for {
		miner.lg.Debug("Miner update loop", "shouldStart", shouldStart, "canStart", canStart)
		select {
		case syncDone := <-syncEventCh:
			if syncDone.DoneType == protocol.SingleShardDone {
				miner.worker.startCh <- syncDone.ShardId
				miner.lg.Info("Miner update loop", "shardIsReady", syncDone.ShardId)
				canStart = true
				if shouldStart && !miner.worker.isRunning() {
					miner.worker.start()
				}
			} else {
				sub.Unsubscribe()
			}
		case <-miner.startCh:
			if canStart {
				miner.worker.start()
			}
			shouldStart = true
		case <-miner.stopCh:
			shouldStart = false
			miner.worker.stop()
		case <-miner.exitCh:
			miner.worker.close()
			return
		}
	}
}

// miner must be started before p2p sync so that it can receive the SyncDone event
func (miner *Miner) Start() {
	miner.startCh <- struct{}{}
}

func (miner *Miner) Stop() {
	miner.lg.Warn("Miner is being stopped...")
	miner.stopCh <- struct{}{}
}

func (miner *Miner) Close() {
	miner.Stop()
	miner.lg.Warn("Miner is being closed...")
	close(miner.exitCh)
	miner.wg.Wait()
}

func (miner *Miner) Mining() bool {
	return miner.worker.isRunning()
}
