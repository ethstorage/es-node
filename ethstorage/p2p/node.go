package p2p

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/hashicorp/go-multierror"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/host"
	p2pmetrics "github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// NodeP2P is a p2p node, which can be used to gossip messages.
type NodeP2P struct {
	host    host.Host           // p2p host (optional, may be nil)
	gater   ConnectionGater     // p2p gater, to ban/unban peers with, may be nil even with p2p enabled
	connMgr connmgr.ConnManager // p2p conn manager, to keep a reliable number of peers, may be nil even with p2p enabled
	isIPSet bool
	// the below components are all optional, and may be nil. They require the host to not be nil.
	dv5Local       *enode.LocalNode // p2p discovery identity
	dv5Udp         *discover.UDPv5  // p2p discovery service
	gs             *pubsub.PubSub   // p2p gossip router
	syncCl         *protocol.SyncClient
	syncSrv        *protocol.SyncServer
	storageManager *ethstorage.StorageManager
}

// NewNodeP2P creates a new p2p node, and returns a reference to it. If the p2p is disabled, it returns nil.
// If metrics are configured, a bandwidth monitor will be spawned in a goroutine.
func NewNodeP2P(resourcesCtx context.Context, rollupCfg *rollup.EsConfig, l1ChainID uint64, log log.Logger, setup SetupP2P,
	storageManager *ethstorage.StorageManager, db ethdb.Database, m metrics.Metricer, feed *event.Feed) (*NodeP2P, error) {
	if setup == nil {
		return nil, errors.New("p2p node cannot be created without setup")
	}
	var n NodeP2P
	if err := n.init(resourcesCtx, rollupCfg, l1ChainID, log, setup, storageManager, db, m, feed); err != nil {
		closeErr := n.Close()
		if closeErr != nil {
			log.Error("Failed to close p2p after starting with err", "closeErr", closeErr, "err", err)
		}
		return nil, err
	}
	if n.host == nil {
		return nil, nil
	}
	return &n, nil
}

func (n *NodeP2P) init(resourcesCtx context.Context, rollupCfg *rollup.EsConfig, l1ChainID uint64, log log.Logger, setup SetupP2P,
	storageManager *ethstorage.StorageManager, db ethdb.Database, m metrics.Metricer, feed *event.Feed) error {
	bwc := p2pmetrics.NewBandwidthCounter()
	n.storageManager = storageManager

	var err error
	// nil if disabled.
	n.host, err = setup.Host(log, bwc)
	if err != nil {
		if n.dv5Udp != nil {
			n.dv5Udp.Close()
		}
		return fmt.Errorf("failed to start p2p host: %w", err)
	}

	if n.host != nil {
		// Enable extra features, if any. During testing we don't setup the most advanced host all the time.
		if extra, ok := n.host.(ExtraHostFeatures); ok {
			n.gater = extra.ConnectionGater()
			n.connMgr = extra.ConnectionManager()
		}

		// Activate the P2P req-resp sync
		n.syncCl = protocol.NewSyncClient(log, rollupCfg, n.host.NewStream, storageManager, setup.SyncerParams(), db, m, feed)
		n.host.Network().Notify(&network.NotifyBundle{
			ConnectedF: func(nw network.Network, conn network.Conn) {
				var (
					shards       map[common.Address][]uint64
					remotePeerId = conn.RemotePeer()
				)
				if len(n.host.Peerstore().Addrs(remotePeerId)) == 0 {
					// As the node host enable NATService, which will create a new connection with another
					// peer id and its Addrs will not be set to Peerstore, so if len of peer Addrs is 0,
					// then ignore this connection.
					log.Debug("No addresses to get shard list, return without close conn", "peer", remotePeerId)
					return
				}
				css, err := n.Host().Peerstore().Get(remotePeerId, protocol.EthStorageENRKey)
				if err != nil {
					// for node which is new to the ethstorage network, and it dial the nodes which do not contain
					// the new node's enr, so the nodes do not know its shard list from enr, so it needs to call
					// n.RequestShardList to fetch the shard list of the new node.
					remoteShardList, e := n.RequestShardList(remotePeerId)
					if e != nil {
						log.Info("Get remote shard list fail", "peer", remotePeerId, "err", e.Error())
						conn.Close()
						return
					}
					log.Debug("Get remote shard list success", "peer", remotePeerId, "shards", remoteShardList)
					n.Host().Peerstore().Put(remotePeerId, protocol.EthStorageENRKey, remoteShardList)
					shards = protocol.ConvertToShardList(remoteShardList)
				} else {
					shards = protocol.ConvertToShardList(css.([]*protocol.ContractShards))
				}
				added := n.syncCl.AddPeer(remotePeerId, shards, conn.Stat().Direction)
				if !added {
					log.Info("Close connection as AddPeer fail", "peer", remotePeerId)
					conn.Close()
				}
			},
			DisconnectedF: func(nw network.Network, conn network.Conn) {
				if len(n.host.Peerstore().Addrs(conn.RemotePeer())) == 0 {
					log.Debug("No addresses in peer store, return without remove peer", "peer", conn.RemotePeer())
					return
				}
				n.syncCl.RemovePeer(conn.RemotePeer())
			},
		})
		n.syncCl.UpdateMaxPeers(int(setup.(*Config).PeersHi))
		// the host may already be connected to peers, add them all to the sync client
		for _, conn := range n.host.Network().Conns() {
			shards := make(map[common.Address][]uint64)
			css, err := n.host.Peerstore().Get(conn.RemotePeer(), protocol.EthStorageENRKey)
			if err != nil {
				log.Debug("Get shards from peer failed", "peer", conn.RemotePeer(), "error", err.Error())
				continue
			} else {
				shards = protocol.ConvertToShardList(css.([]*protocol.ContractShards))
			}
			added := n.syncCl.AddPeer(conn.RemotePeer(), shards, conn.Stat().Direction)
			if !added {
				conn.Close()
			}
		}
		go n.syncCl.ReportPeerSummary()
		n.syncSrv = protocol.NewSyncServer(rollupCfg, storageManager, m)

		blobByRangeHandler := protocol.MakeStreamHandler(resourcesCtx, log.New("serve", "blobs_by_range"), n.syncSrv.HandleGetBlobsByRangeRequest)
		n.host.SetStreamHandler(protocol.GetProtocolID(protocol.RequestBlobsByRangeProtocolID, rollupCfg.L2ChainID), blobByRangeHandler)
		blobByListHandler := protocol.MakeStreamHandler(resourcesCtx, log.New("serve", "blobs_by_list"), n.syncSrv.HandleGetBlobsByListRequest)
		n.host.SetStreamHandler(protocol.GetProtocolID(protocol.RequestBlobsByListProtocolID, rollupCfg.L2ChainID), blobByListHandler)
		requestShardListHandler := protocol.MakeStreamHandler(resourcesCtx, log.New("serve", "get_shard_list"), n.syncSrv.HandleRequestShardList)
		n.host.SetStreamHandler(protocol.RequestShardList, requestShardListHandler)

		// notify of any new connections/streams/etc.
		// TODO: use metric
		n.host.Network().Notify(NewNetworkNotifier(log, nil))
		// note: the IDDelta functionality was removed from libP2P, and no longer needs to be explicitly disabled.
		n.gs, err = NewGossipSub(resourcesCtx, n.host, n.gater, rollupCfg, setup, m, log)
		if err != nil {
			return fmt.Errorf("failed to start gossipsub router: %w", err)
		}

		log.Info("Started p2p host", "addrs", n.host.Addrs(), "peerID", n.host.ID().String(), "targetPeers", setup.TargetPeers())

		tcpPort, err := FindActiveTCPPort(n.host)
		if err != nil {
			log.Warn("Failed to find what TCP port p2p is binded to", "err", err)
		}

		// All nil if disabled.
		n.dv5Local, n.dv5Udp, n.isIPSet, err = setup.Discovery(log.New("p2p", "discv5"), l1ChainID, tcpPort, getLocalPublicIPv4())
		if err != nil {
			return fmt.Errorf("failed to start discv5: %w", err)
		}

		if m != nil {
			go m.RecordBandwidth(resourcesCtx, bwc)
		}
	}
	return nil
}

