package crawler

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelos/modelos/node/addrmgr"
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/peer"
	"github.com/modelos/modelos/node/wire"
)

// PeerKey is a "host:port" string that uniquely identifies a peer.
type PeerKey string

func (p PeerKey) String() string {
	return string(p)
}

func peerKeyFromPeer(p *peer.Peer) PeerKey {
	return PeerKey(p.Addr())
}

func peerKeyFromNA(na *wire.NetAddress) PeerKey {
	return PeerKey(net.JoinHostPort(na.IP.String(), strconv.Itoa(int(na.Port))))
}

// peerKeyFromNAV2 extracts a PeerKey from a v2 network address. It returns
// the key and true if the address is IPv4 or IPv6, or an empty key and false
// for unsupported address types (tor, i2p, cjdns).
func peerKeyFromNAV2(na *wire.NetAddressV2) (PeerKey, bool) {
	legacy := na.ToLegacy()
	if legacy == nil {
		return "", false
	}
	return peerKeyFromNA(legacy), true
}

var (
	ErrRepeatConnection = errors.New("attempted repeat connection to existing peer")
	ErrNoSuchPeer       = errors.New("no record of requested peer")
	ErrAddressTimeout   = errors.New("wait for addresses timed out")
	ErrBlacklistedPeer  = errors.New("peer is blacklisted")
	ErrUnknownNetwork   = errors.New("unknown network name")
)

func newDefaultPeerConfig() peer.Config {
	return peer.Config{
		UserAgentName:    "pearl-seeder",
		UserAgentVersion: "0.1.0",
		Services:         wire.SFNodeP2PV2,
		TrickleInterval:  10 * time.Second,
		ProtocolVersion:  wire.ProtocolVersion,
	}
}

var (
	minimumReadyAddresses = 10
	maximumHandshakeWait  = 5 * time.Second
	connectionDialTimeout = 5 * time.Second
	crawlerThreadTimeout  = 30 * time.Second
	crawlerGoroutineCount = runtime.NumCPU() * 32
	addrQueueBufferSize   = 4096
	blacklistDropTime     = 3 * 24 * time.Hour
)

// Seeder discovers Pearl peers and maintains an address book for DNS serving.
type Seeder struct {
	config *peer.Config
	logger *log.Logger

	// peersMu guards pendingPeers, livePeers, and handshakeSignals.
	// A single mutex covers all three because onVerAck atomically
	// transitions a peer from pending to live and signals completion.
	peersMu          sync.RWMutex
	pendingPeers     map[PeerKey]*peer.Peer
	livePeers        map[PeerKey]*peer.Peer
	handshakeSignals map[PeerKey]chan struct{}

	addrBook  *AddressBook
	addrQueue chan *wire.NetAddress
}

// NewSeeder creates a Seeder configured for the given network name.
func NewSeeder(networkName string) (*Seeder, error) {
	params, err := networkParams(networkName)
	if err != nil {
		return nil, err
	}
	cfg := newDefaultPeerConfig()
	cfg.ChainParams = params

	sink, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0666)
	logger := log.New(sink, "pearl-seeder: ", log.Ldate|log.Ltime|log.Lshortfile|log.LUTC)

	s := &Seeder{
		config:           &cfg,
		logger:           logger,
		pendingPeers:     make(map[PeerKey]*peer.Peer),
		livePeers:        make(map[PeerKey]*peer.Peer),
		handshakeSignals: make(map[PeerKey]chan struct{}),
		addrBook:         NewAddressBook(),
		addrQueue:        make(chan *wire.NetAddress, addrQueueBufferSize),
	}

	s.config.Listeners.OnVerAck = s.onVerAck
	s.config.Listeners.OnAddr = s.onAddr
	s.config.Listeners.OnAddrV2 = s.onAddrV2

	return s, nil
}

func networkParams(name string) (*chaincfg.Params, error) {
	switch name {
	case "mainnet":
		return &chaincfg.MainNetParams, nil
	case "testnet":
		return &chaincfg.TestNetParams, nil
	case "testnet2":
		return &chaincfg.TestNet2Params, nil
	case "regtest":
		return &chaincfg.RegressionNetParams, nil
	case "signet":
		return &chaincfg.SigNetParams, nil
	case "simnet":
		return &chaincfg.SimNetParams, nil
	default:
		return nil, ErrUnknownNetwork
	}
}

// GetNetworkDefaultPort returns the default P2P port for the configured network.
func (s *Seeder) GetNetworkDefaultPort() string {
	return s.config.ChainParams.DefaultPort
}

// ConnectOnDefaultPort connects to a peer at the given address on the
// network's default port.
func (s *Seeder) ConnectOnDefaultPort(addr string) error {
	_, err := s.Connect(addr, s.config.ChainParams.DefaultPort)
	return err
}

