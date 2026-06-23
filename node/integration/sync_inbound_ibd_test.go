//go:build rpctest
// +build rpctest

// Regression test for pickSyncCandidate's inbound fallback during IBD.
// Mirrors Bitcoin Core's p2p_block_sync.py test (PR #24171).

package integration

import (
	"testing"
	"time"

	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/integration/rpctest"
	"github.com/stretchr/testify/require"
)

// TestSyncFromInboundDuringIBD sets up two simnet nodes where the only
// block source dials the victim (making it inbound from victim's POV).
// The victim has no outbound peers. Asserts the victim syncs to the
// source's tip via the inbound fallback in pickSyncCandidate.
func TestSyncFromInboundDuringIBD(t *testing.T) {
	const blocksToMine = 10

	victim, err := rpctest.New(&chaincfg.SimNetParams, nil, nil, "")
	require.NoError(t, err)
	require.NoError(t, victim.SetUp(true, 0))
	t.Cleanup(func() { require.NoError(t, victim.TearDown()) })

	source, err := rpctest.New(&chaincfg.SimNetParams, nil, nil, "")
	require.NoError(t, err)
	require.NoError(t, source.SetUp(true, 0))
	t.Cleanup(func() { _ = source.TearDown() })

	_, err = source.Client.Generate(blocksToMine)
	require.NoError(t, err)

	// Source dials victim: from victim's POV, source is inbound.
	require.NoError(t, rpctest.ConnectNode(source, victim))

	// Victim must sync to source's tip. Without the inbound fallback
	// this times out: no outbound peers, pickSyncCandidate returns
	// nil, handleInvMsg drops source's block announcements.
	require.Eventually(t, func() bool {
		_, h, err := victim.Client.GetBestBlock()
		return err == nil && h >= int32(blocksToMine)
	}, 30*time.Second, 500*time.Millisecond,
		"victim failed to sync from inbound source to h=%d",
		blocksToMine)

	_, victimH, _ := victim.Client.GetBestBlock()
	t.Logf("victim synced to h=%d from inbound source", victimH)
}
