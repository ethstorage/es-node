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
	sharedStats scannedKVs
	statsMu     sync.Mutex // protects sharedStats
}

func New(
	ctx context.Context,
	cfg Config,
	sm *es.StorageManager,
	fetchBlob es.FetchBlobFunc,
	l1 IL1,
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
		sharedStats: scannedKVs{},
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
		s.launchScanLoop(s.blobScanLoopRuntime())
		s.launchScanLoop(s.metaScanLoopRuntime())
	} else {
		s.lg.Info("Scanner running in single mode", "mode", s.cfg.Mode, "interval", s.cfg.IntervalMeta)
		s.launchScanLoop(s.defaultScanLoopRuntime())
	}

	// Launch the scan loop to fix mismatched KVs every 12 minutes
	s.launchFixLoop(time.Minute * 12)

	// Launch the scan loop for the updated KVs within the last scan interval using blob mode
	s.launchScanLoop(&scanLoopRuntime{mode: modeCheckBlob, nextBatch: s.worker.latestUpdated, interval: s.cfg.IntervalBlob, batchSize: 7200})
}

func (s *Scanner) defaultScanLoopRuntime() *scanLoopRuntime {
	interval := s.cfg.IntervalMeta
	if s.cfg.Mode == modeCheckBlob {
		interval = s.cfg.IntervalBlob
	}
	return &scanLoopRuntime{
		mode:      s.cfg.Mode,
		nextBatch: s.worker.getKvsInBatch,
		interval:  interval,
		batchSize: uint64(s.cfg.BatchSize),
		nextIndex: 0,
	}
}

func (s *Scanner) blobScanLoopRuntime() *scanLoopRuntime {
	return &scanLoopRuntime{
		mode:      modeCheckBlob,
		nextBatch: s.worker.getKvsInBatch,
		interval:  s.cfg.IntervalBlob,
		batchSize: uint64(s.cfg.BatchSize),
		nextIndex: 0,
	}
}

func (s *Scanner) metaScanLoopRuntime() *scanLoopRuntime {
	return &scanLoopRuntime{
		mode:      modeCheckMeta,
		nextBatch: s.worker.getKvsInBatch,
		interval:  s.cfg.IntervalMeta,
		batchSize: uint64(s.cfg.BatchSize),
		nextIndex: 0,
	}
}

func (s *Scanner) launchScanLoop(state *scanLoopRuntime) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		s.lg.Info("Scanner configured", "mode", state.mode, "interval", state.interval.String(), "batchSize", state.batchSize)

		mainTicker := time.NewTicker(state.interval)
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

func (s *Scanner) doScan(state *scanLoopRuntime) {
	if !s.acquireScanPermit() {
		return
	}
	err := s.worker.scanBatch(s.ctx, state, func(kvi uint64, m *scanned) {
		s.applyUpdate(kvi, m)
	})
	s.releaseScanPermit()
	if err != nil {
		s.lg.Error("Scan batch failed", "mode", state.mode, "error", err)
	}
}

func (s *Scanner) launchFixLoop(interval time.Duration) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		s.lg.Info("Scanner fix loop started", "interval", interval.String())

		fixTicker := time.NewTicker(interval)
		defer fixTicker.Stop()

		for {
			select {
			case <-fixTicker.C:
				s.lg.Info("Scanner fix loop triggered")

				if !s.acquireScanPermit() {
					return
				}
				// hold for 3 minutes before fixing to allow possible ongoing kv downloading to finish
				time.Sleep(time.Minute * 3)

				s.statsMu.Lock()
				kvIndices := s.sharedStats.needFix()
				s.statsMu.Unlock()

				err := s.worker.fixBatch(s.ctx, kvIndices, func(kvi uint64, m *scanned) {
					s.applyUpdate(kvi, m)
				})
				s.releaseScanPermit()
				if err != nil {
					s.lg.Error("Fix scan batch failed", "error", err)
				}

			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *Scanner) applyUpdate(kvi uint64, m *scanned) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	if m != nil {
		s.sharedStats[kvi] = *m
	} else {
		delete(s.sharedStats, kvi)
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
	if len(s.sharedStats) > 0 {
		mismatched = s.sharedStats.String()
	}
	errSnapshot := make(map[uint64]error)
	if s.sharedStats.hasError() {
		maps.Copy(errSnapshot, s.sharedStats.withErrors())
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
	if s == nil {
		return &ScanStats{}
	}
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	return &ScanStats{
		MismatchedCount: len(s.sharedStats),
		UnfixedCount:    len(s.sharedStats.failed()),
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
