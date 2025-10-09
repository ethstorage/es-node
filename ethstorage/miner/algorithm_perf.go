package miner

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
)

type MinerPerfRunner struct {
	config     Config
	dataReader DataReader
	storageMgr *es.StorageManager
	miner      common.Address
	wg         sync.WaitGroup
	lg         log.Logger
	processed  []uint64
	startTime  time.Time
}

func NewMinerPerfRunner(config Config, dataReader DataReader, storageMgr *es.StorageManager, miner common.Address, lg log.Logger) *MinerPerfRunner {
	return &MinerPerfRunner{
		config:     config,
		dataReader: dataReader,
		storageMgr: storageMgr,
		miner:      miner,
		lg:         lg,
		processed:  make([]uint64, config.ThreadsPerShard),
	}
}

func (r *MinerPerfRunner) Start() {
	randomHash := make([]byte, 32)
	if _, err := rand.Read(randomHash); err != nil {
		r.lg.Crit("failed to init random hash: %v", err)
	}
	r.startTime = time.Now()
	for i := uint64(0); i < r.config.ThreadsPerShard; i++ {
		r.wg.Add(1)
		go func(threadID uint64, rh common.Hash) {
			defer r.wg.Done()
			seg := r.config.NonceLimit / r.config.ThreadsPerShard
			end := seg * (threadID + 1)
			if threadID == r.config.ThreadsPerShard-1 {
				end = r.config.NonceLimit
			}
			for j := seg * threadID; j < end; j++ {
				hash0 := initHash(r.miner, rh, j)
				var err error
				_, _, err = hashimoto(r.storageMgr.KvEntriesBits(),
					r.storageMgr.MaxKvSizeBits(),
					es.SampleSizeBits,
					0,
					r.config.RandomChecks,
					r.storageMgr.ReadSampleUnlocked, hash0)
				r.processed[threadID]++
				if err != nil {
					r.lg.Warn("hashimoto error", "threadID", threadID, "err", err)
					return
				}
			}
			r.lg.Info("thread finished", "threadID", threadID)
		}(i, common.BytesToHash(randomHash))
	}
	r.wg.Wait()
}

func (r *MinerPerfRunner) ProcessState() (string, string) {
	sec := uint64(time.Now().Sub(r.startTime).Seconds())
	processed := uint64(0)
	for _, p := range r.processed {
		processed += p
	}
	if sec == 0 {
		return "mining power", fmt.Sprintf("total processed: 0, processed/s: 0")
	}
	return "mining power", fmt.Sprintf("total processed: %d, processed/s: %d, processed/slot: %d", processed, processed/sec, processed*12/sec)
}

func (r *MinerPerfRunner) Stop() {

}

func FactRead(shardIdx, sampleIdx uint64) (common.Hash, error) {
	randomHash := make([]byte, 32)
	rand.Read(randomHash)
	time.Sleep(time.Millisecond)
	return common.BytesToHash(randomHash), nil
}
