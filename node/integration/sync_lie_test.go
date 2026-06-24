//go:build rpctest
// +build rpctest

// Regression test for pickSyncCandidate's inbound-exclusion + cooldown
// hardening (node/netsync/manager.go).

package integration

import (
	"net"
	"testing"
	"time"

	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/modelos/modelos/node/integration/rpctest"
	"github.com/modelos/modelos/node/peer"
	"github.com/modelos/modelos/node/wire"
	"github.com/stretchr/testify/require"
)

const (
	// Forged tip height advertised by the liar in its version handshake.
	liarClaimedHeight int32 = 999_999

	// Time the victim has to catch up to honest's tip after honest connects.
	catchupWindow = 30 * time.Second

	// Blocks honest pre-mines before connecting; large enough to make the
	// catch-up assertion unambiguous.
	honestStartBlocks uint32 = 10
)

// newLiarPeer dials nodeAddr, completes the v2 handshake claiming
// LastBlock=liarClaimedHeight, and answers getheaders/getblocks with empty
// messages. Empty replies (vs silence) clear the wire-level stall deadline
// so pearld won't disconnect us at the wire layer. The returned peer must
// be Disconnect()'d by the caller.
func newLiarPeer(t *testing.T, nodeAddr string) *peer.Peer {
	t.Helper()

	conn, err := net.DialTimeout("tcp", nodeAddr, 5*time.Second)
	require.NoError(t, err, "liar: dial victim")

	verackCh := make(chan struct{})
	cfg := &peer.Config{
		Listeners: peer.MessageListeners{
			OnVerAck: func(_ *peer.Peer, _ *wire.MsgVerAck) {
				close(verackCh)
			},
			OnGetHeaders: func(p *peer.Peer, _ *wire.MsgGetHeaders) {
				p.QueueMessage(wire.NewMsgHeaders(), nil)
			},
			OnGetBlocks: func(p *peer.Peer, _ *wire.MsgGetBlocks) {
				p.QueueMessage(wire.NewMsgInv(), nil)
			},
		},
		// NewestBlock backs our outgoing version's LastBlock; forge it.
		NewestBlock: func() (*chainhash.Hash, int32, error) {
			return &chainhash.Hash{}, liarClaimedHeight, nil
		},
		UserAgentName:       "liar-peer",
		UserAgentVersion:    "1.0.0",
		Services:            wire.SFNodeNetwork | wire.SFNodeWitness | wire.SFNodeP2PV2,
		ChainParams:         &chaincfg.SimNetParams,
		DisableStallHandler: true, // test drives lifetime via Disconnect()
	}

	p, err := peer.NewOutboundPeer(cfg, nodeAddr)
	if err != nil {
		conn.Close()
		t.Fatalf("liar: NewOutboundPeer: %v", err)
	}
	p.AssociateConnection(conn)

	select {
	case <-verackCh:
		return p
	case <-time.After(15 * time.Second):
		p.Disconnect()
		p.WaitForDisconnect()
		t.Fatal("liar: timed out waiting for verack")
		return nil
	}
}

// TestSyncPeerLiesAboutHeight asserts an inbound peer claiming a vastly
// inflated LastBlock never reaches the syncnode role when an outbound
// peer is available, and the victim syncs from the legitimate outbound
// peer despite the liar holding an open connection.
func TestSyncPeerLiesAboutHeight(t *testing.T) {
	victim, err := rpctest.New(&chaincfg.SimNetParams, nil, nil, "")
	require.NoError(t, err)
	require.NoError(t, victim.SetUp(true, 0))
	t.Cleanup(func() { require.NoError(t, victim.TearDown()) })

	honest, err := rpctest.New(&chaincfg.SimNetParams, nil, nil, "")
	require.NoError(t, err)
	require.NoError(t, honest.SetUp(true, 0))
	t.Cleanup(func() { _ = honest.TearDown() })

	_, err = honest.Client.Generate(honestStartBlocks)
	require.NoError(t, err)

	// Connect honest first (outbound from victim's POV) so the
	// candidate pool has an outbound entry. Then connect the liar
	// inbound — with an outbound available, inbound fallback should
	// NOT activate, and the liar must not become syncnode.
	require.NoError(t, rpctest.ConnectNode(victim, honest))
	time.Sleep(1 * time.Second)

	liar := newLiarPeer(t, victim.P2PAddress())
	defer func() { liar.Disconnect(); liar.WaitForDisconnect() }()

	time.Sleep(2 * time.Second)
	peers, err := victim.Client.GetPeerInfo()
	require.NoError(t, err)
	for _, p := range peers {
		if p.Addr == liar.LocalAddr().String() {
			require.Falsef(t, p.SyncNode,
				"inbound liar at %s was selected as syncnode "+
					"despite outbound peer being available", p.Addr)
		}
	}

	// Victim must catch up from honest.
	require.Eventually(t, func() bool {
		_, h, err := victim.Client.GetBestBlock()
		return err == nil && h >= int32(honestStartBlocks)
	}, catchupWindow, 500*time.Millisecond,
		"victim failed to catch up to honest tip h=%d",
		honestStartBlocks)

	_, hFinal, _ := victim.Client.GetBestBlock()
	t.Logf("victim caught up to h=%d with liar still connected", hFinal)

	// Liar's inbound connection is still present and still not syncnode.
	peers, err = victim.Client.GetPeerInfo()
	require.NoError(t, err)
	var liarFound bool
	for _, p := range peers {
		if p.Addr == liar.LocalAddr().String() {
			liarFound = true
			require.False(t, p.SyncNode, "liar must not be syncnode")
		}
	}
	require.True(t, liarFound, "liar's inbound connection should still be present")
}
