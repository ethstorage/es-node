package metrics

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	ophttp "github.com/ethereum-optimism/optimism/op-service/httputil"
	"github.com/ethereum-optimism/optimism/op-service/metrics"
	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	SubsystemName = "network"
	format        = "{\"id\":\"%s\", \"contract\":\"%s\", \"version\":\"%s\", \"address\":\"%s\", \"shardid\":%d, \"miner\":\"%s\"}"
)

// NetworkMetrics tracks all the metrics for the es-node.
type NetworkMetrics struct {
	// static Status
	PeersOfContract *prometheus.GaugeVec
	PeersOfShards   *prometheus.GaugeVec
	PeersOfVersions *prometheus.GaugeVec
	PeersOfPhase    *prometheus.GaugeVec

	// peer metrics
	PeerState *prometheus.GaugeVec

	registry *prometheus.Registry
	factory  metrics.Factory
}

// NewNetworkMetrics creates a new [NetworkMetrics] instance with the given process name.
func NewNetworkMetrics() *NetworkMetrics {
	ns := Namespace

	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registry.MustRegister(collectors.NewGoCollector())
	factory := metrics.With(registry)
	return &NetworkMetrics{

		PeersOfContract: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "peers_of_contracts",
			Help:      "The number of peers in each contract",
		}, []string{
			"contract",
		}),

		PeersOfShards: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "peers_of_shards",
			Help:      "The number of peers in each shard",
		}, []string{
			"contract",
			"shard_id",
		}),

		PeersOfVersions: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "peers_of_versions",
			Help:      "The number of peers for each version",
		}, []string{
			"contract",
			"version",
		}),

		PeersOfPhase: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "peers_of_phases",
			Help:      "The number of peers for each phase",
		}, []string{
			"contract",
			"shard_id",
			"phase",
		}),

		PeerState: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SubsystemName,
			Name:      "peer_state",
			Help:      "The sync state of each peer",
		}, []string{
			"contract",
			"key",
			"type",
		}),

		registry: registry,
		factory:  factory,
	}
}

func (m *NetworkMetrics) SetPeerInfo(id, contract, version, address string, shardId uint64, miner common.Address) {
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "UpdateTime").Set(float64(time.Now().UnixMilli()))
}

func (m *NetworkMetrics) SetScanState(id, contract, version, address string, shardId uint64, miner common.Address, mismatchedCount, unfixedCount int) {
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "MismatchedKVCount").Set(float64(mismatchedCount))
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "UnfixedKVCount").Set(float64(unfixedCount))
}

func (m *NetworkMetrics) SetDownloadState(id, contract, version, address string, shardId uint64, miner common.Address, savedBlobs, downloadedBlobs uint64) {
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "SavedBlobs").Set(float64(savedBlobs))
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "DownloadedBlobs").Set(float64(downloadedBlobs))
}

func (m *NetworkMetrics) SetSyncState(id, contract, version, address string, shardId uint64, miner common.Address, peerCount int, syncProgress, syncedSeconds,
	fillEmptyProgress, fillEmptySeconds, providedBlobs uint64) {
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "PeerCount").Set(float64(peerCount))
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "SyncProgress").Set(float64(syncProgress) / 100)
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "SyncedSeconds").Set(float64(syncedSeconds) / 3600)
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "FillEmptyProgress").Set(float64(fillEmptyProgress) / 100)
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "FillEmptySeconds").Set(float64(fillEmptySeconds) / 3600)
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "ProvidedBlobs").Set(float64(providedBlobs))
}

func (m *NetworkMetrics) SetMiningState(id, contract, version, address string, shardId uint64, miner common.Address, miningPower, samplingTime uint64) {
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "MiningPower").Set(float64(miningPower) / 100)
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "SamplingTime").Set(float64(samplingTime) / 1000)
}

func (m *NetworkMetrics) SetSubmissionState(id, contract, version, address string, shardId uint64, miner common.Address, succeeded, failed, dropped int, lastSucceededTime int64) {
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "Succeeded").Set(float64(succeeded))
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "Failed").Set(float64(failed))
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "Dropped").Set(float64(dropped))
	m.PeerState.WithLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), "LastSucceededTime").Set(float64(lastSucceededTime))
}

func (m *NetworkMetrics) SetStaticMetrics(contract string, peersCount int,
	versions map[string]int, shards map[uint64]int, phasesOfShard map[uint64]map[string]int) {
	m.PeersOfContract.WithLabelValues(contract).Set(float64(peersCount))

	for shardId, count := range shards {
		m.PeersOfShards.WithLabelValues(contract, fmt.Sprintf("%d", shardId)).Set(float64(count))
	}
	for version, count := range versions {
		m.PeersOfVersions.WithLabelValues(contract, version).Set(float64(count))
	}
	for shardId, phases := range phasesOfShard {
		for phase, count := range phases {
			m.PeersOfPhase.WithLabelValues(contract, fmt.Sprintf("%d", shardId), phase).Set(float64(count))
		}
	}
}

func (m *NetworkMetrics) ResetStaticMetrics() {
	m.PeersOfContract.Reset()
	m.PeersOfShards.Reset()
	m.PeersOfVersions.Reset()
	m.PeersOfPhase.Reset()
}

func (m *NetworkMetrics) DeletePeerInfo(id, contract, version, address string, shardId uint64, miner common.Address) {
	types := []string{"UpdateTime", "PeerCount", "SyncProgress", "SyncedSeconds", "FillEmptyProgress", "FillEmptySeconds",
		"MiningPower", "SamplingTime", "Succeeded", "Failed", "Dropped", "LastSucceededTime", "ProvidedBlobs"}
	for _, t := range types {
		m.PeerState.DeleteLabelValues(contract, fmt.Sprintf(format, id, contract, version, address, shardId, miner.Hex()), t)
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
