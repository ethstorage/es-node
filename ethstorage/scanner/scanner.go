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
	worker     *Worker
	feed       *event.Feed
	cfg        Config
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	running    bool
	mu         sync.Mutex // protects running
	lg         log.Logger
	stats      mismatchTracker
	statsMu    sync.Mutex    // protects stats
	scanPermit chan struct{} // to ensure only one scan at a time
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
	return &Scanner{
		worker:     NewWorker(sm, fetchBlob, l1, lg),
		feed:       feed,
		cfg:        cfg,
		ctx:        cctx,
		cancel:     cancel,
		lg:         lg,
		stats:      make(mismatchTracker),
		scanPermit: make(chan struct{}, 1),
	}
}

// Start begins the scanner's background processing. Must be called after New().
func (s *Scanner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.scanPermit <- struct{}{}
	s.wg.Add(1)
	go s.update()
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

	if s.cfg.Mode&modeSetMeta != 0 {
		s.launchScanLoop(s.metaScanLoopRuntime())
	}
	if s.cfg.Mode&modeSetBlob != 0 {
		s.launchScanLoop(s.blobScanLoopRuntime())
	}
	if s.cfg.Mode&modeSetBlock != 0 {
		s.launchScanLoop(s.blockScanLoopRuntime())
	}

	s.lg.Info("Scanner started", "mode", s.cfg.Mode.String())

	s.startReporter()

	s.launchFixLoop(time.Minute * fixingInterval)
}

func (s *Scanner) launchScanLoop(rt *scanLoopRuntime) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.lg.Info("Launching scanner loop", "mode", rt.mode, "interval", rt.interval, "batchSize", rt.batchSize)
		mainTicker := time.NewTicker(rt.interval)
		defer mainTicker.Stop()

		s.doScan(rt)
		for {
			select {
			case <-mainTicker.C:
				s.doScan(rt)

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
	err := s.worker.scanBatch(s.ctx, state, func(kvi uint64, sc *scanned) {
		s.updateStats(kvi, sc)
	})
	s.releaseScanPermit()
	if err != nil {
		s.lg.Error("Scan batch failed", "mode", state.mode, "error", err)
	}
}

func (s *Scanner) updateStats(kvi uint64, sc *scanned) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	if sc != nil {
		if sc.status == pending && s.stats[kvi].status == failed {
			// keep failed status until fixed
			return
		}
		s.stats[kvi] = *sc
	} else {
		// fixed or recovered
		delete(s.stats, kvi)
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

func (s *Scanner) blobScanLoopRuntime() *scanLoopRuntime {
	return &scanLoopRuntime{
		mode:      modeCheckBlob,
		nextBatch: s.worker.getKvsInBatch,
		interval:  s.cfg.IntervalBlob,
		batchSize: uint64(s.cfg.BatchSize),
		nextIndex: 0,
	}
}

func (s *Scanner) blockScanLoopRuntime() *scanLoopRuntime {
	return &scanLoopRuntime{
		mode:      modeCheckBlock,
		nextBatch: s.worker.latestUpdated,
		interval:  s.cfg.IntervalBlock,
		batchSize: uint64(s.cfg.IntervalBlock / s.cfg.L1SlotTime), // number of slots in the interval
		nextIndex: 0,
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
				// hold until other possible ongoing scans finish
				if !s.acquireScanPermit() {
					return
				}
				s.statsMu.Lock()
				kvIndices := s.stats.needFix()
				s.statsMu.Unlock()
				s.lg.Info("Scanner fixing batch triggered", "mismatchesToFix", kvIndices)
				err := s.worker.fixBatch(s.ctx, kvIndices, func(kvi uint64, m *scanned) {
					s.updateStats(kvi, m)
				})
				s.releaseScanPermit()
				if err != nil {
					s.lg.Error("Fixing batch failed", "error", err)
				}

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
	mismatched := "[]"
	if len(s.stats) > 0 {
		mismatched = s.stats.String()
	}
	errSnapshot := make(map[uint64]error)
	if s.stats.hasError() {
		maps.Copy(errSnapshot, s.stats.withErrors())
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
		MismatchedCount: len(s.stats),
		UnfixedCount:    len(s.stats.failed()),
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

func (s *Scanner) Close() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	s.cancel()
	s.wg.Wait()
	s.lg.Info("Scanner closed")
}
