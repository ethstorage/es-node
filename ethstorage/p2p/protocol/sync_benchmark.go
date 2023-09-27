// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package protocol

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"runtime"
	runtimemetrics "runtime/metrics"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
)

type metric struct {
	name      string
	latestVal int64
	maxVal    int64
	count     int64
	totalVal  int64
}

func (m *metric) addValue(val int64) {
	m.latestVal = val
	if val > m.maxVal {
		m.maxVal = val
	}
	m.count++
	m.totalVal += val
}

func (m *metric) String() string {
	avg := int64(0)
	if m.count > 0 {
		avg = m.totalVal / m.count
	}
	return fmt.Sprintf("metric %s: max value %d; avg value %d;", m.name, m.maxVal, avg)
}

type runtimeMetrics struct {
	cpuSysLoad     *metric
	cpuProcLoad    *metric
	memAllocs      *metric
	memTotal       *metric
	heapUsed       *metric
	diskReadBytes  *metric
	diskWriteBytes *metric
}

func newRuntimeMetrics() *runtimeMetrics {
	return &runtimeMetrics{
		cpuSysLoad:     &metric{name: "cpuSysLoad"},
		cpuProcLoad:    &metric{name: "cpuProcLoad"},
		memAllocs:      &metric{name: "memAllocs"},
		memTotal:       &metric{name: "memTotal"},
		heapUsed:       &metric{name: "heapUsed"},
		diskReadBytes:  &metric{name: "diskReadBytes"},
		diskWriteBytes: &metric{name: "diskWriteBytes"},
	}
}

func (rm *runtimeMetrics) String() string {
	return fmt.Sprintf("runtime metrics:\r\n\t%s \r\n\t%s \r\n\t%s \r\n\t%s \r\n\t%s \r\n\t%s \r\n\t%s",
		rm.cpuSysLoad.String(), rm.cpuProcLoad.String(), rm.memAllocs.String(), rm.memTotal.String(),
		rm.heapUsed.String(), rm.diskReadBytes.String(), rm.diskWriteBytes.String())
}

var runtimeSamples = []runtimemetrics.Sample{
	{Name: "/gc/heap/allocs:bytes"},
	{Name: "/gc/heap/frees:bytes"},
	{Name: "/memory/classes/total:bytes"},
	{Name: "/memory/classes/heap/free:bytes"},
	{Name: "/memory/classes/heap/released:bytes"},
	{Name: "/memory/classes/heap/unused:bytes"},
}

type runtimeStats struct {
	GCAllocBytes uint64
	GCFreedBytes uint64

	MemTotal     uint64
	HeapFree     uint64
	HeapReleased uint64
	HeapUnused   uint64
}

func readRuntimeStats(v *runtimeStats) {
	runtimemetrics.Read(runtimeSamples)
	for _, s := range runtimeSamples {
		// Skip invalid/unknown metrics. This is needed because some metrics
		// are unavailable in older Go versions, and attempting to read a 'bad'
		// metric panics.
		if s.Value.Kind() == runtimemetrics.KindBad {
			continue
		}

		switch s.Name {
		case "/gc/heap/allocs:bytes":
			v.GCAllocBytes = s.Value.Uint64()
		case "/gc/heap/frees:bytes":
			v.GCFreedBytes = s.Value.Uint64()
		case "/memory/classes/total:bytes":
			v.MemTotal = s.Value.Uint64()
		case "/memory/classes/heap/free:bytes":
			v.HeapFree = s.Value.Uint64()
		case "/memory/classes/heap/released:bytes":
			v.HeapReleased = s.Value.Uint64()
		case "/memory/classes/heap/unused:bytes":
			v.HeapUnused = s.Value.Uint64()
		}
	}
}

