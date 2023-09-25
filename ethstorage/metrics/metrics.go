package metrics

import (
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	PeerScores        *prometheus.GaugeVec
	GossipEventsTotal *prometheus.CounterVec
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
