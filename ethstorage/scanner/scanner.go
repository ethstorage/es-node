// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"context"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
)

const (
	MetaDownloadBatchSize = 10
)

type Scanner struct {
	worker   *Worker
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	running  bool
	mu       sync.Mutex
	lg       log.Logger
}

type StorageReader interface {
	TryRead(kvIdx uint64, readLen int, commit common.Hash) ([]byte, bool, error)
	MaxKvSize() uint64
	KvEntries() uint64
	Shards() []uint64
}

func New(ctx context.Context, cfg Config, sr StorageReader, l1 es.Il1Source, lg log.Logger) *Scanner {
	cctx, cancel := context.WithCancel(ctx)
	return &Scanner{
		worker:   NewWorker(sr, l1, lg),
		interval: time.Second * time.Duration(cfg.Interval),
		ctx:      cctx,
		cancel:   cancel,
		lg:       lg,
	}
}

func (s *Scanner) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.doWork()

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.doWork()
			}
		}
	}()
}

func (s *Scanner) Close() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	s.cancel()
	s.lg.Info("Scanner closed")
	s.wg.Wait()
}

func (s *Scanner) doWork() {
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		s.lg.Info("Scan batch done", "took(s)", dur.Seconds())
	}(start)

	if err := s.worker.ScanBatch(); err != nil {
		s.lg.Error("Scan batch", "err", err)
	}
}