func CollectProcessMetrics(refresh time.Duration, rm *runtimeMetrics) {
	// Create the various data collectors
	var (
		cpustats        = make([]metrics.CPUStats, 2)
		diskstats       = make([]metrics.DiskStats, 2)
		rstats          = make([]runtimeStats, 2)
		lastCollectTime time.Time
		cpuCount        = runtime.NumCPU()
	)

	// Iterate loading the different stats and updating the meters.
	now, prev := 0, 1
	for ; ; now, prev = prev, now {
		// Gather CPU times.
		metrics.ReadCPUStats(&cpustats[now])
		collectTime := time.Now()
		secondsSinceLastCollect := collectTime.Sub(lastCollectTime).Seconds()
		lastCollectTime = collectTime
		if secondsSinceLastCollect > 0 {
			// Convert to integer percentage.
			rm.cpuSysLoad.addValue(int64((cpustats[now].GlobalTime - cpustats[prev].GlobalTime) / float64(cpuCount) / secondsSinceLastCollect * 100))
			rm.cpuProcLoad.addValue(int64((cpustats[now].LocalTime - cpustats[prev].LocalTime) / float64(cpuCount) / secondsSinceLastCollect * 100))
		}

		// Go runtime metrics
		readRuntimeStats(&rstats[now])

		rm.memAllocs.addValue(int64((rstats[now].GCAllocBytes - rstats[prev].GCAllocBytes) / 1024 / 1024))

		rm.memTotal.addValue(int64((rstats[now].MemTotal) / 1024 / 1024))
		rm.heapUsed.addValue(int64((rstats[now].MemTotal - rstats[now].HeapUnused - rstats[now].HeapFree - rstats[now].HeapReleased) / 1024 / 1024))

		// Disk
		if metrics.ReadDiskStats(&diskstats[now]) == nil {
			rm.diskReadBytes.addValue((diskstats[now].ReadBytes - diskstats[prev].ReadBytes) / 1024 / 1024)
			rm.diskWriteBytes.addValue((diskstats[now].WriteBytes - diskstats[prev].WriteBytes) / 1024 / 1024)
		}
		time.Sleep(refresh)
	}
}

type Params struct {
	PeerCount   int
	KVSize      uint64
	ChunkSize   uint64
	KVEntries   uint64
	LastKVIndex uint64
	EncodeType  uint64
}

func TestSyncPerfTest(arg Params) {
	var (
		start       = time.Now()
		t           = &testing.T{}
		encodeType  = arg.EncodeType
		testLog     = log.New("TestPerf", "benchmark")
		db          = rawdb.NewMemoryDatabase()
		ctx, cancel = context.WithCancel(context.Background())
		mux         = new(event.Feed)
		shards      = []uint64{0}
		shardMap    = make(map[common.Address][]uint64)
		rm          = newRuntimeMetrics()
		rollupCfg   = &rollup.EsConfig{
			L2ChainID: new(big.Int).SetUint64(3333),
		}
	)

	shardMap[contract] = shards
	shardManager, files := createEthStorage(contract, shards, arg.ChunkSize, arg.KVSize, arg.KVEntries, common.Address{}, encodeType)
	if shardManager == nil {
		t.Fatalf("createEthStorage failed")
	}

	defer func(files []string) {
		for _, file := range files {
			os.Remove(file)
		}
	}(files)

	data := makeKVStorage(contract, shards, arg.ChunkSize, arg.KVSize, arg.KVEntries, arg.LastKVIndex, common.Address{}, encodeType)
	sm := &mockStorageManager{shardManager: shardManager, lastKvIdx: arg.LastKVIndex}
	testLog.Info("Test prepared", "time", time.Since(start))
	start = time.Now()

	localHost, syncCl := createLocalHostAndSyncClient(t, testLog, rollupCfg, db, sm, nil, mux)
	syncCl.Start()

	for i := 0; i < arg.PeerCount; i++ {
		smr := &mockStorageManagerReader{
			kvEntries:       arg.KVEntries,
			maxKvSize:       arg.KVSize,
			encodeType:      encodeType,
			shards:          shards,
			contractAddress: contract,
			shardMiner:      common.Address{},
			blobPayloads:    data[contract],
		}
		remoteHost := createRemoteHost(t, ctx, rollupCfg, smr, nil, testLog)
		connect(t, localHost, remoteHost, shardMap, shardMap)
	}

	go CollectProcessMetrics(time.Second, rm)

	checkStall(t, 3600, mux, cancel)

	if !syncCl.syncDone {
		testLog.Error("sync state %v is not match with expected state %v, peer count %d", syncCl.syncDone, false, len(syncCl.peers))
	}
	testLog.Info("Test done", "time", time.Since(start))
	testLog.Info(rm.String())
}
