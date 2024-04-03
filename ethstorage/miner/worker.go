// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
)

const (
	chainHeadChanSize = 1
	taskQueueSize     = 1
	resultQueueSize   = 10
	sampleSizeBits    = 5 // 32 bytes
	// always use new block hash to mine for each slot
	mineTimeOut              = 12 // seconds
	miningTransactionTimeout = 25 // seconds
)

var (
	minedEventSig = crypto.Keccak256Hash([]byte("MinedBlock(uint256,uint256,uint256,uint256,address,uint256)"))
	errCh         = make(chan miningError, 10)
	errDropped    = errors.New("dropped: not enough profit")
)

type task struct {
	miner    common.Address
	shardIdx uint64
	taskChs  []chan *taskItem
}

type taskItem struct {
	*task
	requiredDiff *big.Int
	blockNumber  *big.Int
	mixHash      common.Hash
	nonceStart   uint64
	nonceEnd     uint64
	mineTime     uint64
	thread       uint64
}

func (t *taskItem) String() string {
	return fmt.Sprintf("shard: %d, thread: %d, block: %v", t.shardIdx, t.thread, t.blockNumber)
}

type miningError struct {
	shardIdx uint64
	block    *big.Int
	err      error
}

func (e miningError) String() string {
	return fmt.Sprintf("shard %d: block %v: %s", e.shardIdx, e.block, e.err.Error())
}

type result struct {
	blockNumber     *big.Int
	startShardId    uint64
	miner           common.Address
	nonce           uint64
	encodedData     []common.Hash
	masks           []*big.Int
	inclusiveProofs [][]byte
	decodeProof     [][]byte
}

// worker is the main object which takes care of storage mining
// and submit the mining result tx to the L1 chain.
type worker struct {
	config     Config
	l1API      L1API
	prover     MiningProver
	storageMgr *es.StorageManager

	chainHeadCh chan eth.L1BlockRef
	startCh     chan uint64
	exitCh      chan struct{}

	shardTaskMap map[uint64]task

	resultCh   chan struct{}
	resultLock sync.Mutex
	resultMap  map[uint64]*result // protected by resultLock

	running int32
	wg      sync.WaitGroup
	lg      log.Logger
}

func newWorker(
	config Config,
	storageMgr *es.StorageManager,
	api L1API,
	chainHeadCh chan eth.L1BlockRef,
	prover MiningProver,
	lg log.Logger,
) *worker {
	worker := &worker{
		config:       config,
		l1API:        api,
		prover:       prover,
		chainHeadCh:  chainHeadCh,
		shardTaskMap: make(map[uint64]task),
		exitCh:       make(chan struct{}),
		startCh:      make(chan uint64, 1),
		resultCh:     make(chan struct{}, 1),
		resultLock:   sync.Mutex{},
		resultMap:    make(map[uint64]*result),
		storageMgr:   storageMgr,
		lg:           lg,
	}
	worker.wg.Add(2)
	go worker.newWorkLoop()
	go worker.resultLoop()
	return worker
}

func (w *worker) start() {
	w.lg.Info("Worker is being started...")
	atomic.StoreInt32(&w.running, 1)
}

func (w *worker) stop() {
	w.lg.Warn("Worker is being stopped...")
	atomic.StoreInt32(&w.running, 0)
}

func (w *worker) isRunning() bool {
	return atomic.LoadInt32(&w.running) == 1
}

func (w *worker) close() {
	w.stop()
	w.lg.Warn("Worker is being closed...")
	close(w.exitCh)
	w.wg.Wait()
	for _, task := range w.shardTaskMap {
		for _, ch := range task.taskChs {
			close(ch)
		}
	}
}

