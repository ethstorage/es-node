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
	worker      *Worker
	feed        *event.Feed
	cfg         Config
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	running     bool
	mu          sync.Mutex // protects running
	lg          log.Logger
	scanPermit  chan struct{} // to ensure only one scan at a time
	statsMu     sync.Mutex    // protects sharedStats
	sharedStats stats
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
		worker:      NewWorker(sm, fetchBlob, l1, uint64(cfg.BatchSize), lg),
		feed:        feed,
		cfg:         cfg,
		ctx:         cctx,
		cancel:      cancel,
		lg:          lg,
		scanPermit:  make(chan struct{}, 1),
		sharedStats: *newStats(),
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

	if s.cfg.Mode == modeDisabled {
		s.lg.Info("Scanner is disabled")
		return
	}
	if s.cfg.Mode == modeCheckBlob+modeCheckMeta {
		s.lg.Info("Scanner running in hybrid mode", "mode", s.cfg.Mode, "metaInterval", s.cfg.IntervalMeta, "blobInterval", s.cfg.IntervalBlob)
		s.launchScanLoop(&scanLoopState{mode: modeCheckBlob}, s.cfg.IntervalBlob)
		s.launchScanLoop(&scanLoopState{mode: modeCheckMeta}, s.cfg.IntervalMeta)
		return
	}
	interval := s.cfg.IntervalMeta
	if s.cfg.Mode == modeCheckBlob {
		interval = s.cfg.IntervalBlob
	}
	s.launchScanLoop(&scanLoopState{mode: s.cfg.Mode}, interval)
}

func (s *Scanner) launchScanLoop(state *scanLoopState, interval time.Duration) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		s.lg.Info("Scanner configured", "mode", state.mode, "interval", interval.String(), "batchSize", s.cfg.BatchSize)

		mainTicker := time.NewTicker(interval)
		defer mainTicker.Stop()

		s.doWork(state)
		for {
			select {
			case <-mainTicker.C:
				s.doWork(state)

			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *Scanner) doWork(state *scanLoopState) {
	if !s.acquireScanPermit() {
		return
	}
	s.statsMu.Lock()
	tracker := s.sharedStats.mismatched.clone()
	s.statsMu.Unlock()
	onUpdate := func(u scanUpdate) {
		s.applyUpdate(u)
	}
	err := s.worker.ScanBatch(s.ctx, state, tracker, onUpdate)
	s.releaseScanPermit()
	if err != nil {
		s.lg.Error("Scanner: initial scan failed", "mode", state.mode, "error", err)
	}
}

func (s *Scanner) applyUpdate(u scanUpdate) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	if u.status != nil {
		if s.sharedStats.mismatched == nil {
			s.sharedStats.mismatched = mismatchTracker{}
		}
		s.sharedStats.mismatched[u.kvIndex] = *u.status
	} else if u.status == nil {
		delete(s.sharedStats.mismatched, u.kvIndex)
	}

	if u.err != nil {
		if s.sharedStats.errs == nil {
			s.sharedStats.errs = scanErrors{}
		}
		s.sharedStats.errs[u.kvIndex] = u.err
		return
	}
	// nil err means clear error state for this kv index
	if s.sharedStats.errs != nil {
		delete(s.sharedStats.errs, u.kvIndex)
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

		s.logStats()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.logStats()
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *Scanner) logStats() {
	s.statsMu.Lock()
	var mismatched string
	if len(s.sharedStats.mismatched) > 0 {
		mismatched = s.sharedStats.mismatched.String()
	}
	errSnapshot := scanErrors{}
	if s.sharedStats.errs != nil {
		maps.Copy(errSnapshot, s.sharedStats.errs)
	}
	s.statsMu.Unlock()

	localKvCount, sum := s.worker.summaryLocalKvs()
	logFields := []any{
		"mode", s.cfg.Mode,
		"localKvs", sum,
		"localKvCount", localKvCount,
	}
	if mismatched != "" {
		logFields = append(logFields, "mismatched", mismatched)
	}
	s.lg.Info("Scanner stats", logFields...)

	for i, e := range errSnapshot {
		s.lg.Info("Scanner error happened earlier", "kvIndex", i, "error", e)
	}
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
