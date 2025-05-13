// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"context"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
)

type Scanner struct {
	worker   *Worker
	feed     *event.Feed
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	running  bool
	mu       sync.Mutex
	lg       log.Logger
	errorCh  chan scanError
	statsCh  chan stats
}

type LoadKvFromCacheFunc func(uint64, common.Hash) []byte

func New(
	ctx context.Context,
	cfg Config,
	sm *es.StorageManager,
	loadKvFromCache LoadKvFromCacheFunc,
	fetchBlob es.FetchBlobFunc,
	l1 es.Il1Source,
	feed *event.Feed,
	lg log.Logger,
) *Scanner {
	cctx, cancel := context.WithCancel(ctx)
	scanner := &Scanner{
		worker:   NewWorker(sm, loadKvFromCache, fetchBlob, l1, cfg, lg),
		feed:     feed,
		interval: time.Minute * time.Duration(cfg.Interval),
		ctx:      cctx,
		cancel:   cancel,
		lg:       lg,
		errorCh:  make(chan scanError, 10),
		statsCh:  make(chan stats, 10),
	}
	scanner.wg.Add(1)
	go scanner.update()
	return scanner
}

func (s *Scanner) update() {
	defer s.wg.Done()
	syncEventCh := make(chan protocol.EthStorageSyncDone)
	sub := s.feed.Subscribe(syncEventCh)
	defer func() {
		sub.Unsubscribe()
		close(syncEventCh)
		s.lg.Debug("Scanner event subscription closed")
	}()

	for {
		s.lg.Debug("Scanner update loop")
		select {
		case syncDone, ok := <-syncEventCh:
			if !ok {
				s.lg.Debug("syncEventCh closed, exiting update loop")
				return
			}
			if syncDone.DoneType == protocol.AllShardDone {
				s.lg.Info("Scanner update loop received event - all shards done.")
				s.start()
				return
			}
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Scanner) start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.wg.Add(2)

	// Reporting and error handling goroutine
	go func() {
		defer s.wg.Done()

		reportTicker := time.NewTicker(1 * time.Minute)
		defer reportTicker.Stop()

		errCache := make(map[uint64]scanError)
		sts := newStatsSum()
		statsUpdated := false

		for {
			select {
			case <-reportTicker.C:
				if statsUpdated { // Wait for stats updated for the first time
					s.lg.Info("Scanner stats", "kvStored", sts.total, "mismatched", sts.mismatched.String(), "fixed", sts.fixed.String(), "failed", sts.failed.String())
					for _, e := range errCache {
						s.lg.Error("Scanner error happened earlier", "kvIndex", e.kvIndex, "error", e.err)
					}
				}
			case err := <-s.errorCh:
				if err.err != nil {
					errCache[err.kvIndex] = err
				} else {
					delete(errCache, err.kvIndex)
				}
			case st := <-s.statsCh:
				sts.update(st)
				statsUpdated = true
			case <-s.ctx.Done():
				return
			}
		}
	}()

	// Main scanning loop
	go func() {
		defer s.wg.Done()

		s.lg.Info("Scanner started", "mode", s.worker.cfg.Mode, "interval", s.interval.String(), "batchSize", s.worker.cfg.BatchSize)

		mainTicker := time.NewTicker(s.interval)
		defer mainTicker.Stop()

		s.doWork()

		for {
			select {
			case <-mainTicker.C:
				s.doWork()
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *Scanner) sendError(kvIndex uint64, err error) {
	select {
	case s.errorCh <- scanError{kvIndex, err}:
	default:
		s.lg.Warn("Scanner: sent error to chan failed", "err", err, "lenOfCh", len(s.errorCh))
	}
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
	s.lg.Info("Scan batch started")
	start := time.Now()
	defer func(stt time.Time) {
		s.lg.Info("Scan batch done", "duration", time.Since(stt).String())
	}(start)

	sts, err := s.worker.ScanBatch(s.ctx, s.sendError)
	if err != nil {
		s.lg.Error("Scan batch failed", "err", err)
	}

	if sts != nil {
		select {
		case s.statsCh <- *sts:
		default:
			s.lg.Warn("Scanner: stats channel is full, dropping stats")
		}
	}
}