// newWorkLoop is a standalone goroutine to do the following upon received events:
// 1) start new task loop
// 2) submit new mining work
func (w *worker) newWorkLoop() {
	defer w.wg.Done()

	for {
		select {
		case shardIdx := <-w.startCh:
			miner, _ := w.storageMgr.GetShardMiner(shardIdx)
			var taskChs []chan *taskItem
			for i := uint64(0); i < w.config.ThreadsPerShard; i++ {
				taskCh := make(chan *taskItem, taskQueueSize)
				taskChs = append(taskChs, taskCh)
				w.wg.Add(1)
				w.lg.Debug("Worker is starting task loop", "shard", shardIdx, "thread", i)
				go w.taskLoop(taskCh)
			}
			w.lg.Info("Worker is starting task loops", "shard", shardIdx, "threads", w.config.ThreadsPerShard)
			task := task{
				miner:    miner,
				shardIdx: shardIdx,
				taskChs:  taskChs,
			}
			w.shardTaskMap[shardIdx] = task
		case block := <-w.chainHeadCh:
			if !w.isRunning() {
				break
			}
			w.lg.Info("Updating tasks with L1 new head", "blockNumber", block.Number, "blockTime", block.Time, "now", uint64(time.Now().Unix()))
			// TODO suspend mining if:
			// 1) a mining tx is already submitted; or
			// 2) if the last mining time is too close (the reward is not enough).
			for shardIdx, task := range w.shardTaskMap {
				reqDiff, err := w.updateDifficulty(shardIdx, block.Time)
				if err != nil {
					continue
				}
				w.assignTasks(task, block, reqDiff)
			}
		case <-w.exitCh:
			w.lg.Warn("Worker is exiting from work loop...")
			return
		}
	}
}

// assign tasks to threads with split nonce range
func (w *worker) assignTasks(task task, block eth.L1BlockRef, reqDiff *big.Int) {
	seg := w.config.NonceLimit / w.config.ThreadsPerShard
	for i := uint64(0); i < w.config.ThreadsPerShard; i++ {
		var ne uint64
		if i == w.config.ThreadsPerShard-1 {
			ne = w.config.NonceLimit
		} else {
			ne = seg * (i + 1)
		}
		ti := &taskItem{
			task:         &task,
			requiredDiff: reqDiff,
			nonceStart:   seg * i,
			nonceEnd:     ne,
			blockNumber:  new(big.Int).SetUint64(block.Number),
			mixHash:      block.MixDigest,
			mineTime:     block.Time,
			thread:       i,
		}
		ch := task.taskChs[i]
		select {
		case ch <- ti:
			w.lg.Debug("Mining task queued", "shard", ti.shardIdx, "thread", ti.thread, "block", ti.blockNumber, "blockTime", block.Time, "now", uint64(time.Now().Unix()))
		case <-w.exitCh:
			w.lg.Warn("Worker is exiting from thread loop...")
			return
		default:
			// try to remove the item in the queue to make sure the task is executed with the latest block number
			select {
			case old := <-ch:
				w.lg.Debug("Old mining task removed", "shard", ti.shardIdx, "thread", ti.thread, "blockOld", old.blockNumber)
			default:
			}

			// note: it is SPSC so we don't need to be worry about the blocking issue here.
			ch <- ti
			w.lg.Debug("Mining task queued", "shard", ti.shardIdx, "thread", ti.thread, "block", ti.blockNumber, "blockTime", block.Time, "now", uint64(time.Now().Unix()))
		}
	}
	w.lg.Info("Mining tasks assigned", "miner", task.miner, "shard", task.shardIdx, "threads", w.config.ThreadsPerShard, "block", block.Number, "nonces", w.config.NonceLimit)
}

func (w *worker) updateDifficulty(shardIdx, blockTime uint64) (*big.Int, error) {
	info, err := w.l1API.GetMiningInfo(
		context.Background(),
		w.storageMgr.ContractAddress(),
		shardIdx,
	)
	if err != nil {
		w.lg.Warn("Failed to get es mining info", "error", err.Error())
		return nil, err
	}
	w.lg.Info("Mining info retrieved", "shard", shardIdx, "LastMineTime", info.LastMineTime, "Difficulty", info.Difficulty, "proofsSubmitted", info.BlockMined)
	reqDiff := new(big.Int).Div(maxUint256, expectedDiff(
		info.LastMineTime,
		blockTime,
		info.Difficulty,
		w.config.Cutoff,
		w.config.DiffAdjDivisor,
		w.config.MinimumDiff,
	))
	return reqDiff, nil
}

