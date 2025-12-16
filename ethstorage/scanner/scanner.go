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
	worker       *Worker
	feed         *event.Feed
	interval     time.Duration
	cfg          Config
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	running      bool
	mu           sync.Mutex // protects running
	lg           log.Logger
	scanPermit   chan struct{} // to ensure only one scan at a time
	statsMu      sync.Mutex    // protects sharedStats
	sharedStats  stats
	localKvCount uint64 // total number of kv entries stored in local
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
		interval:    time.Minute * time.Duration(cfg.Interval),
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

	if s.cfg.Mode == modeCheckBlob+modeCheckMeta {
		// Always keep blob interval 24 * 60 times of meta interval for hybrid mode
		blobInterval := time.Hour * 24 * time.Duration(s.cfg.Interval)
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
	localKvCount := s.localKvCount
	tracker := s.sharedStats.mismatched.clone()
	s.statsMu.Unlock()
	stats, err := s.worker.ScanBatch(s.ctx, state, localKvCount, tracker)
	s.releaseScanPermit()
	if err != nil {
		s.lg.Error("Scanner: initial scan failed", "mode", state.mode, "error", err)
	} else {
		s.updateSharedStats(stats)
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
				// update local entries info
				localKvs, sum := s.worker.summaryLocalKvs()
				s.statsMu.Lock()
				s.localKvCount = localKvs
				s.statsMu.Unlock()

				s.logStats(sum)
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *Scanner) logStats(sum string) {
	s.statsMu.Lock()
	localKvCount := s.localKvCount
	var mismatched string
	if len(s.sharedStats.mismatched) > 0 {
		mismatched = s.sharedStats.mismatched.String()
	}
	errSnapshot := scanErrors{}
	if s.sharedStats.errs != nil {
		maps.Copy(errSnapshot, s.sharedStats.errs)
	}
	s.statsMu.Unlock()

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

func (s *Scanner) updateSharedStats(sts *stats) {
	if sts == nil {
		return
	}
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	if sts.mismatched != nil {
		s.sharedStats.mismatched = sts.mismatched.clone()
	} else {
		s.sharedStats.mismatched = mismatchTracker{}
	}
	if sts.errs != nil {
		if s.sharedStats.errs == nil {
			s.sharedStats.errs = scanErrors{}
		}
		s.sharedStats.errs.merge(sts.errs)
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
