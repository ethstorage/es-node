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

	switch s.cfg.Mode {
	case modeDisabled:
		s.lg.Info("Scanner is disabled")
		return
	case modeCheckMeta:
		s.launchScanLoop(s.metaScanLoopRuntime())
		s.lg.Info("Scanner started in meta check mode")
	case modeCheckBlob:
		s.launchScanLoop(s.blobScanLoopRuntime())
		s.lg.Info("Scanner started in blob check mode")
	case modeCheckBlock:
		// Launch the scan loop for the updated KVs in the latest blocks every 24 hours
		s.launchScanLoop(s.latestScanLoopRuntime())
		s.lg.Info("Scanner started in block check mode")
	case modeHybrid:
		// hybrid mode
		s.launchScanLoop(s.metaScanLoopRuntime())
		s.launchScanLoop(s.latestScanLoopRuntime())
		s.lg.Info("Scanner started in hybrid mode")
	default:
		s.lg.Error("Invalid scanner mode", "mode", s.cfg.Mode)
		return
	}

	s.startReporter()
	// Launch the scan loop to fix mismatched KVs every 12 minutes FIXME: adjust interval?
	s.launchFixLoop(time.Minute * 12)
}

func (s *Scanner) latestScanLoopRuntime() *scanLoopRuntime {
	return &scanLoopRuntime{
		mode:      modeCheckBlock,
		nextBatch: s.worker.latestUpdated,
		interval:  s.cfg.IntervalBlock,
		batchSize: 7200, // start back from 7200 blocks (1 day for Ethereum L1) ago
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

		s.lg.Info("Launching scanner loop", "mode", state.mode, "interval", state.interval.String(), "batchSize", state.batchSize)

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
		s.updateStats(kvi, m)
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

		s.lg.Info("Launching scan fix loop", "interval", interval.String())

		fixTicker := time.NewTicker(interval)
		defer fixTicker.Stop()

		for {
			select {
			case <-fixTicker.C:
				s.lg.Info("Scanner fix batch triggered")
				// hold for 3 minutes before fixing to allow possible ongoing kv downloading to finish
				time.Sleep(time.Minute * 3)
				if !s.acquireScanPermit() {
					s.lg.Warn("Skipping fix scan batch since another scan is ongoing")
					continue
				}
				s.statsMu.Lock()
				kvIndices := s.sharedStats.needFix()
				s.statsMu.Unlock()

				err := s.worker.fixBatch(s.ctx, kvIndices, func(kvi uint64, m *scanned) {
					s.updateStats(kvi, m)
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

func (s *Scanner) updateStats(kvi uint64, m *scanned) {
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

		localKvCount, sum := s.worker.summaryLocalKvs()
		s.lg.Info("Local storage summary", "localKvs", sum, "localKvCount", localKvCount)
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
	localKvCount, sum := s.worker.summaryLocalKvs()
	s.lg.Info("Local storage summary", "localKvs", sum, "localKvCount", localKvCount)

	s.statsMu.Lock()
	mismatched := "(none)"
	if len(s.sharedStats) > 0 {
		mismatched = s.sharedStats.String()
	}
	errSnapshot := make(map[uint64]error)
	if s.sharedStats.hasError() {
		maps.Copy(errSnapshot, s.sharedStats.withErrors())
	}
	s.statsMu.Unlock()

	s.lg.Info("Scanner stats", "mode", s.cfg.Mode, "mismatched", mismatched)
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
