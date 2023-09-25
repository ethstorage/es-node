// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package protocol

import (
	"strconv"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

const (
	Namespace = "p2p"

	SyncServerSubsystem = "sync_server"
	SyncClientSubsystem = "sync_client"
)

type Metricer interface {
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
}

// Metrics tracks all the metrics for the op-node.
type Metrics struct {
	SyncClientRequestsTotal              *prometheus.CounterVec
	SyncClientRequestDurationSeconds     *prometheus.HistogramVec
	SyncClientState                      *prometheus.GaugeVec
	SyncClientPeerRequestsTotal          *prometheus.CounterVec
	SyncClientPeerRequestDurationSeconds *prometheus.HistogramVec
	SyncClientPeerState                  *prometheus.GaugeVec

	SyncClientPerfCallTotal           *prometheus.CounterVec
	SyncClientPerfCallDurationSeconds *prometheus.HistogramVec

	// P2P Metrics
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
	m.SyncClientPerfCallTotal.WithLabelValues(method).Add(float64(duration) / float64(time.Second) / float64(count))
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
	m.SyncClientPerfCallTotal.WithLabelValues(method).Add(float64(duration) / float64(time.Second))
}

func (m *Metrics) ClientOnBlobsByList(peerID string, reqCount, retBlobCount, insertedCount uint64, duration time.Duration) {

	method := "onBlobsByList"
	m.SyncClientPerfCallTotal.WithLabelValues(method).Inc()
	m.SyncClientPerfCallTotal.WithLabelValues(method).Add(float64(duration) / float64(time.Second))
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

type noopMetricer struct{}

var NoopMetrics Metricer = new(noopMetricer)

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
