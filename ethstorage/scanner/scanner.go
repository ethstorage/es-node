// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"context"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
)

type Scanner struct {
	worker    *Worker
	feed      *event.Feed
	interval  time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   bool
	mu        sync.Mutex
	lg        log.Logger
	scanStats ScanStats
}

func New(
	ctx context.Context,
	cfg Config,
	sm *es.StorageManager,
	fetchBlob es.FetchBlobFunc,
	l1 es.Il1Source,
	feed *event.Feed,
	lg log.Logger,
) *Scanner {
	cctx, cancel := context.WithCancel(ctx)
	scanner := &Scanner{
		worker:    NewWorker(sm, fetchBlob, l1, cfg, lg),
		feed:      feed,
		interval:  time.Minute * time.Duration(cfg.Interval),
		ctx:       cctx,
		cancel:    cancel,
		lg:        lg,
		scanStats: ScanStats{0, 0},
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

		s.lg.Info("Scanner started", "mode", s.worker.cfg.Mode, "interval", s.interval.String(), "batchSize", s.worker.cfg.BatchSize)

		mainTicker := time.NewTicker(s.interval)
		reportTicker := time.NewTicker(1 * time.Minute)
		defer mainTicker.Stop()
		defer reportTicker.Stop()
		stats := newStats()
		sts, errCache, err := s.doWork(mismatchTracker{})
		if err != nil {
			s.lg.Error("Initial scan failed", "error", err)
		}
		if sts != nil {
			stats = sts
			s.setScanState(sts)
		}
		for {
			select {
			case <-mainTicker.C:
				sts, scanErrs, err := s.doWork(stats.mismatched.clone())
				if err != nil {
					s.lg.Error("Scanner: scan batch failed", "error", err)
					continue
				}
				if sts != nil {
					stats = sts
					s.setScanState(sts)
				}
				errCache.merge(scanErrs)

			case <-reportTicker.C:
				s.logStats(stats)
				for i, e := range errCache {
					s.lg.Info("Scanner error happened earlier", "kvIndex", i, "error", e)
				}

			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *Scanner) logStats(sts *stats) {
	logFields := []any{
		"localKvs", sts.localKvs,
		"localKvsCount", sts.total,
	}
	if len(sts.mismatched) > 0 {
		logFields = append(logFields, "mismatched", sts.mismatched.String())
	}
	s.lg.Info("Scanner stats", logFields...)
}

func (s *Scanner) GetScanState() *ScanStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := s.scanStats // Make a copy
	return &snapshot        // Return a pointer to the copy
}

func (s *Scanner) setScanState(sts *stats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scanStats.MismatchedCount = len(sts.mismatched)
	s.scanStats.UnfixedCount = len(sts.mismatched.failed())
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

func (s *Scanner) doWork(tracker mismatchTracker) (*stats, scanErrors, error) {
	s.lg.Debug("Scan batch started")
	start := time.Now()
	defer func(stt time.Time) {
		s.lg.Info("Scan batch done", "duration", time.Since(stt).String())
	}(start)

	return s.worker.ScanBatch(s.ctx, tracker)
}
