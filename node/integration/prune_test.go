// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is ignored during the regular tests due to the following build tag.
//go:build rpctest
// +build rpctest

package integration

import (
	"testing"

	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/integration/rpctest"
	"github.com/stretchr/testify/require"
)

func TestPrune(t *testing.T) {
	t.Parallel()

	// Boilerplate code to make a pruned node.
	pearldCfg := []string{"--prune=1536"}
	r, err := rpctest.New(&chaincfg.SimNetParams, nil, pearldCfg, "")
	require.NoError(t, err)

	if err := r.SetUp(false, 0); err != nil {
		require.NoError(t, err)
	}
	t.Cleanup(func() { r.TearDown() })

	// Check that the rpc call for block chain info comes back correctly.
	chainInfo, err := r.Client.GetBlockChainInfo()
	require.NoError(t, err)

	if !chainInfo.Pruned {
		t.Fatalf("expected the node to be pruned but the pruned "+
			"boolean was %v", chainInfo.Pruned)
	}
}
