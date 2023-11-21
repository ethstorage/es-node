package p2p

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	lconf "github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/sec/insecure"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoreds"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	tls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"

	"github.com/ethereum/go-ethereum/log"
)

type ExtraHostFeatures interface {
	host.Host
	ConnectionGater() ConnectionGater
	ConnectionManager() connmgr.ConnManager
}

type extraHost struct {
	host.Host
	gater   ConnectionGater
	connMgr connmgr.ConnManager
	log     log.Logger

	staticPeers []*peer.AddrInfo

	quitC chan struct{}
}

func (e *extraHost) ConnectionGater() ConnectionGater {
	return e.gater
}

func (e *extraHost) ConnectionManager() connmgr.ConnManager {
	return e.connMgr
}

func (e *extraHost) Close() error {
	close(e.quitC)
	return e.Host.Close()
}

func (e *extraHost) initStaticPeers() {
	for _, addr := range e.staticPeers {
		e.Peerstore().AddAddrs(addr.ID, addr.Addrs, time.Hour*24*7)
		// We protect the peer, so the connection manager doesn't decide to prune it.
		// We tag it with "static" so other protects/unprotects with different tags don't affect this protection.
		e.connMgr.Protect(addr.ID, "static")
		// Try to dial the node in the background
		go func(addr *peer.AddrInfo) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
			defer cancel()
			if err := e.dialStaticPeer(ctx, addr); err != nil {
				e.log.Warn("Error dialing static peer", "peer", addr.ID, "err", err)
			}
		}(addr)
	}
}

func (e *extraHost) dialStaticPeer(ctx context.Context, addr *peer.AddrInfo) error {
	e.log.Info("Dialing static peer", "peer", addr.ID, "addrs", addr.Addrs)
	if _, err := e.Network().DialPeer(ctx, addr.ID); err != nil {
		return err
	}
	return nil
}

func (e *extraHost) monitorStaticPeers() {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
			var wg sync.WaitGroup

			e.log.Debug("Polling static peers", "peers", len(e.staticPeers))
			for _, addr := range e.staticPeers {
				connectedness := e.Network().Connectedness(addr.ID)
				e.log.Trace("Static peer connectedness", "peer", addr.ID, "connectedness", connectedness)

				if connectedness == network.Connected {
					continue
				}

				wg.Add(1)
				go func(addr *peer.AddrInfo) {
					e.log.Warn("Static peer disconnected, reconnecting", "peer", addr.ID)
					if err := e.dialStaticPeer(ctx, addr); err != nil {
						e.log.Warn("Error reconnecting to static peer", "peer", addr.ID, "err", err)
					}
					wg.Done()
				}(addr)
			}

			wg.Wait()
			cancel()
		case <-e.quitC:
			return
		}
	}
}

var _ ExtraHostFeatures = (*extraHost)(nil)

func (conf *Config) Host(log log.Logger, reporter metrics.Reporter) (host.Host, error) {
	if conf.DisableP2P {
		return nil, nil
	}
	pub := conf.Priv.GetPublic()
	pid, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("failed to derive pubkey from network priv key: %w", err)
	}

	ps, err := pstoreds.NewPeerstore(context.Background(), conf.Store, pstoreds.DefaultOpts())
	if err != nil {
		return nil, fmt.Errorf("failed to open peerstore: %w", err)
	}

	if err := ps.AddPrivKey(pid, conf.Priv); err != nil {
		return nil, fmt.Errorf("failed to set up peerstore with priv key: %w", err)
	}
	if err := ps.AddPubKey(pid, pub); err != nil {
		return nil, fmt.Errorf("failed to set up peerstore with pub key: %w", err)
	}

	connGtr, err := conf.ConnGater(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection gater: %w", err)
	}

	connMngr, err := conf.ConnMngr(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection manager: %w", err)
	}

	listenAddr, err := addrFromIPAndPort(conf.ListenIP, conf.ListenTCPPort)
	if err != nil {
		return nil, fmt.Errorf("failed to make listen addr: %w", err)
	}
	tcpTransport := libp2p.Transport(
		tcp.NewTCPTransport,
		tcp.WithConnectionTimeout(time.Minute*60)) // break unused connections
	if err != nil {
		return nil, fmt.Errorf("failed to create TCP transport: %w", err)
	}
	// TODO: technically we can also run the node on websocket and QUIC transports. Maybe in the future?

	var nat lconf.NATManagerC // disabled if nil
	if conf.NAT {
		nat = basichost.NewNATManager
	}

	opts := []libp2p.Option{
		libp2p.Identity(conf.Priv),
		// Explicitly set the user-agent, so we can differentiate from other Go libp2p users.
		libp2p.UserAgent(conf.UserAgent),
		tcpTransport,
		libp2p.WithDialTimeout(conf.TimeoutDial),
		// No relay services, direct connections between peers only.
		libp2p.DisableRelay(),
		// host will start and listen to network directly after construction from config.
		libp2p.ListenAddrs(listenAddr),
		libp2p.ConnectionGater(connGtr),
		libp2p.ConnectionManager(connMngr),
		// libp2p.ResourceManager(nil), // TODO use resource manager interface to manage resources per peer better.
		libp2p.NATManager(nat),
		libp2p.Peerstore(ps),
		libp2p.BandwidthReporter(reporter), // may be nil if disabled
		libp2p.MultiaddrResolver(madns.DefaultResolver),
		// Ping is a small built-in libp2p protocol that helps us check/debug latency between peers.
		libp2p.Ping(true),
		// Help peers with their NAT reachability status, but throttle to avoid too much work.
		libp2p.EnableNATService(),
		libp2p.AutoNATServiceRateLimit(10, 5, time.Second*60),
	}
	opts = append(opts, conf.HostMux...)
	if conf.NoTransportSecurity {
		opts = append(opts, libp2p.Security(insecure.ID, insecure.NewWithIdentity))
	} else {
		opts = append(opts, conf.HostSecurity...)
	}
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	staticPeers := make([]*peer.AddrInfo, len(conf.StaticPeers))
	for i, peerAddr := range conf.StaticPeers {
		addr, err := peer.AddrInfoFromP2pAddr(peerAddr)
		if err != nil {
			return nil, fmt.Errorf("bad peer address: %w", err)
		}
		staticPeers[i] = addr
	}

	out := &extraHost{
		Host:        h,
		connMgr:     connMngr,
		log:         log,
		staticPeers: staticPeers,
		quitC:       make(chan struct{}),
	}
	out.initStaticPeers()
	if len(conf.StaticPeers) > 0 {
		go out.monitorStaticPeers()
	}

	// Only add the connection gater if it offers the full interface we're looking for.
	if g, ok := connGtr.(ConnectionGater); ok {
		out.gater = g
	}
	return out, nil
}

// Creates a multi-addr to bind to. Does not contain a PeerID component (required for usage by external peers)
func addrFromIPAndPort(ip net.IP, port uint16) (ma.Multiaddr, error) {
	ipScheme := "ip4"
	if ip4 := ip.To4(); ip4 == nil {
		ipScheme = "ip6"
	} else {
		ip = ip4
	}
	return ma.NewMultiaddr(fmt.Sprintf("/%s/%s/tcp/%d", ipScheme, ip.String(), port))
}

func YamuxC() libp2p.Option {
	return libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport)
}


func NoiseC() libp2p.Option {
	return libp2p.Security(noise.ID, noise.New)
}

func TlsC() libp2p.Option {
	return libp2p.Security(tls.ID, tls.New)
}
