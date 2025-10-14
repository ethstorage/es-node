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

const (
	RandomChecks   = 2
	SampleSizeBits = 5
)

type MinerPerfRunner struct {
	nonceLimit    uint64
	threads       uint64
	kvEntriesBits uint64
	maxKvSizeBits uint64
	dataFileName  string
	miner         common.Address
	wg            sync.WaitGroup
	lg            log.Logger
	processed     []uint64
	startTime     time.Time
}

func NewMinerPerfRunner(kvEntriesBits, maxKvSizeBits, nonceLimit, threadsCount uint64, filename string, miner common.Address, lg log.Logger) *MinerPerfRunner {
	return &MinerPerfRunner{
		kvEntriesBits: kvEntriesBits,
		maxKvSizeBits: maxKvSizeBits,
		nonceLimit:    nonceLimit,
		threads:       threadsCount,
		dataFileName:  filename,
		miner:         miner,
		lg:            lg,
		processed:     make([]uint64, threadsCount),
	}
}

func (r *MinerPerfRunner) Start() {
	randomHash := make([]byte, 32)
	if _, err := rand.Read(randomHash); err != nil {
		r.lg.Crit("failed to init random hash: %v", err)
	}
	r.startTime = time.Now()
	for i := uint64(0); i < r.threads; i++ {
		r.wg.Add(1)
		go func(threadID uint64, rh common.Hash) {
			defer r.wg.Done()
			seg := r.nonceLimit / r.threads
			end := seg * (threadID + 1)
			if threadID == r.threads-1 {
				end = r.nonceLimit
			}
			df, err := es.OpenDataFile(r.dataFileName)
			if err != nil {
				r.lg.Crit("failed to open data file fail for thread %d: %v", threadID, err)
			}
			if df.Miner() != r.miner {
				r.lg.Crit("miner mismatches data file")
			}

			for j := seg * threadID; j < end; j++ {
				hash0 := initHash(r.miner, rh, j)
				var err error
				_, _, err = hashimoto(r.kvEntriesBits,
					r.maxKvSizeBits,
					SampleSizeBits,
					0,
					RandomChecks,
					df.ReadSample, hash0)
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