// Connect establishes an outbound v2 encrypted connection and completes the
// version handshake. Returns the connected peer or an error.
func (s *Seeder) Connect(addr, port string) (*peer.Peer, error) {
	host := net.JoinHostPort(addr, port)

	if s.addrBook.IsBlacklisted(PeerKey(host)) {
		return nil, ErrBlacklistedPeer
	}

	cfg := *s.config

	p, err := peer.NewOutboundPeer(&cfg, host)
	if err != nil {
		return nil, err
	}

	return s.dialAndHandshake(p)
}

// reservePeer atomically checks that pk is not already pending, live, or
// signaled, then inserts it into pendingPeers and handshakeSignals. Returns
// the signal channel on success or ErrRepeatConnection.
func (s *Seeder) reservePeer(pk PeerKey, p *peer.Peer) (chan struct{}, error) {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()

	if _, exists := s.pendingPeers[pk]; exists {
		return nil, ErrRepeatConnection
	}
	if _, exists := s.handshakeSignals[pk]; exists {
		return nil, ErrRepeatConnection
	}
	if _, exists := s.livePeers[pk]; exists {
		return nil, ErrRepeatConnection
	}

	sig := make(chan struct{}, 1)
	s.pendingPeers[pk] = p
	s.handshakeSignals[pk] = sig
	return sig, nil
}

// releasePeer removes a peer from pendingPeers and handshakeSignals. Safe to
// call even if the peer was already moved to livePeers by onVerAck.
func (s *Seeder) releasePeer(pk PeerKey) {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()

	delete(s.pendingPeers, pk)
	delete(s.handshakeSignals, pk)
}

// dialAndHandshake dials the peer, associates the connection, and waits
// for the verack handshake signal.
func (s *Seeder) dialAndHandshake(p *peer.Peer) (*peer.Peer, error) {
	pk := peerKeyFromPeer(p)

	sig, err := s.reservePeer(pk, p)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialTimeout("tcp", p.Addr(), connectionDialTimeout)
	if err != nil {
		s.releasePeer(pk)
		return nil, err
	}

	p.AssociateConnection(conn)

	select {
	case <-sig:
		s.logger.Printf("Handshake completed with peer %s", p.Addr())
		return p, nil
	case <-time.After(maximumHandshakeWait):
		s.releasePeer(pk)
		p.Disconnect()
		p.WaitForDisconnect()
		return nil, errors.New("peer handshake timed out")
	}
}

// GetPeer returns a connected live peer.
func (s *Seeder) GetPeer(addr PeerKey) (*peer.Peer, error) {
	s.peersMu.RLock()
	defer s.peersMu.RUnlock()

	p, ok := s.livePeers[addr]
	if !ok {
		return nil, ErrNoSuchPeer
	}
	return p, nil
}

// DisconnectPeer disconnects and removes a live peer.
func (s *Seeder) DisconnectPeer(addr PeerKey) error {
	p, ok := s.removeLivePeer(addr)
	if !ok {
		return ErrNoSuchPeer
	}

	s.logger.Printf("Disconnecting from peer %s", p.Addr())
	p.Disconnect()
	p.WaitForDisconnect()
	return nil
}

// DisconnectAndBlacklist disconnects a peer and adds it to the blacklist.
func (s *Seeder) DisconnectAndBlacklist(addr PeerKey) error {
	p, ok := s.removeLivePeer(addr)
	if !ok {
		return ErrNoSuchPeer
	}

	p.Disconnect()
	p.WaitForDisconnect()
	s.addrBook.Blacklist(addr)
	return nil
}

// removeLivePeer atomically removes and returns a peer from livePeers.
func (s *Seeder) removeLivePeer(addr PeerKey) (*peer.Peer, bool) {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()

	p, ok := s.livePeers[addr]
	if ok {
		delete(s.livePeers, addr)
	}
	return p, ok
}

// DisconnectAllPeers terminates all live and pending connections.
func (s *Seeder) DisconnectAllPeers() {
	pending, liveKeys := s.snapshotAndClearPending()

	for _, p := range pending {
		p.Disconnect()
		p.WaitForDisconnect()
	}
	for _, k := range liveKeys {
		s.DisconnectPeer(k)
	}
}

// snapshotAndClearPending returns all pending peers (clearing the map) and
// all live peer keys, under a single lock.
func (s *Seeder) snapshotAndClearPending() ([]*peer.Peer, []PeerKey) {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()

	pending := make([]*peer.Peer, 0, len(s.pendingPeers))
	for k, p := range s.pendingPeers {
		pending = append(pending, p)
		delete(s.pendingPeers, k)
	}

	liveKeys := make([]PeerKey, 0, len(s.livePeers))
	for k := range s.livePeers {
		liveKeys = append(liveKeys, k)
	}

	return pending, liveKeys
}

