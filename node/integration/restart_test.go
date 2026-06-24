//go:build rpctest
// +build rpctest

// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package integration

import (
	"testing"

	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/integration/rpctest"
	"github.com/stretchr/testify/require"
)

// TestNodeRestartWithExistingData verifies that a node can restart
// with pre-existing blockchain data. This specifically tests the
// genesis block index initialization on restart.
//
// This test was added to prevent regression of the bug where
// mismatched genesis hashes caused the node to fail on restart with:
// "assertion failed: initChainState: Expected first entry in block
// index to be genesis block, found <wrong hash>"
func TestNodeRestartWithExistingData(t *testing.T) {
	t.Parallel()

	// Create a new node with transaction and filter indexes enabled
	// to exercise more code paths during restart.
	pearldCfg := []string{"--txindex", "--addrindex"}
	r, err := rpctest.New(&chaincfg.SimNetParams, nil, pearldCfg, "")
	require.NoError(t, err)

	// Start the node and generate some blocks.
	require.NoError(t, r.SetUp(true, 10))
	t.Cleanup(func() { r.TearDown() })

	// Get the current chain state before restart.
	infoBeforeRestart, err := r.Client.GetBlockChainInfo()
	require.NoError(t, err)
	t.Logf("Before restart: height=%d, bestblock=%s",
		infoBeforeRestart.Blocks, infoBeforeRestart.BestBlockHash)

	// Generate more blocks to ensure we have data beyond genesis.
	_, err = r.Client.Generate(5)
	require.NoError(t, err)

	heightBeforeRestart, err := r.Client.GetBlockCount()
	require.NoError(t, err)
	t.Logf("Generated blocks, height before restart: %d", heightBeforeRestart)

	// Restart the node - this is the critical test.
	// If genesis hash doesn't match, this will fail.
	t.Log("Restarting node...")
	require.NoError(t, r.Restart(), "node should restart successfully")

	// Verify chain state after restart.
	infoAfterRestart, err := r.Client.GetBlockChainInfo()
	require.NoError(t, err)
	t.Logf("After restart: height=%d, bestblock=%s",
		infoAfterRestart.Blocks, infoAfterRestart.BestBlockHash)

	// Verify the height is preserved.
	require.Equal(t, int32(heightBeforeRestart), infoAfterRestart.Blocks,
		"chain height should be preserved after restart")

	// Verify we can still generate blocks after restart.
	_, err = r.Client.Generate(1)
	require.NoError(t, err)

	finalHeight, err := r.Client.GetBlockCount()
	require.NoError(t, err)
	require.Equal(t, heightBeforeRestart+1, int64(finalHeight),
		"should be able to mine after restart")

	t.Logf("Test passed: node restarted successfully, final height: %d", finalHeight)
}

// TestNodeMultipleRestarts verifies that a node can handle multiple
// restarts in succession without issues.
func TestNodeMultipleRestarts(t *testing.T) {
	t.Parallel()

	pearldCfg := []string{"--txindex"}
	r, err := rpctest.New(&chaincfg.SimNetParams, nil, pearldCfg, "")
	require.NoError(t, err)

	require.NoError(t, r.SetUp(true, 5))
	t.Cleanup(func() { r.TearDown() })

	// Perform multiple restart cycles.
	for i := 0; i < 3; i++ {
		// Generate a block before restart.
		_, err := r.Client.Generate(1)
		require.NoError(t, err)

		heightBefore, err := r.Client.GetBlockCount()
		require.NoError(t, err)
		t.Logf("Restart cycle %d: height before restart = %d", i+1, heightBefore)

		// Restart the node.
		require.NoError(t, r.Restart(), "restart cycle %d should succeed", i+1)

		// Verify state preserved.
		heightAfter, err := r.Client.GetBlockCount()
		require.NoError(t, err)
		require.Equal(t, heightBefore, heightAfter,
			"height should be preserved after restart cycle %d", i+1)
	}

	t.Log("Test passed: node handled multiple restarts successfully")
}
