// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

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
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	Namespace = "es_node"

	SyncServerSubsystem = "sync_server"
	SyncClientSubsystem = "sync_client"
	ContractMetrics     = "contract_data"
)

type Metricer interface {
	SetLastKVIndexAndMaxShardId(lastL1Block, lastKVIndex uint64, maxShardId uint64)
	SetMiningInfo(shardId uint64, difficulty, minedTime, blockMined uint64, miner common.Address, gasFee, reward uint64)

	ClientGetBlobsByRangeEvent(peerID string, resultCode byte, duration time.Duration)
	ClientGetBlobsByListEvent(peerID string, resultCode byte, duration time.Duration)
	ClientFillEmptyBlobsEvent(count uint64, duration time.Duration)
	ClientOnBlobsByRange(peerID string, reqCount, retBlobCount, insertedCount uint64, duration time.Duration)
	ClientOnBlobsByList(peerID string, reqCount, retBlobCount, insertedCount uint64, duration time.Duration)
	ClientRecordTimeUsed(method string) func()
	IncDropPeerCount()
	IncPeerCount()
	DecPeerCount()
	ServerGetBlobsByRangeEvent(peerID string, resultCode byte, duration time.Duration)
	ServerGetBlobsByListEvent(peerID string, resultCode byte, duration time.Duration)
	ServerReadBlobs(peerID string, read, sucRead uint64, timeUse time.Duration)
	ServerRecordTimeUsed(method string) func()
	Document() []metrics.DocumentedMetric
	RecordGossipEvent(evType int32)
	SetPeerScores(map[string]float64)
	RecordUp()
	RecordInfo(version string)
	Serve(ctx context.Context, hostname string, port int) error
}

// Metrics tracks all the metrics for the es-node.
type Metrics struct {
	lastSubmissionTimes map[uint64]uint64

	// Contract Status
	LastL1Block             prometheus.Gauge
	LastKVIndex             prometheus.Gauge
	Shards                  prometheus.Gauge
	Difficulties            *prometheus.GaugeVec
	LastSubmissionTime      *prometheus.GaugeVec
	MinedTime               *prometheus.GaugeVec
	BlockMined              *prometheus.GaugeVec
	LastMinerSubmissionTime *prometheus.GaugeVec
	MiningReward            *prometheus.GaugeVec
	GasFee                  *prometheus.GaugeVec

	// P2P Metrics
	PeerScores        *prometheus.GaugeVec
	GossipEventsTotal *prometheus.CounterVec

	SyncClientRequestsTotal              *prometheus.CounterVec
	SyncClientRequestDurationSeconds     *prometheus.HistogramVec
	SyncClientState                      *prometheus.GaugeVec
	SyncClientPeerRequestsTotal          *prometheus.CounterVec
	SyncClientPeerRequestDurationSeconds *prometheus.HistogramVec
	SyncClientPeerState                  *prometheus.GaugeVec

	SyncClientPerfCallTotal           *prometheus.CounterVec
	SyncClientPerfCallDurationSeconds *prometheus.HistogramVec

	PeerCount     prometheus.Gauge
	DropPeerCount prometheus.Counter

	SyncServerHandleReqTotal                  *prometheus.CounterVec
	SyncServerHandleReqDurationSeconds        *prometheus.HistogramVec
	SyncServerHandleReqState                  *prometheus.GaugeVec
	SyncServerHandleReqTotalPerPeer           *prometheus.CounterVec
	SyncServerHandleReqDurationSecondsPerPeer *prometheus.HistogramVec
	SyncServerHandleReqStatePerPeer           *prometheus.GaugeVec
	SyncServerPerfCallTotal                   *prometheus.CounterVec
	SyncServerPerfCallDurationSeconds         *prometheus.HistogramVec

	Info *prometheus.GaugeVec
	Up   prometheus.Gauge

	registry *prometheus.Registry
	factory  metrics.Factory
}

var _ Metricer = (*Metrics)(nil)

