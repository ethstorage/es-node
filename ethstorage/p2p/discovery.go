package p2p

import (
	"context"
	secureRand "crypto/rand"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"time"

	decredSecp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	gcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	discoverIntervalFast   = time.Second * 5
	discoverIntervalSlow   = time.Second * 20
	connectionIntervalFast = time.Second * 5
	connectionIntervalSlow = time.Second * 20
	connectionWorkerCount  = 4
	connectionBufferSize   = 10
	discoveredNodesBuffer  = 3
	tableKickoffDelay      = time.Second * 3
	discoveredAddrTTL      = time.Hour * 24
	collectiveDialTimeout  = time.Second * 30
	// if the node do not have AdvertiseIP and public ip (like run in the docker or behind a nat), we can get the public IP
	// and the TCP port from the libp2p.host's addresses (get from the nat service or address observed by peers) and set
	// to the node discovery. As the host's addresses would not show when it start, so we will check it every initLocalNodeAddrInterval.
	// After the public IP and the TCP port initialized, we will also refresh the address in case the address change.
	initLocalNodeAddrInterval    = time.Second * 30
	refreshLocalNodeAddrInterval = time.Minute * 10
	p2pVersion                   = 0
)

func (conf *Config) Discovery(log log.Logger, l1ChainID uint64, tcpPort uint16, fallbackIP net.IP) (*enode.LocalNode, *discover.UDPv5, bool, error) {
	isIPSet := false
	if conf.NoDiscovery {
		return nil, nil, isIPSet, nil
	}

	priv := (*decredSecp.PrivateKey)(conf.Priv).ToECDSA()
	// use the geth curve definition. Same crypto, but geth needs to detect it as *their* definition of the curve.
	priv.Curve = gcrypto.S256()
	localNode := enode.NewLocalNode(conf.DiscoveryDB, priv)
	if conf.AdvertiseIP != nil {
		localNode.SetStaticIP(conf.AdvertiseIP)
		isIPSet = true
	} else if fallbackIP != nil {
		localNode.SetFallbackIP(fallbackIP)
		isIPSet = true
	}
	if conf.AdvertiseUDPPort != 0 { // explicitly advertised port gets priority
		localNode.SetFallbackUDP(int(conf.AdvertiseUDPPort))
	} else if conf.ListenUDPPort != 0 { // otherwise default to the port we configured it to listen on
		localNode.SetFallbackUDP(int(conf.ListenUDPPort))
	}
	if conf.AdvertiseTCPPort != 0 { // explicitly advertised port gets priority
		localNode.Set(enr.TCP(conf.AdvertiseTCPPort))
	} else if tcpPort != 0 { // otherwise try to pick up whatever port LibP2P binded to (listen port, or dynamically picked)
		localNode.Set(enr.TCP(tcpPort))
	} else if conf.ListenTCPPort != 0 { // otherwise default to the port we configured it to listen on
		localNode.Set(enr.TCP(conf.ListenTCPPort))
	} else {
		return nil, nil, isIPSet, fmt.Errorf("no TCP port to put in discovery record")
	}
	dat := protocol.EthStorageENRData{
		L1ChainID: l1ChainID,
		Version:   p2pVersion,
		Shards:    protocol.ConvertToContractShards(ethstorage.Shards()),
	}
	localNode.Set(&dat)
	// put shards info to Peerstore PeerMetadata, shards struct ([]*ContractShards) need to
	// register like gob.Register(dat.Shards)
	gob.Register(dat.Shards)

	udpAddr := &net.UDPAddr{
		IP:   conf.ListenIP,
		Port: int(conf.ListenUDPPort),
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, nil, isIPSet, err
	}
	if udpAddr.Port == 0 { // if we picked a port dynamically, then find the port we got, and update our node record
		localUDPAddr := conn.LocalAddr().(*net.UDPAddr)
		localNode.SetFallbackUDP(localUDPAddr.Port)
	}

	cfg := discover.Config{
		PrivateKey:   priv,
		NetRestrict:  nil,
		Bootnodes:    conf.Bootnodes,
		Unhandled:    nil, // Not used in dv5
		Log:          log,
		ValidSchemes: enode.ValidSchemes,
	}
	udpV5, err := discover.ListenV5(conn, localNode, cfg)
	if err != nil {
		return nil, nil, isIPSet, err
	}

	log.Info("Started discovery service", "enr", localNode.Node(), "id", localNode.ID(), "port", udpV5.Self().UDP())

	// TODO: periodically we can pull the external IP and TCP port from libp2p NAT service,
	// and add it as a statement to keep the localNode accurate (if we trust the NAT device more than the discv5 statements)

	return localNode, udpV5, isIPSet, nil
}

