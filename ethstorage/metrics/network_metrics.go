package metrics

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"net"
	"strconv"
	"time"

	ophttp "github.com/ethereum-optimism/optimism/op-service/httputil"
	"github.com/ethereum-optimism/optimism/op-service/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	SubsystemName = "network"
)

// NetworkMetrics tracks all the metrics for the es-node.
type NetworkMetrics struct {
	lastSubmissionTimes map[uint64]uint64

	// static Status
	PeersTotal      prometheus.Gauge
	MinersOfShards  *prometheus.GaugeVec
	PeersOfShards   *prometheus.GaugeVec
	PeersOfVersions *prometheus.GaugeVec
	PeersOfPhase    *prometheus.GaugeVec

	// peer metrics
	PeerInfo        *prometheus.GaugeVec
	SyncState       *prometheus.GaugeVec
	MiningState     *prometheus.GaugeVec
	SubmissionState *prometheus.GaugeVec

	registry *prometheus.Registry
	factory  metrics.Factory
}

// NewMetrics creates a new [NetworkMetrics] instance with the given process name.
func NewNetworkMetrics() *NetworkMetrics {
	ns := Namespace

	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registry.MustRegister(collectors.NewGoCollector())
	factory := metrics.With(registry)
	return &NetworkMetrics{

		PeersTotal: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "peers",
			Help:      "The number of peers existed in the last 10 minutes",
		}),

		MinersOfShards: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "miners",
			Help:      "The number of miners existed in each shard",
		}, []string{
			"shard_id",
		}),

		PeersOfShards: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "peers_of_shards",
			Help:      "The number of peers in each shard",
		}, []string{
			"shard_id",
		}),

		PeersOfVersions: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "peers_of_versions",
			Help:      "The number of peers for each version",
		}, []string{
			"version",
		}),

		PeersOfPhase: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "peers_of_phases",
			Help:      "The number of peers for each phase",
		}, []string{
			"shard_id",
			"phase",
		}),

		PeerInfo: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "peer_info",
			Help:      "The peer info of that record",
		}, []string{
			"peer_id",
			"Version",
			"Address",
		}),

		SyncState: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "sync_state",
			Help:      "The sync state of each peer",
		}, []string{
			"peer_id",
			"shard_id",
			"miner",
			"type",
		}),

		MiningState: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "mining_state",
			Help:      "The mining state of each peer",
		}, []string{
			"peer_id",
			"shard_id",
			"miner",
			"type",
		}),

		SubmissionState: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "submission_state",
			Help:      "The submission state of each peer",
		}, []string{
			"peer_id",
			"shard_id",
			"miner",
			"type",
		}),

		registry: registry,

		factory: factory,
	}
}

func (m *NetworkMetrics) SetPeerInfo(id, version, address string) {
	m.PeerInfo.WithLabelValues(id, version, address).Set(float64(time.Now().UnixMilli()))
}

func (m *NetworkMetrics) SetSyncState(id, shardId string, miner common.Address, peerCount int, syncProgress, syncedSeconds,
	syncTimeRemain, fillEmptyProgress, fillEmptySeconds, fillEmptyTimeRemain, providedBlobs uint64) {
	m.SyncState.WithLabelValues(id, shardId, miner.Hex(), "PeerCount").Set(float64(peerCount))
	m.SyncState.WithLabelValues(id, shardId, miner.Hex(), "SyncProgress").Set(float64(syncProgress) / 10000)
	m.SyncState.WithLabelValues(id, shardId, miner.Hex(), "SyncedSeconds").Set(float64(syncedSeconds) / 3600)
	m.SyncState.WithLabelValues(id, shardId, miner.Hex(), "SyncTimeRemain").Set(float64(syncTimeRemain) / 3600)
	m.SyncState.WithLabelValues(id, shardId, miner.Hex(), "FillEmptyProgress").Set(float64(fillEmptyProgress) / 10000)
	m.SyncState.WithLabelValues(id, shardId, miner.Hex(), "FillEmptySeconds").Set(float64(fillEmptySeconds) / 3600)
	m.SyncState.WithLabelValues(id, shardId, miner.Hex(), "FillEmptyTimeRemain").Set(float64(fillEmptyTimeRemain) / 3600)
	m.SyncState.WithLabelValues(id, shardId, miner.Hex(), "ProvidedBlobs").Set(float64(providedBlobs))
}

func (m *NetworkMetrics) SetMiningState(id, shardId string, miner common.Address, miningPower, samplingTime uint64) {
	m.MiningState.WithLabelValues(id, shardId, miner.Hex(), "MiningPower").Set(float64(miningPower) / 10000)
	m.MiningState.WithLabelValues(id, shardId, miner.Hex(), "SamplingTime").Set(float64(samplingTime) / 1000)
}

func (m *NetworkMetrics) SetSubmissionState(id, shardId string, miner common.Address, succeeded, failed, dropped int) {
	m.SubmissionState.WithLabelValues(id, shardId, miner.Hex(), "Succeeded").Set(float64(succeeded))
	m.SubmissionState.WithLabelValues(id, shardId, miner.Hex(), "Failed").Set(float64(failed))
	m.SubmissionState.WithLabelValues(id, shardId, miner.Hex(), "Dropped").Set(float64(dropped))
}

func (m *NetworkMetrics) SetStaticMetrics(peersTotal int, minerOfShards map[uint64]map[common.Address]struct{},
	versions map[string]int, shards map[uint64]int, phasesOfShard map[uint64]map[string]int) {
	m.PeersTotal.Set(float64(peersTotal))

	for shardId, miners := range minerOfShards {
		m.MinersOfShards.WithLabelValues(fmt.Sprintf("%d", shardId)).Set(float64(len(miners)))
	}
	for shardId, count := range shards {
		m.PeersOfShards.WithLabelValues(fmt.Sprintf("%d", shardId)).Set(float64(count))
	}
	for version, count := range versions {
		m.PeersOfVersions.WithLabelValues(version).Set(float64(count))
	}
	for shardId, phases := range phasesOfShard {
		for phase, count := range phases {
			m.PeersOfPhase.WithLabelValues(fmt.Sprintf("%d", shardId), phase).Set(float64(count))
		}
	}
}

func (m *NetworkMetrics) Serve(ctx context.Context, hostname string, port int) error {
	addr := net.JoinHostPort(hostname, strconv.Itoa(port))
	server := ophttp.NewHttpServer(promhttp.InstrumentMetricHandler(
		m.registry, promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}),
	))
	server.Addr = addr
	go func() {
		<-ctx.Done()
		server.Close()
	}()
	return server.ListenAndServe()
}