// NewMetrics creates a new [Metrics] instance with the given process name.
func NewMetrics(procName string) *Metrics {
	if procName == "" {
		procName = "default"
	}
	ns := Namespace + "_" + procName

	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registry.MustRegister(collectors.NewGoCollector())
	factory := metrics.With(registry)
	return &Metrics{
		lastSubmissionTimes: make(map[uint64]uint64),

		LastL1Block: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: ContractMetrics,
			Name:      "last_l1_block",
			Help:      "the last l1 block monitored",
		}),

		LastKVIndex: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: ContractMetrics,
			Name:      "last_kv_index",
			Help:      "the last kv index in the l1 miner contract",
		}),

		Shards: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: ContractMetrics,
			Name:      "max_shard_id",
			Help:      "the max shard id support by the l1 miner contract",
		}),

		Difficulties: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: ContractMetrics,
			Name:      "difficulty_of_shards",
			Help:      "The difficulty of shards in the l1 miner contract",
		}, []string{
			"shard_id",
		}),

		LastSubmissionTime: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: ContractMetrics,
			Name:      "last_submission_time_of_shards",
			Help:      "The last time of shards in the l1 miner contract",
		}, []string{
			"shard_id",
		}),

		MinedTime: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: ContractMetrics,
			Name:      "last_mined_time_of_shards",
			Help:      "The time used by mining of shards in the l1 miner contract",
		}, []string{
			"shard_id",
			"block_mined",
		}),

		BlockMined: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: ContractMetrics,
			Name:      "block_mined_of_shards",
			Help:      "The block mined of shards in the l1 miner contract",
		}, []string{
			"shard_id",
		}),

		LastMinerSubmissionTime: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: ContractMetrics,
			Name:      "last_miner_submission_time_of_shards",
			Help:      "The last submission time of shards for miners in the l1 miner contract",
		}, []string{
			"shard_id",
			"miner",
			"block_mined",
		}),

		MiningReward: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: ContractMetrics,
			Name:      "mining_reward_of_submission",
			Help:      "The mining reward of a submission in the l1 miner contract",
		}, []string{
			"shard_id",
			"miner",
			"block_mined",
		}),

		GasFee: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: ContractMetrics,
			Name:      "gas_fee_of_submission",
			Help:      "The gas fee of a submission in the l1 miner contract",
		}, []string{
			"shard_id",
			"miner",
			"block_mined",
		}),

		SyncClientRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Subsystem: SyncClientSubsystem,
			Name:      "requests_total",
			Help:      "Total P2P requests sent",
		}, []string{
			"p2p_method",
			"result_code",
		}),

		SyncClientRequestDurationSeconds: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns,
			Subsystem: SyncClientSubsystem,
			Name:      "request_duration_seconds",
			Buckets:   []float64{},
			Help:      "Duration of P2P requests",
		}, []string{
			"p2p_method",
			"result_code",
		}),

		SyncClientState: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SyncClientSubsystem,
			Name:      "sync_state",
			Help:      "The state of sync client",
		}, []string{
			"state",
		}),

		SyncClientPeerRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Subsystem: SyncClientSubsystem,
			Name:      "requests_total_for_peer",
			Help:      "Total P2P requests sent by a peer",
		}, []string{
			"peer_id",
			"p2p_method",
			"result_code",
		}),

		SyncClientPeerRequestDurationSeconds: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns,
			Subsystem: SyncClientSubsystem,
			Name:      "request_duration_seconds_for_peer",
			Buckets:   []float64{},
			Help:      "Duration of P2P requests per peer",
		}, []string{
			"peer_id",
			"p2p_method",
			"result_code",
		}),

		SyncClientPeerState: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SyncClientSubsystem,
			Name:      "sync_state_for_peer",
			Help:      "The sync state of peer",
		}, []string{
			"peer_id",
			"state",
		}),

		SyncClientPerfCallTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Subsystem: SyncClientSubsystem,
			Name:      "calls_total",
			Help:      "Number of call for method which need performance data",
		}, []string{
			"method",
		}),

		SyncClientPerfCallDurationSeconds: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns,
			Subsystem: SyncClientSubsystem,
			Name:      "call_duration_seconds",
			Buckets:   []float64{},
			Help:      "Duration of calls",
		}, []string{
			"method",
		}),

		PeerCount: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SyncClientSubsystem,
			Name:      "peer_count",
			Help:      "Count of currently connected p2p peers",
		}),

		DropPeerCount: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SyncClientSubsystem,
			Name:      "drop_peer_count",
			Help:      "Count of peers drop by sync client deal to peer limit",
		}),

		SyncServerHandleReqTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Subsystem: SyncServerSubsystem,
			Name:      "handle_req_total",
			Help:      "Number of P2P requests handle by sync server",
		}, []string{
			"p2p_method",
			"result_code",
		}),

		SyncServerHandleReqDurationSeconds: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns,
			Subsystem: SyncServerSubsystem,
			Name:      "handle_req_duration_seconds",
			Buckets:   []float64{},
			Help:      "Duration of P2P requests",
		}, []string{
			"p2p_method",
			"result_code",
		}),

		SyncServerHandleReqState: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SyncServerSubsystem,
			Name:      "handle_req_state",
			Help:      "The handle request state of sync server",
		}, []string{
			"state",
		}),

		SyncServerHandleReqTotalPerPeer: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Subsystem: SyncServerSubsystem,
			Name:      "handle_req_total_per_peer",
			Help:      "Number of P2P requests per peer",
		}, []string{
			"peer_id",
			"p2p_method",
			"result_code",
		}),

		SyncServerHandleReqDurationSecondsPerPeer: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns,
			Subsystem: SyncServerSubsystem,
			Name:      "handle_req_duration_seconds_per_peer",
			Buckets:   []float64{},
			Help:      "Duration of P2P requests per peer",
		}, []string{
			"peer_id",
			"p2p_method",
			"result_code",
		}),

		SyncServerHandleReqStatePerPeer: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: SyncServerSubsystem,
			Name:      "handle_req_state_of_peer",
			Help:      "The handle request state of peer",
		}, []string{
			"peer_id",
			"state",
		}),

		SyncServerPerfCallTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Subsystem: SyncServerSubsystem,
			Name:      "calls_total",
			Help:      "Number of call for method which need performance data",
		}, []string{
			"method",
		}),

		SyncServerPerfCallDurationSeconds: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns,
			Subsystem: SyncServerSubsystem,
			Name:      "call_duration_seconds",
			Buckets:   []float64{},
			Help:      "Duration of calls",
		}, []string{
			"method",
		}),

		PeerScores: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: "p2p",
			Name:      "peer_scores",
			Help:      "Count of peer scores grouped by score",
		}, []string{
			"band",
		}),

		GossipEventsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Subsystem: "p2p",
			Name:      "gossip_events_total",
			Help:      "Count of gossip events by type",
		}, []string{
			"type",
		}),

		Info: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "info",
			Help:      "Pseudo-metric tracking version and config info",
		}, []string{
			"version",
		}),

		Up: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "up",
			Help:      "1 if the es node has finished starting up",
		}),

		registry: registry,

		factory: factory,
	}
}