// livePeerSnapshot returns a snapshot of all live peers.
func (s *Seeder) livePeerSnapshot() []*peer.Peer {
	s.peersMu.RLock()
	defer s.peersMu.RUnlock()

	peers := make([]*peer.Peer, 0, len(s.livePeers))
	for _, p := range s.livePeers {
		peers = append(peers, p)
	}
	return peers
}

// RequestAddresses sends getaddr to all live peers, then verifies incoming
// addresses by connecting to them. Returns the number of new peers discovered.
func (s *Seeder) RequestAddresses() int {
	for _, p := range s.livePeerSnapshot() {
		s.logger.Printf("Requesting addresses from peer %s", p.Addr())
		p.QueueMessage(wire.NewMsgGetAddr(), nil)
	}

	var peerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(crawlerGoroutineCount)

	for range crawlerGoroutineCount {
		go func() {
			defer wg.Done()
			for {
				var na *wire.NetAddress
				select {
				case next := <-s.addrQueue:
					na = next
				case <-time.After(crawlerThreadTimeout):
					return
				}

				nav2 := wire.NetAddressV2FromBytes(
					na.Timestamp, na.Services, na.IP, na.Port,
				)
				if !addrmgr.IsRoutable(nav2) && !s.config.AllowSelfConns {
					continue
				}

				pk := peerKeyFromNA(na)
				if s.addrBook.IsKnown(pk) {
					continue
				}

				portStr := strconv.Itoa(int(na.Port))
				newPeer, err := s.Connect(na.IP.String(), portStr)
				if err != nil {
					if errors.Is(err, ErrRepeatConnection) {
						continue
					}
					s.addrBook.Blacklist(pk)
					continue
				}

				newPeer.QueueMessage(wire.NewMsgGetAddr(), nil)
				peerCount.Add(1)
				s.addrBook.Add(pk)
			}
		}()
	}

	wg.Wait()
	return int(peerCount.Load())
}

// RefreshAddresses re-verifies all known-good addresses. Peers that fail
// are blacklisted. If disconnect is true, peers are disconnected after
// verification.
func (s *Seeder) RefreshAddresses(disconnect bool) {
	s.logger.Printf("Refreshing address book")

	refreshQueue := s.addrBook.EnqueueAddrs()
	if len(refreshQueue) == 0 {
		return
	}

	var wg sync.WaitGroup
	for range crawlerGoroutineCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for next := range refreshQueue {
				na := next.netaddr
				_, err := s.Connect(na.IP.String(), strconv.Itoa(int(na.Port)))
				if err != nil {
					if !errors.Is(err, ErrRepeatConnection) {
						s.addrBook.Blacklist(next.asPeerKey())
					}
					continue
				}
				if disconnect {
					s.DisconnectPeer(next.asPeerKey())
				}
			}
		}()
	}
	wg.Wait()
}

// RetryBlacklist attempts to reconnect to blacklisted peers, redeeming
// those that succeed and dropping entries older than blacklistDropTime.
func (s *Seeder) RetryBlacklist() {
	s.logger.Printf("Retrying blacklist")

	blacklistQueue := s.addrBook.EnqueueBlacklist()
	if len(blacklistQueue) == 0 {
		return
	}

	var peerCount atomic.Int32
	var wg sync.WaitGroup
	for range crawlerGoroutineCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for next := range blacklistQueue {
				na := next.netaddr
				_, err := s.Connect(na.IP.String(), strconv.Itoa(int(na.Port)))
				if err != nil {
					if time.Since(next.lastUpdate) > blacklistDropTime {
						s.addrBook.DropFromBlacklist(next.asPeerKey())
					}
					continue
				}
				s.DisconnectPeer(next.asPeerKey())
				peerCount.Add(1)
				s.addrBook.Redeem(next.asPeerKey())
			}
		}()
	}
	wg.Wait()
	s.logger.Printf("Redeemed %d peers from blacklist", peerCount.Load())
}

// WaitForAddresses blocks until at least n addresses are known or timeout.
func (s *Seeder) WaitForAddresses(n int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if s.addrBook.WaitForAddresses(ctx, n) {
		return nil
	}
	return ErrAddressTimeout
}

// Ready returns true if the seeder has enough addresses to serve.
func (s *Seeder) Ready() bool {
	return s.addrBook.Count() >= minimumReadyAddresses
}

// Addresses returns up to n shuffled IPv4 addresses on the default port.
func (s *Seeder) Addresses(n int) []net.IP {
	return s.addrBook.ShuffleAddressList(n, false, s.GetNetworkDefaultPort())
}

// AddressesV6 returns up to n shuffled IPv6 addresses on the default port.
func (s *Seeder) AddressesV6(n int) []net.IP {
	return s.addrBook.ShuffleAddressList(n, true, s.GetNetworkDefaultPort())
}

// GetPeerCount returns the number of known-good peers.
func (s *Seeder) GetPeerCount() int {
	return s.addrBook.Count()
}
