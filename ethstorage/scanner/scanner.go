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
}

func New(
	ctx context.Context,
	cfg Config,
	sm *es.StorageManager,
	loadKvFromCache func(uint64, common.Hash) []byte,
	l1 es.Il1Source,
	feed *event.Feed,
	lg log.Logger,
) *Scanner {
	cctx, cancel := context.WithCancel(ctx)
	scanner := &Scanner{
		worker:   NewWorker(sm, loadKvFromCache, l1, cfg, lg),
		feed:     feed,
		interval: time.Minute * time.Duration(cfg.Interval),
		ctx:      cctx,
		cancel:   cancel,
		lg:       lg,
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

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		mainTicker := time.NewTicker(s.interval)
		s.lg.Info("Scanner started", "mode", s.worker.cfg.Mode, "interval", s.interval.String(), "batchSize", s.worker.cfg.BatchSize)
		defer mainTicker.Stop()
		reportTicker := time.NewTicker(1 * time.Minute)
		defer reportTicker.Stop()
		errCache := make([]scanError, 0)
		rpt := report{}

		s.doWork()

	loop:
		for {
			select {
			case <-mainTicker.C:
				s.doWork()
			case <-reportTicker.C:
				s.lg.Info("Scanner stats", "kvStored", rpt.total, "mismatched", rpt.mismatched, "fixed", rpt.fixed, "failed", rpt.failed)
				for _, e := range errCache {
					s.lg.Error("Scanner error happened", "kvIndex", e.kvIndex, "error", e.err)
				}
			case r := <-reportCh:
				rpt.total = r.total
				rpt.mismatched += r.mismatched
				rpt.fixed += r.fixed
				rpt.failed += r.failed
			case err := <-errCh:
				// cache only the last error for each kvIndex
				for _, e := range errCache {
					if e.kvIndex == err.kvIndex {
						continue loop
					}
				}
				errCache = append(errCache, err)
			case <-s.ctx.Done():
				return
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
	s.lg.Info("Scan batch started")
	start := time.Now()
	defer func(stt time.Time) {
		s.lg.Info("Scan batch done", "duration", time.Since(stt).String())
	}(start)
	if err := s.worker.ScanBatch(s.ctx); err != nil {
		s.lg.Error("Scan batch failed", "err", err)
	}
}