func (m *Metrics) Document() []metrics.DocumentedMetric {
	return m.factory.Document()
}

// Serve starts the metrics server on the given hostname and port.
// The server will be closed when the passed-in context is cancelled.
func (m *Metrics) Serve(ctx context.Context, hostname string, port int) error {
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

func (m *Metrics) SetLastKVIndexAndMaxShardId(lastL1Block, lastKVIndex uint64, maxShardId uint64) {
	m.LastL1Block.Set(float64(lastL1Block))
	m.LastKVIndex.Set(float64(lastKVIndex))
	m.Shards.Set(float64(maxShardId))
}

func (m *Metrics) SetMiningInfo(shardId uint64, difficulty, minedTime, blockMined uint64, miner common.Address, gasFee, reward uint64) {
	if t, ok := m.lastSubmissionTimes[shardId]; ok && t <= minedTime {
		m.Difficulties.WithLabelValues(fmt.Sprintf("%d", shardId)).Set(float64(difficulty))
		m.LastSubmissionTime.WithLabelValues(fmt.Sprintf("%d", shardId)).Set(float64(minedTime))
		m.BlockMined.WithLabelValues(fmt.Sprintf("%d", shardId)).Set(float64(blockMined))

		m.LastMinerSubmissionTime.WithLabelValues(fmt.Sprintf("%d", shardId), miner.Hex(), fmt.Sprintf("%d", blockMined)).Set(float64(minedTime))
		m.GasFee.WithLabelValues(fmt.Sprintf("%d", shardId), miner.Hex(), fmt.Sprintf("%d", blockMined)).Set(float64(gasFee))
		m.MiningReward.WithLabelValues(fmt.Sprintf("%d", shardId), miner.Hex(), fmt.Sprintf("%d", blockMined)).Set(float64(reward))

		m.MinedTime.WithLabelValues(fmt.Sprintf("%d", shardId), fmt.Sprintf("%d", blockMined)).Set(float64(minedTime - t))
	}
	m.lastSubmissionTimes[shardId] = minedTime
}

func (m *Metrics) RecordGossipEvent(evType int32) {
	m.GossipEventsTotal.WithLabelValues(pb.TraceEvent_Type_name[evType]).Inc()
}

// SetPeerScores updates the peer score [prometheus.GaugeVec].
// This takes a map of labels to scores.
func (m *Metrics) SetPeerScores(scores map[string]float64) {
	for label, score := range scores {
		m.PeerScores.WithLabelValues(label).Set(score)
	}
}

func (m *Metrics) ClientGetBlobsByRangeEvent(peerID string, resultCode byte, duration time.Duration) {
	code := strconv.FormatUint(uint64(resultCode), 10)
	m.SyncClientRequestsTotal.WithLabelValues("get_blobs_by_range", code).Inc()
	m.SyncClientRequestDurationSeconds.WithLabelValues("get_blobs_by_range", code).Observe(float64(duration) / float64(time.Second))
	m.SyncClientPeerRequestsTotal.WithLabelValues(peerID, "get_blobs_by_range", code).Inc()
	m.SyncClientPeerRequestDurationSeconds.WithLabelValues(peerID, "get_blobs_by_range", code).Observe(float64(duration) / float64(time.Second))
}

func (m *Metrics) ClientGetBlobsByListEvent(peerID string, resultCode byte, duration time.Duration) {
	code := strconv.FormatUint(uint64(resultCode), 10)
	m.SyncClientRequestsTotal.WithLabelValues("get_blobs_by_list", code).Inc()
	m.SyncClientRequestDurationSeconds.WithLabelValues("get_blobs_by_list", code).Observe(float64(duration) / float64(time.Second))
	m.SyncClientPeerRequestsTotal.WithLabelValues(peerID, "get_blobs_by_list", code).Inc()
	m.SyncClientPeerRequestDurationSeconds.WithLabelValues(peerID, "get_blobs_by_list", code).Observe(float64(duration) / float64(time.Second))
}

func (m *Metrics) ClientFillEmptyBlobsEvent(count uint64, duration time.Duration) {
	method := "fillEmpty"
	m.SyncClientPerfCallTotal.WithLabelValues(method).Add(float64(count))
	m.SyncClientPerfCallDurationSeconds.WithLabelValues(method).Observe(float64(duration) / float64(time.Second) / float64(count))
}

func (m *Metrics) ClientOnBlobsByRange(peerID string, reqBlobCount, retBlobCount, insertedCount uint64, duration time.Duration) {
	m.SyncClientState.WithLabelValues("reqBlobCount").Add(float64(reqBlobCount))
	m.SyncClientState.WithLabelValues("retBlobCount").Add(float64(retBlobCount))
	m.SyncClientState.WithLabelValues("insertedBlobCount").Add(float64(insertedCount))

	m.SyncClientPeerState.WithLabelValues(peerID, "reqBlobCount").Add(float64(reqBlobCount))
	m.SyncClientPeerState.WithLabelValues(peerID, "retBlobCount").Add(float64(retBlobCount))
	m.SyncClientPeerState.WithLabelValues(peerID, "insertedBlobCount").Add(float64(insertedCount))

	method := "onBlobsByRange"
	m.SyncClientPerfCallTotal.WithLabelValues(method).Inc()
	m.SyncClientPerfCallDurationSeconds.WithLabelValues(method).Observe(float64(duration) / float64(time.Second))
}

func (m *Metrics) ClientOnBlobsByList(peerID string, reqCount, retBlobCount, insertedCount uint64, duration time.Duration) {
	m.SyncClientState.WithLabelValues("reqBlobCount").Add(float64(reqCount))
	m.SyncClientState.WithLabelValues("retBlobCount").Add(float64(retBlobCount))
	m.SyncClientState.WithLabelValues("insertedBlobCount").Add(float64(insertedCount))

	m.SyncClientPeerState.WithLabelValues(peerID, "reqBlobCount").Add(float64(reqCount))
	m.SyncClientPeerState.WithLabelValues(peerID, "retBlobCount").Add(float64(retBlobCount))
	m.SyncClientPeerState.WithLabelValues(peerID, "insertedBlobCount").Add(float64(insertedCount))

	method := "onBlobsByList"
	m.SyncClientPerfCallTotal.WithLabelValues(method).Inc()
	m.SyncClientPerfCallDurationSeconds.WithLabelValues(method).Observe(float64(duration) / float64(time.Second))
}

func (m *Metrics) ClientRecordTimeUsed(method string) func() {
	m.SyncClientPerfCallTotal.WithLabelValues(method).Inc()
	timer := prometheus.NewTimer(m.SyncClientPerfCallDurationSeconds.WithLabelValues(method))
	return func() {
		timer.ObserveDuration()
	}
}

func (m *Metrics) IncDropPeerCount() {
	m.DropPeerCount.Inc()
}

func (m *Metrics) IncPeerCount() {
	m.PeerCount.Inc()
}

func (m *Metrics) DecPeerCount() {
	m.PeerCount.Dec()
}

func (m *Metrics) ServerGetBlobsByRangeEvent(peerID string, resultCode byte, duration time.Duration) {
	code := strconv.FormatUint(uint64(resultCode), 10)
	m.SyncServerHandleReqTotal.WithLabelValues("get_blobs_by_range", code).Inc()
	m.SyncServerHandleReqDurationSeconds.WithLabelValues("get_blobs_by_range", code).Observe(float64(duration) / float64(time.Second))

	m.SyncServerHandleReqTotalPerPeer.WithLabelValues(peerID, "get_blobs_by_range", code).Inc()
	m.SyncServerHandleReqDurationSecondsPerPeer.WithLabelValues(peerID, "get_blobs_by_range", code).Observe(float64(duration) / float64(time.Second))
}

func (m *Metrics) ServerGetBlobsByListEvent(peerID string, resultCode byte, duration time.Duration) {
	code := strconv.FormatUint(uint64(resultCode), 10)
	m.SyncServerHandleReqTotal.WithLabelValues("get_blobs_by_list", code).Inc()
	m.SyncServerHandleReqDurationSeconds.WithLabelValues("get_blobs_by_list", code).Observe(float64(duration) / float64(time.Second))

	m.SyncServerHandleReqTotalPerPeer.WithLabelValues(peerID, "get_blobs_by_list", code).Inc()
	m.SyncServerHandleReqDurationSecondsPerPeer.WithLabelValues(peerID, "get_blobs_by_list", code).Observe(float64(duration) / float64(time.Second))
}

func (m *Metrics) ServerReadBlobs(peerID string, read, sucRead uint64, timeUse time.Duration) {
	m.SyncServerHandleReqState.WithLabelValues("read").Add(float64(read))
	m.SyncServerHandleReqState.WithLabelValues("sucRead").Add(float64(sucRead))
	m.SyncServerHandleReqStatePerPeer.WithLabelValues(peerID, "read").Add(float64(read))
	m.SyncServerHandleReqStatePerPeer.WithLabelValues(peerID, "sucRead").Add(float64(sucRead))

	method := "readBlobs"
	m.SyncServerPerfCallTotal.WithLabelValues(method).Inc()
	m.SyncServerPerfCallDurationSeconds.WithLabelValues(method).Observe(float64(timeUse) / float64(time.Second))
}

func (m *Metrics) ServerRecordTimeUsed(method string) func() {
	m.SyncServerPerfCallTotal.WithLabelValues(method).Inc()
	timer := prometheus.NewTimer(m.SyncServerPerfCallDurationSeconds.WithLabelValues(method))
	return func() {
		timer.ObserveDuration()
	}
}

// RecordInfo sets a pseudo-metric that contains versioning and
// config info for the es node.
func (m *Metrics) RecordInfo(version string) {
	m.Info.WithLabelValues(version).Set(1)
}

// RecordUp sets the up metric to 1.
func (m *Metrics) RecordUp() {
	prometheus.MustRegister()
	m.Up.Set(1)
}

type noopMetricer struct{}

var NoopMetrics = new(noopMetricer)

func (n *noopMetricer) Document() []metrics.DocumentedMetric {
	return nil
}

func (m *noopMetricer) Serve(ctx context.Context, hostname string, port int) error {
	return nil
}

func (m *noopMetricer) SetLastKVIndexAndMaxShardId(lastL1Block, lastKVIndex uint64, maxShardId uint64) {
}

func (m *noopMetricer) SetMiningInfo(shardId uint64, difficulty, minedTime, blockMined uint64, miner common.Address, gasFee, reward uint64) {
}

func (n *noopMetricer) ClientGetBlobsByRangeEvent(peerID string, resultCode byte, duration time.Duration) {
}

func (n *noopMetricer) ClientGetBlobsByListEvent(peerID string, resultCode byte, duration time.Duration) {
}

func (n *noopMetricer) ClientFillEmptyBlobsEvent(count uint64, duration time.Duration) {
}

func (n *noopMetricer) ClientOnBlobsByRange(peerID string, reqCount, retBlobCount, insertedCount uint64, duration time.Duration) {

}

func (n *noopMetricer) ClientOnBlobsByList(peerID string, reqCount, retBlobCount, insertedCount uint64, duration time.Duration) {
}

func (n *noopMetricer) ClientRecordTimeUsed(method string) func() {
	return func() {}
}

func (n *noopMetricer) IncDropPeerCount() {
}

func (n *noopMetricer) IncPeerCount() {
}

func (n *noopMetricer) DecPeerCount() {
}

func (n *noopMetricer) ServerGetBlobsByRangeEvent(peerID string, resultCode byte, duration time.Duration) {
}

func (n *noopMetricer) ServerGetBlobsByListEvent(peerID string, resultCode byte, duration time.Duration) {
}

func (n *noopMetricer) ServerReadBlobs(peerID string, read, sucRead uint64, timeUse time.Duration) {
}

func (n *noopMetricer) ServerRecordTimeUsed(method string) func() {
	return func() {}
}

func (m *noopMetricer) RecordGossipEvent(evType int32) {
}

func (m *noopMetricer) SetPeerScores(scores map[string]float64) {
}

func (n *noopMetricer) RecordInfo(version string) {
}

func (n *noopMetricer) RecordUp() {
}
