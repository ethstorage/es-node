package p2p

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/conngater"
	cmgr "github.com/libp2p/go-libp2p/p2p/net/connmgr"
)

var DefaultBootnodes = []*enode.Node{
	enode.MustParse("enr:-J24QKTMozsV7vECSF7pqLAefTlVuMzWemnxIcdvsyIpfxWdFJxQs2Z7GnflniDpjeM_xjUpPO7gmsx6hOOIhHvnqimGAYfi6xtbimV0aHN0b3JhZ2XAgmlkgnY0gmlwhMCoAQKJc2VjcDI1NmsxoQN-8fpPc95ilMsRoMs1cRCi-s8kQrsT_cciktg_cUsuNYN0Y3CCJAaDdWRwgnZh"),
}

type P2pSetupConfig interface {
}

type GossipSetupConfigurables interface {
	PeerScoringParams() *pubsub.PeerScoreParams
	TopicScoringParams() *pubsub.TopicScoreParams
	BanPeers() bool
	// ConfigureGossip creates configuration options to apply to the GossipSub setup
	ConfigureGossip(rollupCfg *rollup.EsConfig) []pubsub.Option
	PeerBandScorer() *BandScoreThresholds
}

// SetupP2P provides a host and discovery service for usage in the rollup node.
type SetupP2P interface {
	Check() error
	Disabled() bool
	// Host creates a libp2p host service. Returns nil, nil if p2p is disabled.
	Host(log log.Logger, reporter metrics.Reporter) (host.Host, error)
	// Discovery creates a disc-v5 service. Returns nil, nil, nil if discovery is disabled.
	Discovery(log log.Logger, l1ChainID uint64, tcpPort uint16) (*enode.LocalNode, *discover.UDPv5, error)
	TargetPeers() uint
	GossipSetupConfigurables
}

// Config sets up a p2p host and discv5 service from configuration.
// This implements SetupP2P.
type Config struct {
	Priv *crypto.Secp256k1PrivateKey

	DisableP2P  bool
	NoDiscovery bool

	// Enable P2P-based alt-syncing method (req-resp protocol, not gossip)
	AltSync bool

	// Pubsub Scoring Parameters
	PeerScoring  pubsub.PeerScoreParams
	TopicScoring pubsub.TopicScoreParams

	// Peer Score Band Thresholds
	BandScoreThresholds BandScoreThresholds

	// Whether to ban peers based on their [PeerScoring] score.
	BanningEnabled bool

	ListenIP      net.IP
	ListenTCPPort uint16

	// Port to bind discv5 to
	ListenUDPPort uint16

	AdvertiseIP      net.IP
	AdvertiseTCPPort uint16
	AdvertiseUDPPort uint16
	Bootnodes        []*enode.Node
	DiscoveryDB      *enode.DB

	StaticPeers []core.Multiaddr

	HostMux             []libp2p.Option
	HostSecurity        []libp2p.Option
	NoTransportSecurity bool

	PeersLo    uint
	PeersHi    uint
	PeersGrace time.Duration

	MeshD     int // topic stable mesh target count
	MeshDLo   int // topic stable mesh low watermark
	MeshDHi   int // topic stable mesh high watermark
	MeshDLazy int // gossip target

	// FloodPublish publishes messages from ourselves to peers outside of the gossip topic mesh but supporting the same topic.
	FloodPublish bool

	// If true a NAT manager will host a NAT port mapping that is updated with PMP and UPNP by libp2p/go-nat
	NAT bool

	UserAgent string

	TimeoutNegotiation time.Duration
	TimeoutAccept      time.Duration
	TimeoutDial        time.Duration

	TestSimpleSyncStart uint64
	TestSimpleSyncEnd   uint64

	// Underlying store that hosts connection-gater and peerstore data.
	Store ds.Batching

	ConnGater func(conf *Config) (connmgr.ConnectionGater, error)
	ConnMngr  func(conf *Config) (connmgr.ConnManager, error)
}

