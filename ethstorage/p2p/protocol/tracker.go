package protocol

import (
	"github.com/ethereum/go-ethereum/log"
	"math"
	"sync"
	"time"
)

// measurementImpact is the impact a single measurement has on a peer's final
// capacity value. A value closer to 0 reacts slower to sudden network changes,
// but it is also more stable against temporary hiccups. 0.1 worked well for
// most of Ethereum's existence, so might as well go with it.
const measurementImpact = 0.1

// capacityOverestimation is the ratio of items to over-estimate when retrieving
// a peer's capacity to avoid locking into a lower value due to never attempting
// to fetch more than some local stable value.
const capacityOverestimation = 1.01

// Tracker estimates the throughput capacity of a peer with regard to each data
// type it can deliver. The goal is to dynamically adjust request sizes to max
// out network throughput without overloading either the peer or th elocal node.
//
// By tracking in real time the latencies and bandiwdths peers exhibit for each
// packet type, it's possible to prevent overloading by detecting a slowdown on
// one type when another type is pushed too hard.
//
// Similarly, real time measurements also help avoid overloading the local net
// connection if our peers would otherwise be capable to deliver more, but the
// local link is saturated. In that case, the live measurements will force us
// to reduce request sizes until the throughput gets stable.
//
// Lastly, message rate measurements allows us to detect if a peer is unsuaully
// slow compared to other peers, in which case we can decide to keep it around
// or free up the slot so someone closer.
//
// Since throughput tracking and estimation adapts dynamically to live network
// conditions, it's fine to have multiple trackers locally track the same peer
// in different subsystem. The throughput will simply be distributed across the
// two trackers if both are highly active.
type Tracker struct {
	// capacity is the number of items retrievable per second of a given type.
	// It is analogous to bandwidth, but we deliberately avoided using bytes
	// as the unit, since serving nodes also spend a lot of time loading data
	// from disk, which is linear in the number of items, but mostly constant
	// in their sizes.
	peerID   string
	capacity float64

	lock sync.RWMutex
}

// NewTracker creates a new message rate tracker for a specific peer.
func NewTracker(peerID string, cap float64) *Tracker {
	return &Tracker{
		peerID:   peerID,
		capacity: cap,
	}
}

// Capacity calculates the number of items the peer is estimated to be able to
// retrieve within the allotted time slot. The method will round up any division
// errors and will add an additional overestimation ratio on top. The reason for
// overshooting the capacity is because certain message types might not increase
// the load proportionally to the requested items, so fetching a bit more might
// still take the same RTT. By forcefully overshooting by a small amount, we can
// avoid locking into a lower-that-real capacity.
func (t *Tracker) Capacity(targetRTT float64) float64 {
	t.lock.RLock()
	defer t.lock.RUnlock()

	// Calculate the actual measured throughput
	throughput := t.capacity * targetRTT

	// Return an overestimation to force the peer out of a stuck minima, adding
	// +1 in case the item count is too low for the overestimator to dent
	return roundCapacity(1 + capacityOverestimation*throughput)
}

// roundCapacity gives the integer value of a capacity.
// The result fits int32, and is guaranteed to be positive.
func roundCapacity(cap float64) float64 {
	return math.Min(maxRequestSize, math.Ceil(cap))
}

// Update modifies the peer's capacity values for a specific data type with a new
// measurement. If the delivery is zero, the peer is assumed to have either timed
// out or to not have the requested data, resulting in a slash to 0 capacity. This
// avoids assigning the peer retrievals that it won't be able to honour.
func (t *Tracker) Update(elapsed time.Duration, items int) {
	t.lock.Lock()
	defer t.lock.Unlock()

	// Otherwise update the throughput with a new measurement
	if elapsed <= 0 {
		elapsed = 1 // +1 (ns) to ensure non-zero divisor
	}
	measured := float64(items) / (float64(elapsed) / float64(time.Second))

	oldcap := t.capacity
	t.capacity = (1-measurementImpact)*(t.capacity) + measurementImpact*measured
	log.Debug("Update tracker", "peer id", t.peerID, "elapsed", elapsed, "items", items, "old capacity", oldcap, "capacity", t.capacity)
}
