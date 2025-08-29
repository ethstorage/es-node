package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/golang/snappy"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/host"
)

const (
	// maxGossipSize limits the total size of gossip RPC containers as well as decompressed individual messages.
	maxGossipSize = 10 * (1 << 20)
	// minGossipSize is used to make sure that there is at least some data to validate the signature against.
	minGossipSize          = 66
	maxOutboundQueue       = 256
	maxValidateQueue       = 256
	globalValidateThrottle = 512
	gossipHeartbeat        = 500 * time.Millisecond
	// seenMessagesTTL limits the duration that message IDs are remembered for gossip deduplication purposes
	// 130 * gossipHeartbeat
	seenMessagesTTL  = 130 * gossipHeartbeat
	DefaultMeshD     = 8  // topic stable mesh target count
	DefaultMeshDlo   = 6  // topic stable mesh low watermark
	DefaultMeshDhi   = 12 // topic stable mesh high watermark
	DefaultMeshDlazy = 6  // gossip target
	// peerScoreInspectFrequency is the frequency at which peer scores are inspected
	peerScoreInspectFrequency = 15 * time.Second
)

// Message domains, the msg id function uncompresses to keep data monomorphic,
// but invalid compressed data will need a unique different id.

var MessageDomainInvalidSnappy = [4]byte{0, 0, 0, 0}
var MessageDomainValidSnappy = [4]byte{1, 0, 0, 0}

type GossipIn interface {
	// OnUnsafeL2Payload(ctx context.Context, from peer.ID, msg *eth.ExecutionPayload) error
}

// TODO:
func blocksTopicV1(cfg *rollup.EsConfig) string {
	// return fmt.Sprintf("/optimism/%s/0/blocks", cfg.L2ChainID.String())
	return ""
}

// BuildSubscriptionFilter builds a simple subscription filter,
// to help protect against peers spamming useless subscriptions.
func BuildSubscriptionFilter(cfg *rollup.EsConfig) pubsub.SubscriptionFilter {
	return pubsub.NewAllowlistSubscriptionFilter(blocksTopicV1(cfg)) // add more topics here in the future, if any.
}

var msgBufPool = sync.Pool{New: func() any {
	// note: the topic validator concurrency is limited, so pool won't blow up, even with large pre-allocation.
	x := make([]byte, 0, maxGossipSize)
	return &x
}}

//go:generate mockery --name GossipMetricer
type GossipMetricer interface {
	RecordGossipEvent(evType int32)
	// SetPeerScores Peer Scoring Metric Funcs
	SetPeerScores(map[string]float64)
}

// BuildMsgIdFn builds a generic message ID function for gossipsub that can handle compressed payloads,
// mirroring the eth2 p2p gossip spec.
func BuildMsgIdFn(cfg *rollup.EsConfig) pubsub.MsgIdFunction {
	return func(pmsg *pb.Message) string {
		valid := false
		var data []byte
		// If it's a valid compressed snappy data, then hash the uncompressed contents.
		// The validator can throw away the message later when recognized as invalid,
		// and the unique hash helps detect duplicates.
		dLen, err := snappy.DecodedLen(pmsg.Data)
		if err == nil && dLen <= maxGossipSize {
			res := msgBufPool.Get().(*[]byte)
			defer msgBufPool.Put(res)
			if data, err = snappy.Decode((*res)[:0], pmsg.Data); err == nil {
				*res = data // if we ended up growing the slice capacity, fine, keep the larger one.
				valid = true
			}
		}
		if data == nil {
			data = pmsg.Data
		}
		h := sha256.New()
		if valid {
			h.Write(MessageDomainValidSnappy[:])
		} else {
			h.Write(MessageDomainInvalidSnappy[:])
		}
		// The chain ID is part of the gossip topic, making the msg id unique
		topic := pmsg.GetTopic()
		var topicLen [8]byte
		binary.LittleEndian.PutUint64(topicLen[:], uint64(len(topic)))
		h.Write(topicLen[:])
		h.Write([]byte(topic))
		h.Write(data)
		// the message ID is shortened to save space, a lot of these may be gossiped.
		return string(h.Sum(nil)[:20])
	}
}

func (c *Config) ConfigureGossip(rollupCfg *rollup.EsConfig) []pubsub.Option {
	params := BuildGlobalGossipParams(rollupCfg)

	// override with CLI changes
	params.D = c.MeshD
	params.Dlo = c.MeshDLo
	params.Dhi = c.MeshDHi
	params.Dlazy = c.MeshDLazy

	// in the future we may add more advanced options like scoring and PX / direct-mesh / episub
	return []pubsub.Option{
		pubsub.WithGossipSubParams(params),
		pubsub.WithFloodPublish(c.FloodPublish),
	}
}

func BuildGlobalGossipParams(cfg *rollup.EsConfig) pubsub.GossipSubParams {
	params := pubsub.DefaultGossipSubParams()
	params.D = DefaultMeshD                    // topic stable mesh target count
	params.Dlo = DefaultMeshDlo                // topic stable mesh low watermark
	params.Dhi = DefaultMeshDhi                // topic stable mesh high watermark
	params.Dlazy = DefaultMeshDlazy            // gossip target
	params.HeartbeatInterval = gossipHeartbeat // interval of heartbeat
	params.FanoutTTL = 24 * time.Second        // ttl for fanout maps for topics we are not subscribed to but have published to
	params.HistoryLength = 12                  // number of windows to retain full messages in cache for IWANT responses
	params.HistoryGossip = 3                   // number of windows to gossip about

	return params
}

// NewGossipSub configures a new pubsub instance with the specified parameters.
// PubSub uses a GossipSubRouter as it's router under the hood.
func NewGossipSub(p2pCtx context.Context, h host.Host, g ConnectionGater, cfg *rollup.EsConfig, gossipConf GossipSetupConfigurables, m GossipMetricer, lg log.Logger) (*pubsub.PubSub, error) {
	denyList, err := pubsub.NewTimeCachedBlacklist(30 * time.Second)
	if err != nil {
		return nil, err
	}
	gossipOpts := []pubsub.Option{
		pubsub.WithMaxMessageSize(maxGossipSize),
		pubsub.WithMessageIdFn(BuildMsgIdFn(cfg)),
		pubsub.WithNoAuthor(),
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithSubscriptionFilter(BuildSubscriptionFilter(cfg)),
		pubsub.WithValidateQueueSize(maxValidateQueue),
		pubsub.WithPeerOutboundQueueSize(maxOutboundQueue),
		pubsub.WithValidateThrottle(globalValidateThrottle),
		pubsub.WithSeenMessagesTTL(seenMessagesTTL),
		pubsub.WithPeerExchange(false),
		pubsub.WithBlacklist(denyList),
		// pubsub.WithEventTracer(&gossipTracer{m: m}),
	}
	// gossipOpts = append(gossipOpts, ConfigurePeerScoring(h, g, gossipConf, m, lg)...)
	// gossipOpts = append(gossipOpts, gossipConf.ConfigureGossip(cfg)...)
	return pubsub.NewGossipSub(p2pCtx, h, gossipOpts...)
}