// taskLoop is a standalone goroutine to fetch mining task from the task channel and mine the task.
func (w *worker) taskLoop(taskCh chan *taskItem) {
	defer w.wg.Done()
	for {
		select {
		case ti := <-taskCh:
			success, err := w.mineTask(ti)
			if err != nil {
				select {
				case errCh <- miningError{ti.shardIdx, ti.blockNumber, err}:
				default:
					w.lg.Warn("Sent miningError to errCh failed", "lenOfCh", len(errCh))
				}
				w.lg.Warn("Mine task fail", "shard", ti.shardIdx, "thread", ti.thread, "block", ti.blockNumber, "err", err.Error())
			}
			if success {
				w.lg.Info("Mine task success", "shard", ti.shardIdx, "thread", ti.thread, "block", ti.blockNumber)
			}
		case <-w.exitCh:
			w.lg.Warn("Worker is exiting from task loop...")
			return
		}
	}
}

func (w *worker) getResult() *result {
	w.resultLock.Lock()
	defer w.resultLock.Unlock()

	for k := range w.resultMap {
		if w.resultMap[k] != nil {
			r := w.resultMap[k]
			w.resultMap[k] = nil
			return r
		}
	}
	return nil
}

func (w *worker) notifyResultLoop() {
	select {
	case w.resultCh <- struct{}{}:
	default:
	}
}

// resultLoop is a standalone goroutine to submit mining result to L1 contract.
func (w *worker) resultLoop() {
	defer w.wg.Done()
	var startTime = time.Now().Format("2006-01-02 15:04:05")
	var succeeded, dropped int
	errorCache := make([]miningError, 0)
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-w.resultCh:
			result := w.getResult()
			if result == nil {
				continue
			}
			w.lg.Info("Mining result loop get result", "shard", result.startShardId, "block", result.blockNumber, "nonce", result.nonce)
			txHash, err := w.l1API.SubmitMinedResult(
				context.Background(),
				w.storageMgr.ContractAddress(),
				*result,
				w.config,
			)
			if err != nil {
				if err == errDropped {
					dropped++
				} else {
					errorCache = append(errorCache, miningError{result.startShardId, result.blockNumber, err})
					w.lg.Error("Failed to submit mined result", "shard", result.startShardId, "block", result.blockNumber, "error", err.Error())
				}
			} else {
				succeeded++
			}
			if txHash != (common.Hash{}) {
				// waiting for tx confirmation or timeout
				ticker := time.NewTicker(1 * time.Second)
				checked := 0
				for range ticker.C {
					if checked > miningTransactionTimeout {
						log.Warn("Waiting for mining transaction confirm timed out", "txHash", txHash)
						break
					}

					checked++
					_, isPending, err := w.l1API.TransactionByHash(context.Background(), txHash)
					if err != nil {
						log.Error("Querying transaction by hash failed", "error", err, "txHash", txHash)
						continue
					} else if !isPending {
						log.Info("Mining transaction confirmed", "txHash", txHash)
						w.checkTxStatus(txHash, result.miner)
						break
					}
				}
				ticker.Stop()
			}
			// optimistically check next result if exists
			w.notifyResultLoop()
		case <-ticker.C:
			if len(errorCache) > 0 {
				log.Error(fmt.Sprintf("Mining stats since %s", startTime),
					"succeeded", succeeded,
					"failed", len(errorCache),
					"dropped", dropped,
					"lastError", errorCache[len(errorCache)-1],
				)
			} else {
				log.Info(fmt.Sprintf("Mining stats since %s", startTime),
					"succeeded", succeeded,
					"failed", len(errorCache),
					"dropped", dropped,
				)
			}
		case err := <-errCh:
			errorCache = append(errorCache, err)
		case <-w.exitCh:
			w.lg.Warn("Worker is exiting from result loop...")
			return
		}
	}
}