//go:generate mockery --name ConnectionGater
type ConnectionGater interface {
	connmgr.ConnectionGater

	// BlockPeer adds a peer to the set of blocked peers.
	// Note: active connections to the peer are not automatically closed.
	BlockPeer(p peer.ID) error
	UnblockPeer(p peer.ID) error
	ListBlockedPeers() []peer.ID

	// BlockAddr adds an IP address to the set of blocked addresses.
	// Note: active connections to the IP address are not automatically closed.
	BlockAddr(ip net.IP) error
	UnblockAddr(ip net.IP) error
	ListBlockedAddrs() []net.IP

	// BlockSubnet adds an IP subnet to the set of blocked addresses.
	// Note: active connections to the IP subnet are not automatically closed.
	BlockSubnet(ipnet *net.IPNet) error
	UnblockSubnet(ipnet *net.IPNet) error
	ListBlockedSubnets() []*net.IPNet
}

func DefaultConnGater(conf *Config) (connmgr.ConnectionGater, error) {
	return conngater.NewBasicConnectionGater(conf.Store)
}

func DefaultConnManager(conf *Config) (connmgr.ConnManager, error) {
	return cmgr.NewConnManager(
		int(conf.PeersLo),
		int(conf.PeersHi),
		cmgr.WithGracePeriod(conf.PeersGrace),
		cmgr.WithSilencePeriod(time.Minute),
		cmgr.WithEmergencyTrim(true))
}

func (conf *Config) TargetPeers() uint {
	return conf.PeersLo
}

func (conf *Config) Disabled() bool {
	return conf.DisableP2P
}

func (conf *Config) PeerScoringParams() *pubsub.PeerScoreParams {
	return &conf.PeerScoring
}

func (conf *Config) PeerBandScorer() *BandScoreThresholds {
	return &conf.BandScoreThresholds
}

func (conf *Config) BanPeers() bool {
	return conf.BanningEnabled
}

func (conf *Config) TopicScoringParams() *pubsub.TopicScoreParams {
	return &conf.TopicScoring
}

const maxMeshParam = 1000

func (conf *Config) Check() error {
	if conf.DisableP2P {
		return nil
	}
	if conf.Store == nil {
		return errors.New("p2p requires a persistent or in-memory peerstore, but found none")
	}
	if !conf.NoDiscovery {
		if conf.DiscoveryDB == nil {
			return errors.New("discovery requires a persistent or in-memory discv5 db, but found none")
		}
	}
	if conf.PeersLo == 0 || conf.PeersHi == 0 || conf.PeersLo > conf.PeersHi {
		return fmt.Errorf("peers lo/hi tides are invalid: %d, %d", conf.PeersLo, conf.PeersHi)
	}
	if conf.ConnMngr == nil {
		return errors.New("need a connection manager")
	}
	if conf.ConnGater == nil {
		return errors.New("need a connection gater")
	}
	if conf.MeshD <= 0 || conf.MeshD > maxMeshParam {
		return fmt.Errorf("mesh D param must not be 0 or exceed %d, but got %d", maxMeshParam, conf.MeshD)
	}
	if conf.MeshDLo <= 0 || conf.MeshDLo > maxMeshParam {
		return fmt.Errorf("mesh Dlo param must not be 0 or exceed %d, but got %d", maxMeshParam, conf.MeshDLo)
	}
	if conf.MeshDHi <= 0 || conf.MeshDHi > maxMeshParam {
		return fmt.Errorf("mesh Dhi param must not be 0 or exceed %d, but got %d", maxMeshParam, conf.MeshDHi)
	}
	if conf.MeshDLazy <= 0 || conf.MeshDLazy > maxMeshParam {
		return fmt.Errorf("mesh Dlazy param must not be 0 or exceed %d, but got %d", maxMeshParam, conf.MeshDLazy)
	}
	return nil
}
