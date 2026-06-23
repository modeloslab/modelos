// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"testing"
	"time"

	"github.com/modelos/modelos/node/chaincfg"
	_ "github.com/modelos/modelos/wallet/walletdb/bdb"
)

// TestCreateWatchingOnly checks that we can construct a watching-only
// wallet.
func TestCreateWatchingOnly(t *testing.T) {
	// Set up a wallet.
	dir := t.TempDir()

	pubPass := []byte("hello")

	loader := NewLoader(
		&chaincfg.TestNetParams, dir, true, defaultDBTimeout, 250,
		WithWalletSyncRetryInterval(10*time.Millisecond),
	)
	_, err := loader.CreateNewWatchingOnlyWallet(pubPass, time.Now())
	if err != nil {
		t.Fatalf("unable to create wallet: %v", err)
	}
}
