package crawler

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/peer"
	"github.com/modelos/modelos/node/wire"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useAddrV2 controls whether the mock peer sends addrv2 or addr messages.
var useAddrV2 atomic.Uint32

// secondListenerPort is a fixed port for a second mock peer that will be
// reported as a "discovered" address.
const secondListenerPort = "12345"

func TestMain(m *testing.M) {
	if err := startMockPeers(); err != nil {
		fmt.Printf("Failed to start mock peers: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func startMockPeers() error {
	cfg := &peer.Config{
		UserAgentName:    "mock-peer",
		UserAgentVersion: "0.0.1",
		ChainParams:      &chaincfg.RegressionNetParams,
		Services:         0,
		TrickleInterval:  10 * time.Second,
		ProtocolVersion:  wire.ProtocolVersion,
		AllowSelfConns:   true,
	}

	cfg.Listeners.OnGetAddr = func(p *peer.Peer, msg *wire.MsgGetAddr) {
		addr1 := wire.NewNetAddressTimestamp(
			time.Now(), 0, net.ParseIP("127.0.0.1"), 18233,
		)
		addr2 := wire.NewNetAddressTimestamp(
			time.Now(), 0, net.ParseIP("127.0.0.1"), 31337,
		)
		addr3 := wire.NewNetAddressTimestamp(
			time.Now(), 0, net.ParseIP("127.0.0.1"), 12345,
		)

		if useAddrV2.Load() != 0 {
			v2List := []*wire.NetAddressV2{
				wire.NetAddressV2FromBytes(addr1.Timestamp, addr1.Services, addr1.IP, addr1.Port),
				wire.NetAddressV2FromBytes(addr2.Timestamp, addr2.Services, addr2.IP, addr2.Port),
				wire.NetAddressV2FromBytes(addr3.Timestamp, addr3.Services, addr3.IP, addr3.Port),
			}
			p.PushAddrV2Msg(v2List)
		} else {
			p.PushAddrMsg([]*wire.NetAddress{addr1, addr2, addr3})
		}
	}

	defaultPort := chaincfg.RegressionNetParams.DefaultPort
	l1, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", defaultPort))
	if err != nil {
		return err
	}
	l2, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", secondListenerPort))
	if err != nil {
		return err
	}

	accept := func(l net.Listener, c *peer.Config) {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			mp := peer.NewInboundPeer(c)
			mp.AssociateConnection(conn)
		}
	}

	go accept(l1, cfg)

	cfg2 := *cfg
	go accept(l2, &cfg2)

	return nil
}

// newTestSeeder creates a Seeder that allows self-connections for testing.
func newTestSeeder(networkName string) (*Seeder, error) {
	s, err := NewSeeder(networkName)
	if err != nil {
		return nil, err
	}
	s.config.AllowSelfConns = true
	s.logger = log.New(os.Stdout, "pearl-seeder: ", log.Ldate|log.Ltime|log.Lshortfile|log.LUTC)
	return s, nil
}

func TestOutboundPeerSync(t *testing.T) {
	s, err := newTestSeeder("regtest")
	require.NoError(t, err)

	err = s.ConnectOnDefaultPort("127.0.0.1")
	require.NoError(t, err)

	p, err := s.GetPeer("127.0.0.1:18444")
	require.NoError(t, err)
	assert.True(t, p.Connected())

	s.DisconnectPeer("127.0.0.1:18444")

	_, err = s.GetPeer("127.0.0.1:18444")
	assert.ErrorIs(t, err, ErrNoSuchPeer)
}

func TestOutboundPeerAsync(t *testing.T) {
	s, err := newTestSeeder("regtest")
	require.NoError(t, err)

	errs := make(chan error, 4)
	for range 4 {
		go func() { errs <- s.ConnectOnDefaultPort("127.0.0.1") }()
	}
	for range 4 {
		e := <-errs
		if e != nil {
			assert.ErrorIs(t, e, ErrRepeatConnection)
		}
	}

	p, err := s.GetPeer("127.0.0.1:18444")
	require.NoError(t, err)
	assert.True(t, p.Connected())

	err = s.ConnectOnDefaultPort("127.0.0.1")
	assert.ErrorIs(t, err, ErrRepeatConnection)

	s.DisconnectAllPeers()
}

func TestRequestAddresses(t *testing.T) {
	s, err := newTestSeeder("regtest")
	require.NoError(t, err)

	err = s.ConnectOnDefaultPort("127.0.0.1")
	require.NoError(t, err)

	go s.RequestAddresses()
	err = s.WaitForAddresses(1, 5*time.Second)
	assert.NoError(t, err)

	err = s.WaitForAddresses(500, 1*time.Second)
	assert.ErrorIs(t, err, ErrAddressTimeout)
}

func TestRequestAddressesV2(t *testing.T) {
	useAddrV2.Store(1)
	defer useAddrV2.Store(0)

	s, err := newTestSeeder("regtest")
	require.NoError(t, err)

	originalFn := s.config.Listeners.OnAddrV2
	var receivedAddrV2 atomic.Uint32
	s.config.Listeners.OnAddrV2 = func(p *peer.Peer, msg *wire.MsgAddrV2) {
		receivedAddrV2.Store(1)
		originalFn(p, msg)
	}

	err = s.ConnectOnDefaultPort("127.0.0.1")
	require.NoError(t, err)

	go s.RequestAddresses()
	err = s.WaitForAddresses(1, 5*time.Second)
	assert.NoError(t, err)

	assert.Equal(t, uint32(1), receivedAddrV2.Load(), "OnAddrV2 was not called")
}

func TestBlacklist(t *testing.T) {
	s, err := newTestSeeder("regtest")
	require.NoError(t, err)

	err = s.ConnectOnDefaultPort("127.0.0.1")
	require.NoError(t, err)

	go s.RequestAddresses()
	err = s.WaitForAddresses(1, 5*time.Second)
	require.NoError(t, err)

	workingPeer := PeerKey("127.0.0.1:12345")

	// Disconnect the peer that was discovered during RequestAddresses
	// so we can test reconnection.
	s.DisconnectAllPeers()

	s.addrBook.Blacklist(workingPeer)
	_, err = s.Connect("127.0.0.1", secondListenerPort)
	assert.ErrorIs(t, err, ErrBlacklistedPeer)

	s.addrBook.Redeem(workingPeer)
	_, err = s.Connect("127.0.0.1", secondListenerPort)
	assert.NoError(t, err)
}
