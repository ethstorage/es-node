package metrics

import (
	"fmt"
	"runtime"
	runtimemetrics "runtime/metrics"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
)

var Enabled = false

var rm *runtimeMetrics

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
	loger          log.Logger
}

func NewRuntimeMetrics() *runtimeMetrics {
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

func CollectProcessMetrics(refresh time.Duration) {
	if !Enabled {
		return
	}

	if rm == nil {
		rm = NewRuntimeMetrics()
	}

	// Create the various data collectors
	var (
		cpuStats        = make([]metrics.CPUStats, 2)
		diskStats       = make([]metrics.DiskStats, 2)
		rStats          = make([]runtimeStats, 2)
		lastCollectTime time.Time
		cpuCount        = runtime.NumCPU()
	)

	// Iterate loading the different stats and updating the meters.
	now, prev := 0, 1
	for ; ; now, prev = prev, now {
		// Gather CPU times.
		metrics.ReadCPUStats(&cpuStats[now])
		collectTime := time.Now()
		secondsSinceLastCollect := collectTime.Sub(lastCollectTime).Seconds()
		lastCollectTime = collectTime
		if secondsSinceLastCollect > 0 {
			// Convert to integer percentage.
			rm.cpuSysLoad.addValue(int64((cpuStats[now].GlobalTime - cpuStats[prev].GlobalTime) / float64(cpuCount) / secondsSinceLastCollect * 100))
			rm.cpuProcLoad.addValue(int64((cpuStats[now].LocalTime - cpuStats[prev].LocalTime) / float64(cpuCount) / secondsSinceLastCollect * 100))
		}

		// Go runtime metrics
		readRuntimeStats(&rStats[now])

		rm.memAllocs.addValue(int64((rStats[now].GCAllocBytes - rStats[prev].GCAllocBytes) / 1024 / 1024))
		rm.memTotal.addValue(int64((rStats[now].MemTotal) / 1024 / 1024))
		rm.heapUsed.addValue(int64((rStats[now].MemTotal - rStats[now].HeapUnused - rStats[now].HeapFree - rStats[now].HeapReleased) / 1024 / 1024))

		// Disk
		if metrics.ReadDiskStats(&diskStats[now]) == nil {
			rm.diskReadBytes.addValue((diskStats[now].ReadBytes - diskStats[prev].ReadBytes) / 1024 / 1024)
			rm.diskWriteBytes.addValue((diskStats[now].WriteBytes - diskStats[prev].WriteBytes) / 1024 / 1024)
		}

		log.Info("runtime metrics", "cpu (%)", int64((cpuStats[now].GlobalTime-cpuStats[prev].GlobalTime)/float64(cpuCount)/secondsSinceLastCollect*100),
			"memory (MB)", int64((rStats[now].MemTotal)/1024/1024), "disk read (KB)", (diskStats[now].ReadBytes-diskStats[prev].ReadBytes)/1024, "disk write (KB)",
			(diskStats[now].WriteBytes-diskStats[prev].WriteBytes)/1024)
		time.Sleep(refresh)
	}
}

func PrintRuntimeMetrics() {
	if Enabled && rm != nil {
		rm.loger.Info("runtime metrics", "summary", rm.String())
	}
}
