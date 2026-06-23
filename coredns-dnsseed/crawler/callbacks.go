package crawler

import (
	"github.com/modelos/modelos/node/peer"
	"github.com/modelos/modelos/node/wire"
)

func (s *Seeder) onVerAck(p *peer.Peer, msg *wire.MsgVerAck) {
	pk := peerKeyFromPeer(p)

	sig, ok := s.promotePeer(pk, p)
	if !ok {
		s.logger.Printf("Got verack from unexpected peer %s", p.Addr())
		return
	}

	sig <- struct{}{}

	if s.addrBook.IsKnown(pk) {
		s.addrBook.Touch(pk)
	}
}

// promotePeer atomically moves a peer from pending to live and returns its
// signal channel. Returns false if the peer is not in pendingPeers or has
// no signal channel.
func (s *Seeder) promotePeer(pk PeerKey, p *peer.Peer) (chan struct{}, bool) {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()

	if _, ok := s.pendingPeers[pk]; !ok {
		return nil, false
	}

	sig, hasSig := s.handshakeSignals[pk]
	if !hasSig {
		return nil, false
	}

	s.livePeers[pk] = p
	delete(s.pendingPeers, pk)
	delete(s.handshakeSignals, pk)
	return sig, true
}

func (s *Seeder) onAddr(p *peer.Peer, msg *wire.MsgAddr) {
	if len(msg.AddrList) == 0 {
		s.logger.Printf("Got empty addr from peer %s, disconnecting", p.Addr())
		s.DisconnectPeer(peerKeyFromPeer(p))
		return
	}

	s.logger.Printf("Got %d addrs from peer %s", len(msg.AddrList), p.Addr())

	for _, na := range msg.AddrList {
		if s.addrBook.IsKnown(peerKeyFromNA(na)) {
			continue
		}
		s.addrQueue <- na
	}
}

// onAddrV2 handles addrv2 messages, converting IPv4/IPv6 entries to legacy
// NetAddress for the address queue. Tor, I2P, and CJDNS addresses are skipped.
func (s *Seeder) onAddrV2(p *peer.Peer, msg *wire.MsgAddrV2) {
	if len(msg.AddrList) == 0 {
		s.logger.Printf("Got empty addrv2 from peer %s, disconnecting", p.Addr())
		s.DisconnectPeer(peerKeyFromPeer(p))
		return
	}

	s.logger.Printf("Got %d addrv2s from peer %s", len(msg.AddrList), p.Addr())

	for _, nav2 := range msg.AddrList {
		legacy := nav2.ToLegacy()
		if legacy == nil {
			continue
		}
		pk, ok := peerKeyFromNAV2(nav2)
		if !ok {
			continue
		}
		if s.addrBook.IsKnown(pk) {
			continue
		}
		s.addrQueue <- legacy
	}
}
