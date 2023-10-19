package p2p

import (
	"github.com/libp2p/go-libp2p/core/network"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethereum/go-ethereum/log"
)

type NotificationsMetricer interface {
	IncPeerCount()
	DecPeerCount()
	IncStreamCount()
	DecStreamCount()
}

type NoopNotificationsMetricer struct {
}

func (*NoopNotificationsMetricer) IncPeerCount() {

}

func (*NoopNotificationsMetricer) DecPeerCount() {

}

func (*NoopNotificationsMetricer) IncStreamCount() {

}

func (*NoopNotificationsMetricer) DecStreamCount() {

}

type notifications struct {
	log log.Logger
	m   NotificationsMetricer
}

func (notif *notifications) Listen(n network.Network, a ma.Multiaddr) {
	notif.log.Info("Started listening network address", "addr", a)
}
func (notif *notifications) ListenClose(n network.Network, a ma.Multiaddr) {
	notif.log.Info("Stopped listening network address", "addr", a)
}
func (notif *notifications) Connected(n network.Network, v network.Conn) {
	notif.m.IncPeerCount()
	notif.log.Info("Connected to peer", "peer", v.RemotePeer(), "addr", v.RemoteMultiaddr())
}
func (notif *notifications) Disconnected(n network.Network, v network.Conn) {
	notif.m.DecPeerCount()
	notif.log.Info("Disconnected from peer", "peer", v.RemotePeer(), "addr", v.RemoteMultiaddr())
}
func (notif *notifications) OpenedStream(n network.Network, v network.Stream) {
	notif.m.IncStreamCount()
	c := v.Conn()
	notif.log.Trace("Opened stream", "protocol", v.Protocol(), "peer", c.RemotePeer(), "addr", c.RemoteMultiaddr())
}
func (notif *notifications) ClosedStream(n network.Network, v network.Stream) {
	notif.m.DecStreamCount()
	c := v.Conn()
	notif.log.Trace("Opened stream", "protocol", v.Protocol(), "peer", c.RemotePeer(), "addr", c.RemoteMultiaddr())
}

func NewNetworkNotifier(log log.Logger, m NotificationsMetricer) network.Notifiee {
	if m == nil {
		m = &NoopNotificationsMetricer{}
	}
	return &notifications{log: log, m: m}
}