func (w *worker) checkTxStatus(txHash common.Hash, miner common.Address) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	receipt, err := w.l1API.TransactionReceipt(ctx, txHash)
	if err != nil || receipt == nil {
		log.Warn("Mining transaction not found!", "err", err, "txHash", txHash)
	} else if receipt.Status == 1 {
		log.Info("Mining transaction success!      √", "miner", miner)
		log.Info("Mining transaction details", "txHash", txHash, "gasUsed", receipt.GasUsed, "effectiveGasPrice", receipt.EffectiveGasPrice)
		cost := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
		var reward *big.Int
		for _, rLog := range receipt.Logs {
			if rLog.Topics[0] == minedEventSig {
				// the last param of total unindexed 3
				reward = new(big.Int).SetBytes(rLog.Data[64:])
				break
			}
		}
		if reward != nil {
			log.Info("Mining transaction accounting (in ether)",
				"reward", weiToEther(reward),
				"cost", weiToEther(cost),
				"profit", weiToEther(new(big.Int).Sub(reward, cost)),
			)
		}
	} else if receipt.Status == 0 {
		log.Warn("Mining transaction failed!      ×", "txHash", txHash)
	}
}

// https://github.com/ethereum/go-ethereum/issues/21221#issuecomment-805852059
func weiToEther(wei *big.Int) *big.Float {
	f := new(big.Float)
	f.SetPrec(236) //  IEEE 754 octuple-precision binary floating-point format: binary256
	f.SetMode(big.ToNearestEven)
	if wei == nil {
		return f.SetInt64(0)
	}
	fWei := new(big.Float)
	fWei.SetPrec(236) //  IEEE 754 octuple-precision binary floating-point format: binary256
	fWei.SetMode(big.ToNearestEven)
	return f.Quo(fWei.SetInt(wei), big.NewFloat(params.Ether))
}

// mineTask actually executes a mining task
func (w *worker) mineTask(t *taskItem) (bool, error) {
	startTime := time.Now()
	nonce := t.nonceStart
	w.lg.Debug("Mining task started", "shard", t.shardIdx, "thread", t.thread, "block", t.blockNumber, "nonces", fmt.Sprintf("%d~%d", t.nonceStart, t.nonceEnd))
	for w.isRunning() {
		if time.Since(startTime).Seconds() > mineTimeOut {
			if t.thread == 0 {
				nonceTriedTotal := (nonce - t.nonceStart) * w.config.ThreadsPerShard
				w.lg.Warn("Mining tasks timed out", "shard", t.shardIdx, "block", t.blockNumber,
					"noncesTried", fmt.Sprintf("%d(%.1f%%)", nonceTriedTotal, float64(nonceTriedTotal*100)/float64(w.config.NonceLimit)),
				)
			}
			w.lg.Debug("Mining task timed out", "shard", t.shardIdx, "thread", t.thread, "block", t.blockNumber, "noncesTried", nonce-t.nonceStart)
			break
		}
		if nonce >= t.nonceEnd {
			samplingTime := fmt.Sprintf("%.1fs", time.Since(startTime).Seconds())
			if t.thread == 0 {
				w.lg.Info("Sampling done with all nonces",
					"samplingTime", samplingTime, "shard", t.shardIdx, "block", t.blockNumber)
			}
			w.lg.Debug("Sampling done with all nonces",
				"samplingTime", samplingTime, "shard", t.shardIdx, "block", t.blockNumber, "thread", t.thread, "nonceEnd", nonce)
			break
		}
		hash0 := initHash(t.miner, t.mixHash, nonce)
		hash1, sampleIdxs, err := w.computeHash(t.task.shardIdx, hash0)
		if err != nil {
			w.lg.Error("Calculate hash error", "shard", t.shardIdx, "thread", t.thread, "block", t.blockNumber, "err", err.Error())
			return false, err
		}
		if t.requiredDiff.Cmp(new(big.Int).SetBytes(hash1.Bytes())) >= 0 {
			w.lg.Info("Calculated a valid hash", "shard", t.shardIdx, "thread", t.thread, "block", t.blockNumber, "nonce", nonce)
			dataSet, kvIdxs, sampleIdxsInKv, encodingKeys, encodedSamples, err := w.getMiningData(t.task, sampleIdxs)
			if err != nil {
				w.lg.Error("Get sample data failed", "kvIdxs", kvIdxs, "sampleIdxsInKv", sampleIdxsInKv, "err", err.Error())
				return false, err
			}
			w.lg.Info("Got sample data", "shard", t.shardIdx, "thread", t.thread, "block", t.blockNumber, "kvIdxs", kvIdxs, "sampleIdxsInKv", sampleIdxsInKv)
			masks, decodeProof, inclusiveProofs, err := w.prover.GetStorageProof(dataSet, encodingKeys, sampleIdxsInKv)
			if err != nil {
				w.lg.Error("Get storage proof error", "kvIdx", kvIdxs, "sampleIdxsInKv", sampleIdxsInKv, "error", err.Error())
				return false, fmt.Errorf("get proof err: %v", err)
			}
			w.lg.Info("Got storage proof", "shard", t.shardIdx, "thread", t.thread, "block", t.blockNumber, "kvIdx", kvIdxs, "sampleIdxsInKv", sampleIdxsInKv)
			newResult := &result{
				blockNumber:     t.blockNumber,
				startShardId:    t.shardIdx,
				miner:           t.miner,
				nonce:           nonce,
				encodedData:     encodedSamples,
				masks:           masks,
				decodeProof:     decodeProof,
				inclusiveProofs: inclusiveProofs,
			}
			// push result to the result map
			w.resultLock.Lock()
			// override the existing result if not nil
			w.resultMap[t.shardIdx] = newResult
			w.resultLock.Unlock()
			w.lg.Info("Set mining result", "shard", t.shardIdx, "block", t.blockNumber, "nonce", nonce)

			// notify the result worker to wake up
			w.notifyResultLoop()
			return true, nil
		}
		nonce++
	}

	return false, nil
}