func updateLocalNodeIPAndTCP(addrs []ma.Multiaddr, localNode *enode.LocalNode) bool {
	for _, addr := range addrs {
		ipStr, err := addr.ValueForProtocol(4)
		if err != nil {
			continue
		}
		ip := net.ParseIP(ipStr)
		if ip.IsPrivate() || !ip.IsGlobalUnicast() {
			continue
		}
		tcpStr, err := addr.ValueForProtocol(ma.P_TCP)
		if err != nil {
			continue
		}
		tcpPort, _ := strconv.Atoi(tcpStr)
		localNode.SetFallbackIP(ip)
		localNode.Set(enr.TCP(tcpPort))
		log.Debug("update LocalNode IP and TCP", "IP", localNode.Node().IP(), "tcp", localNode.Node().TCP())
		return true
	}
	return false
}

// Secp256k1 is like the geth Secp256k1 enr entry type, but using the libp2p pubkey representation instead
type Secp256k1 crypto.Secp256k1PublicKey

func (v Secp256k1) ENRKey() string { return "secp256k1" }

// EncodeRLP implements rlp.Encoder.
func (v Secp256k1) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, (*decredSecp.PublicKey)(&v).SerializeCompressed())
}

// DecodeRLP implements rlp.Decoder.
func (v *Secp256k1) DecodeRLP(s *rlp.Stream) error {
	buf, err := s.Bytes()
	if err != nil {
		return err
	}
	pk, err := decredSecp.ParsePubKey(buf)
	if err != nil {
		return err
	}
	*v = (Secp256k1)(*pk)
	return nil
}

func enrToAddrInfo(r *enode.Node) (*peer.AddrInfo, *crypto.Secp256k1PublicKey, error) {
	ip := r.IP()
	ipScheme := "ip4"
	if ip4 := ip.To4(); ip4 == nil {
		ipScheme = "ip6"
	} else {
		ip = ip4
	}
	mAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/%s/%s/tcp/%d", ipScheme, ip.String(), r.TCP()))
	if err != nil {
		return nil, nil, fmt.Errorf("could not construct multi addr: %w", err)
	}
	var enrPub Secp256k1
	if err := r.Load(&enrPub); err != nil {
		return nil, nil, fmt.Errorf("failed to load pubkey as libp2p pubkey type from ENR")
	}
	pub := (*crypto.Secp256k1PublicKey)(&enrPub)
	peerID, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return nil, pub, fmt.Errorf("could not compute peer ID from pubkey for multi-addr: %w", err)
	}
	return &peer.AddrInfo{
		ID:    peerID,
		Addrs: []multiaddr.Multiaddr{mAddr},
	}, pub, nil
}

func FilterEnodes(log log.Logger, l1ChainID uint64) func(node *enode.Node) bool {
	return func(node *enode.Node) bool {
		var dat protocol.EthStorageENRData
		err := node.Load(&dat)
		// if the entry does not exist, or if it is invalid, then ignore the node
		if err != nil {
			log.Trace("Discovered node record has no ethstorage info", "node", node.ID(), "err", err)
			return false
		}
		// check chain ID matches
		if l1ChainID != dat.L1ChainID {
			log.Trace("Discovered node record has no matching chain ID", "node", node.ID(), "got", dat.L1ChainID, "expected", l1ChainID)
			return false
		}
		// check Version matches
		if dat.Version != p2pVersion {
			log.Trace("Discovered node record has no matching Version", "node", node.ID(), "got", dat.Version, "expected", p2pVersion)
			return false
		}
		shards := ethstorage.Shards()
		for _, cs := range dat.Shards {
			ss, ok := shards[cs.Contract]
			if !ok {
				continue
			}
			for _, sid := range ss {
				for _, rsid := range cs.ShardIds {
					if sid == rsid {
						return true
					}
				}
			}
		}
		return false
	}
}

