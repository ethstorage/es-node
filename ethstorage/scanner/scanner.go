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
	worker     *Worker
	feed       *event.Feed
	interval   time.Duration
	cfg        Config
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	running    bool
	mu         sync.Mutex
	lg         log.Logger
	scanStats  ScanStats
	scanPermit chan struct{} // to ensure only one scan at a time
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
		worker:     NewWorker(sm, fetchBlob, l1, lg),
		feed:       feed,
		interval:   time.Minute * time.Duration(cfg.Interval),
		cfg:        cfg,
		ctx:        cctx,
		cancel:     cancel,
		lg:         lg,
		scanStats:  ScanStats{0, 0},
		scanPermit: make(chan struct{}, 1),
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

	if s.cfg.Mode == modeCheckBlob+modeCheckMeta {
		// TODO: blobInterval := time.Hour * 24 * time.Duration(s.cfg.Interval)
		//  test only
		blobInterval := time.Minute * 9 * time.Duration(s.cfg.Interval)
		s.lg.Info("Scanner running in hybrid mode", "metaInterval", s.interval, "blobInterval", blobInterval)
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

		reportTicker := time.NewTicker(time.Minute)
		defer reportTicker.Stop()

		sts := newStats()
		errCache := scanErrors{}
		if !s.acquireScanPermit(state.mode) {
			return
		}
		initSts, initErrs, err := s.doWork(state, mismatchTracker{})
		s.releaseScanPermit()
		if err != nil {
			s.lg.Error("Scanner: initial scan failed", "mode", state.mode, "error", err)
		} else {
			sts = initSts
			errCache = initErrs
		}

		for {
			select {
			case <-mainTicker.C:
				if !s.acquireScanPermit(state.mode) {
					return
				}
				newSts, scanErrs, err := s.doWork(state, sts.mismatched.clone())
				s.releaseScanPermit()
				if err != nil {
					s.lg.Error("Scanner: scan batch failed", "mode", state.mode, "error", err)
					continue
				}
				sts = newSts
				errCache.merge(scanErrs)

			case <-reportTicker.C:
				s.logStats(state.mode, sts)
				for i, e := range errCache {
					s.lg.Info("Scanner error happened earlier", "kvIndex", i, "error", e)
				}

			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *Scanner) acquireScanPermit(mode int) bool {
	s.lg.Info("Scanner acquiring scan permit for mode", "mode", mode)
	select {
	case <-s.ctx.Done():
		return false
	case <-s.scanPermit:
		s.lg.Info("Scanner acquired scan permit for mode", "mode", mode)
		return true
	}
}

func (s *Scanner) releaseScanPermit() {
	select {
	case s.scanPermit <- struct{}{}:
	default:
	}
}

func (s *Scanner) logStats(mode int, sts *stats) {
	logFields := []any{
		"mode", mode,
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

func (s *Scanner) doWork(state *scanLoopState, tracker mismatchTracker) (*stats, scanErrors, error) {
	start := time.Now()
	defer func(stt time.Time) {
		s.lg.Info("Scanner: scan batch done", "mode", state.mode, "duration", time.Since(stt).String())
	}(start)

	sts, scanErrs, nextIndex, err := s.worker.ScanBatch(s.ctx, state.mode, s.cfg.BatchSize, state.nextIndex, tracker)
	if err == nil {
		state.nextIndex = nextIndex
		s.setScanState(sts)
	}
	return sts, scanErrs, err
}