// computeHash calculates final hash from hash0
func (w *worker) computeHash(shardIdx uint64, hash0 common.Hash) (common.Hash, []uint64, error) {
	return hashimoto(w.storageMgr.KvEntriesBits(),
		w.storageMgr.MaxKvSizeBits(), sampleSizeBits,
		shardIdx,
		w.config.RandomChecks,
		w.storageMgr.ReadSampleUnlocked,
		hash0,
	)
}

// getMiningData retrieves data needed to generte proof and verify against the contract.
func (w *worker) getMiningData(t *task, sampleIdx []uint64) ([][]byte, []uint64, []uint64, []common.Hash, []common.Hash, error) {
	checksLen := w.config.RandomChecks
	dataSet := make([][]byte, checksLen)
	kvIdxs, sampleIdxsInKv := make([]uint64, checksLen), make([]uint64, checksLen)
	encodingKeys, encodedSamples := make([]common.Hash, checksLen), make([]common.Hash, checksLen)
	sampleLenBits := w.storageMgr.MaxKvSizeBits() - sampleSizeBits
	for i := uint64(0); i < checksLen; i++ {
		kvIdxs[i] = sampleIdx[i] >> sampleLenBits
	}
	kvHashes, err := w.l1API.GetDataHashes(context.Background(), w.storageMgr.ContractAddress(), kvIdxs)
	if err != nil {
		w.lg.Error("Get data hashes error", "kvIdxs", kvIdxs, "error", err.Error())
		return nil, nil, nil, nil, nil, err
	}
	for i := uint64(0); i < checksLen; i++ {
		kvData, exist, err := w.storageMgr.TryRead(kvIdxs[i], int(w.storageMgr.MaxKvSize()), kvHashes[i])
		if exist && err == nil {
			dataSet[i] = kvData
			sampleIdxsInKv[i] = sampleIdx[i] % (1 << sampleLenBits)
			encodingKeys[i] = es.CalcEncodeKey(kvHashes[i], kvIdxs[i], t.miner)
			encodedSample, err := w.storageMgr.ReadSampleUnlocked(t.shardIdx, sampleIdx[i])
			if err != nil {
				return nil, nil, nil, nil, nil, err
			}
			encodedSamples[i] = encodedSample
		} else {
			if !exist {
				err = fmt.Errorf("kv not found: index=%d", kvIdxs[i])
			}
			w.lg.Error("Get data error", "index", kvIdxs[i], "error", err.Error())
			return nil, nil, nil, nil, nil, err
		}
	}
	return dataSet, kvIdxs, sampleIdxsInKv, encodingKeys, encodedSamples, nil
}