// DiscoveryProcess runs a discovery process that randomly walks the DHT to fill the peerstore,
// and connects to nodes in the peerstore that we are not already connected to.
// Nodes from the peerstore will be shuffled, unsuccessful connection attempts will cause peers to be avoided,
// and only nodes with addresses (under TTL) will be connected to.
func (n *NodeP2P) DiscoveryProcess(ctx context.Context, log log.Logger, l1ChainID uint64, connectGoal uint) {
	if n.dv5Udp == nil {
		log.Warn("Peer discovery is disabled")
		return
	}
	filter := FilterEnodes(log, l1ChainID)
	// We pull nodes from discv5 DHT in random order to find new peers.
	// Eventually we'll find a peer record that matches our filter.
	randomNodeIter := n.dv5Udp.RandomNodes()

	randomNodeIter = enode.Filter(randomNodeIter, filter)
	defer randomNodeIter.Close()

	// We pull from the DHT in a slow/fast interval, depending on the need to find more peers
	discoverTicker := time.NewTicker(discoverIntervalFast)
	defer discoverTicker.Stop()

	// We connect to the peers we know of to maintain a target,
	// but do so with polling to avoid scanning the connection count continuously
	connectTicker := time.NewTicker(connectionIntervalFast)
	defer connectTicker.Stop()

	// We can go faster/slower depending on the need
	slower := func() {
		discoverTicker.Reset(discoverIntervalSlow)
		connectTicker.Reset(connectionIntervalSlow)
	}
	faster := func() {
		discoverTicker.Reset(discoverIntervalFast)
		connectTicker.Reset(connectionIntervalFast)
	}

	// We try to connect to peers in parallel: some may be slow to respond
	connAttempts := make(chan peer.ID, connectionBufferSize)
	connectWorker := func(ctx context.Context) {
		for {
			id, ok := <-connAttempts
			if !ok {
				return
			}
			addrs := n.Host().Peerstore().Addrs(id)
			log.Debug("Attempting connection", "peer", id, "Addr", addrs)
			ctx, cancel := context.WithTimeout(ctx, time.Second*10)
			err := n.Host().Connect(ctx, peer.AddrInfo{ID: id, Addrs: addrs})
			cancel()
			if err != nil {
				log.Debug("Failed connection attempt", "peer", id, "Addr", addrs, "err", err)
			}
		}
	}

	go func() {
		if n.isIPSet {
			return
		}
		updateLocalNodeTicker := time.NewTicker(initLocalNodeAddrInterval)
		initialized := false
		defer updateLocalNodeTicker.Stop()
		for {
			select {
			case <-updateLocalNodeTicker.C:
				if updateLocalNodeIPAndTCP(n.host.Addrs(), n.dv5Local) && !initialized {
					initialized = true
					updateLocalNodeTicker.Reset(refreshLocalNodeAddrInterval)
					log.Info("Update local TCP IP address", "ip", n.dv5Local.Node().IP(), "udp", n.dv5Local.Node().UDP(),
						"tcp", n.dv5Local.Node().TCP(), "seq", n.dv5Local.Seq(), "enr", n.dv5Local.Node().String())
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// stops all the workers when we are done
	defer close(connAttempts)
	// start workers to try connect to peers
	for i := 0; i < connectionWorkerCount; i++ {
		go connectWorker(ctx)
	}

	// buffer discovered nodes, so don't stall on the dht iteration as much
	randomNodesCh := make(chan *enode.Node, discoveredNodesBuffer)
	defer close(randomNodesCh)
	bufferNodes := func() {
		for {
			select {
			case <-discoverTicker.C:
				if !randomNodeIter.Next() {
					log.Info("Discv5 DHT iteration stopped, closing peer discovery now...")
					return
				}
				found := randomNodeIter.Node()
				select {
				// block once we have found enough nodes
				case randomNodesCh <- found:
					continue
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}
	// Walk the DHT in parallel, the discv5 interface does not use channels for the iteration
	go bufferNodes()

	// Kick off by trying the nodes we have in our table (previous nodes from last run and/or bootnodes)
	go func() {
		<-time.After(tableKickoffDelay)
		// At the start we might have trouble walking the DHT,
		// but we do have a table with some nodes,
		// so take the table and feed it into the discovery process
		for _, rec := range n.dv5Udp.AllNodes() {
			if filter(rec) {
				select {
				case randomNodesCh <- rec:
					continue
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	pstore := n.Host().Peerstore()
	for {
		select {
		case <-ctx.Done():
			log.Info("Stopped peer discovery")
			return // no ctx error, expected close
		case found := <-randomNodesCh:
			// get the most recent version of the node record in case it change deal to the remote node TCP or UDP port change.
			node := n.dv5Udp.Resolve(found)
			if node.Seq() != found.Seq() {
				log.Debug("Remote node ENR changed", "ID", node.ID(), "remote IP", node.IP(), "ENR", node.String())
			}
			var dat protocol.EthStorageENRData
			if err := node.Load(&dat); err != nil { // we already filtered on chain ID and Version
				continue
			}
			info, pub, err := enrToAddrInfo(node)
			if err != nil {
				continue
			}
			if dat.L1ChainID != l1ChainID {
				continue
			}
			// We add the addresses to the peerstore, and update the address TTL.
			// After that we stop using the address, assuming it may not be valid anymore (until we rediscover the node)
			pstore.AddAddrs(info.ID, info.Addrs, discoveredAddrTTL)
			err = pstore.Put(info.ID, protocol.EthStorageENRKey, dat.Shards)
			if err != nil {
				log.Info("Peerstore put EthStorageENRKey error", "err", err.Error())
				continue
			}
			_ = pstore.AddPubKey(info.ID, pub)
			// Tag the peer, we'd rather have the connection manager prune away old peers,
			// or peers on different chains, or anyone we have not seen via discovery.
			// There is no tag score decay yet, so just set it to 42.
			n.ConnectionManager().TagPeer(info.ID, fmt.Sprintf("ethstorage-%d-%d", dat.L1ChainID, dat.Version), 42)
			log.Debug("Discovered peer", "peer", info.ID, "nodeID", node.ID(), "addr", info.Addrs[0])
		case <-connectTicker.C:
			connected := n.Host().Network().Peers()
			log.Debug("Peering tick", "connected", len(connected),
				"advertisedUdp", n.dv5Local.Node().UDP(),
				"advertisedTcp", n.dv5Local.Node().TCP(),
				"advertisedIP", n.dv5Local.Node().IP())
			if uint(len(connected)) < connectGoal {
				// Start looking for more peers more actively again
				faster()

				peersWithAddrs := n.Host().Peerstore().PeersWithAddrs()
				if err := shufflePeers(peersWithAddrs); err != nil {
					continue
				}

				existing := make(map[peer.ID]struct{})
				for _, p := range connected {
					existing[p] = struct{}{}
				}

				// Keep using these peers, and don't try new discovery/connections.
				// We don't need to search for more peers and try new connections if we already have plenty
				ctx, cancel := context.WithTimeout(ctx, collectiveDialTimeout)
			peerLoop:
				for _, id := range peersWithAddrs {
					// never dial ourselves
					if n.Host().ID() == id {
						continue
					}
					// skip peers that we are already connected to
					if _, ok := existing[id]; ok {
						continue
					}
					// skip peers that we were just connected to
					if n.Host().Network().Connectedness(id) == network.CannotConnect {
						continue
					}
					// schedule, if there is still space to schedule (this may block)
					select {
					case connAttempts <- id:
					case <-ctx.Done():
						break peerLoop
					}
				}
				cancel()
			} else {
				// we have enough connections, slow down actively filling the peerstore
				slower()
			}
		}
	}
}

// shuffle the slice of peer IDs in-place with a RNG seeded by secure randomness.
func shufflePeers(ids peer.IDSlice) error {
	var x [8]byte // shuffling is not critical, just need to avoid basic predictability by outside peers
	if _, err := io.ReadFull(secureRand.Reader, x[:]); err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(x[:]))))
	rng.Shuffle(len(ids), ids.Swap)
	return nil
}
