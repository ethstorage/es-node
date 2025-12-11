// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
)

type Scanner struct {
	worker         *Worker
	feed           *event.Feed
	interval       time.Duration
	cfg            Config
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	running        bool
	mu             sync.Mutex // protects running
	lg             log.Logger
	scanPermit     chan struct{} // to ensure only one scan at a time
	statsMu        sync.Mutex    // protects sharedStats and sharedErrCache
	sharedStats    stats
	sharedErrCache scanErrors
}

type scanLoopState struct {
	mode      int
	nextIndex uint64
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
		worker:         NewWorker(sm, fetchBlob, l1, uint64(cfg.BatchSize), lg),
		feed:           feed,
		interval:       time.Minute * time.Duration(cfg.Interval),
		cfg:            cfg,
		ctx:            cctx,
		cancel:         cancel,
		lg:             lg,
		scanPermit:     make(chan struct{}, 1),
		sharedStats:    *newStats(),
		sharedErrCache: scanErrors{},
	}
	scanner.scanPermit <- struct{}{}
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

	s.startReporter()

	if s.cfg.Mode == modeCheckBlob+modeCheckMeta {
		// TODO: blobInterval := time.Hour * 24 * time.Duration(s.cfg.Interval)
		blobInterval := time.Minute * 9 * time.Duration(s.cfg.Interval) //  test only
		s.lg.Info("Scanner running in hybrid mode", "mode", s.cfg.Mode, "metaInterval", s.interval, "blobInterval", blobInterval)
		s.launchScanLoop(&scanLoopState{mode: modeCheckBlob}, blobInterval)
		s.launchScanLoop(&scanLoopState{mode: modeCheckMeta}, s.interval)
		return
	}

	s.launchScanLoop(&scanLoopState{mode: s.cfg.Mode}, s.interval)
}

func (s *Scanner) launchScanLoop(state *scanLoopState, interval time.Duration) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		s.lg.Info("Scanner started", "mode", state.mode, "interval", interval.String(), "batchSize", s.cfg.BatchSize)

		mainTicker := time.NewTicker(interval)
		defer mainTicker.Stop()

		s.doScan(state)
		for {
			select {
			case <-mainTicker.C:
				s.doScan(state)

			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *Scanner) doScan(state *scanLoopState) {
	if !s.acquireScanPermit() {
		return
	}
	tracker := s.cloneSharedMismatches()
	initSts, initErrs, err := s.doWork(state, tracker)
	s.releaseScanPermit()
	if err != nil {
		s.lg.Error("Scanner: initial scan failed", "mode", state.mode, "error", err)
	} else {
		s.updateSharedStats(initSts, initErrs)
	}
}

func (s *Scanner) acquireScanPermit() bool {
	select {
	case <-s.ctx.Done():
		return false
	case <-s.scanPermit:
		return true
	}
}

func (s *Scanner) releaseScanPermit() {
	select {
	case s.scanPermit <- struct{}{}:
	default:
	}
}

func (s *Scanner) startReporter() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				statsSnapshot, errSnapshot := s.snapshotSharedState()
				s.logStats(statsSnapshot)
				for i, e := range errSnapshot {
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
		"mode", s.cfg.Mode,
		"localKvs", sts.localKvs,
		"localKvsCount", sts.total,
	}
	if len(sts.mismatched) > 0 {
		logFields = append(logFields, "mismatched", sts.mismatched.String())
	}
	s.lg.Info("Scanner stats", logFields...)
}

func (s *Scanner) cloneSharedMismatches() mismatchTracker {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	return s.sharedStats.mismatched.clone()
}

func (s *Scanner) updateSharedStats(sts *stats, errs scanErrors) {
	if sts == nil {
		return
	}
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	s.sharedStats.localKvs = sts.localKvs
	s.sharedStats.total = sts.total
	if sts.mismatched != nil {
		s.sharedStats.mismatched = sts.mismatched.clone()
	} else {
		s.sharedStats.mismatched = mismatchTracker{}
	}
	if errs != nil {
		if s.sharedErrCache == nil {
			s.sharedErrCache = scanErrors{}
		}
		s.sharedErrCache.merge(errs)
	}
}

func (s *Scanner) snapshotSharedState() (*stats, scanErrors) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	statsCopy := &stats{
		localKvs:   s.sharedStats.localKvs,
		total:      s.sharedStats.total,
		mismatched: s.sharedStats.mismatched.clone(),
	}
	errCopy := scanErrors{}
	maps.Copy(errCopy, s.sharedErrCache)
	return statsCopy, errCopy
}

func (s *Scanner) GetScanState() *ScanStats {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	return &ScanStats{
		MismatchedCount: len(s.sharedStats.mismatched),
		UnfixedCount:    len(s.sharedStats.mismatched.failed()),
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

func (s *Scanner) doWork(state *scanLoopState, tracker mismatchTracker) (*stats, scanErrors, error) {
	start := time.Now()
	defer func(stt time.Time) {
		s.lg.Info("Scanner: scan batch done", "mode", state.mode, "duration", time.Since(stt).String())
	}(start)

	return s.worker.ScanBatch(s.ctx, state, tracker)
}