func (n *NodeP2P) RequestL2Range(ctx context.Context, start, end uint64) (uint64, error) {
	return n.syncCl.RequestL2Range(start, end)
}

// RequestShardList fetches shard list from remote peer
func (n *NodeP2P) RequestShardList(remotePeer peer.ID) ([]*protocol.ContractShards, error) {
	remoteShardList := make([]*protocol.ContractShards, 0)
	ctx, _ := context.WithTimeout(context.Background(), protocol.NewStreamTimeout)
	s, err := n.Host().NewStream(ctx, remotePeer, protocol.RequestShardList)
	if err != nil {
		return remoteShardList, err
	}
	defer func() {
		if s != nil {
			s.Close()
		}
	}()

	code, err := protocol.SendRPC(s, make([]byte, 0), &remoteShardList)
	if err != nil {
		return remoteShardList, err
	}
	if code != 0 {
		return remoteShardList, fmt.Errorf("request shard list fail, code %d", code)
	}

	return remoteShardList, nil
}

func (n *NodeP2P) Host() host.Host {
	return n.host
}

func (n *NodeP2P) Dv5Local() *enode.LocalNode {
	return n.dv5Local
}

func (n *NodeP2P) Dv5Udp() *discover.UDPv5 {
	return n.dv5Udp
}

func (n *NodeP2P) ConnectionManager() connmgr.ConnManager {
	return n.connMgr
}

func (n *NodeP2P) Start() error {
	if n.syncCl != nil {
		return n.syncCl.Start()
	}
	return nil
}

func (n *NodeP2P) Close() error {
	var result *multierror.Error
	if n.dv5Udp != nil {
		n.dv5Udp.Close()
	}
	// if n.gsOut != nil {
	// 	if err := n.gsOut.Close(); err != nil {
	// 		result = multierror.Append(result, fmt.Errorf("failed to close gossip cleanly: %w", err))
	// 	}
	// }
	if n.host != nil {
		if err := n.host.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close p2p host cleanly: %w", err))
		}
		if n.syncCl != nil {
			if err := n.syncCl.Close(); err != nil {
				result = multierror.Append(result, fmt.Errorf("failed to close p2p sync client cleanly: %w", err))
			}
		}
	}
	return result.ErrorOrNil()
}

func FindActiveTCPPort(h host.Host) (uint16, error) {
	var tcpPort uint16
	for _, addr := range h.Addrs() {
		tcpPortStr, err := addr.ValueForProtocol(ma.P_TCP)
		if err != nil {
			continue
		}
		v, err := strconv.ParseUint(tcpPortStr, 10, 16)
		if err != nil {
			continue
		}
		tcpPort = uint16(v)
		break
	}
	return tcpPort, nil
}

func getLocalPublicIPv4() net.IP {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		log.Debug("getLocalPublicIPv4 fail", "err", err.Error())
		return nil
	}

	for _, addr := range addresses {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.To4() == nil {
			continue
		}
		if ipnet.IP.IsGlobalUnicast() && !ipnet.IP.IsPrivate() {
			return ipnet.IP.To4()
		}
	}
	return nil
}
