// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"testing"

	"github.com/modelos/modelos/node/wire"
)

// MinAcceptableProtocolVersion must never be derived from wire.ProtocolVersion.
//
// Coupling them means a release that bumps the protocol version also raises the
// floor, so the first node to upgrade refuses every peer still on the old
// version — isolating itself, then splitting the network into two mining halves
// whose reunion is a deep reorg. A protocol upgrade must ship in two steps:
// bump the version first and leave the floor alone, then raise the floor only
// once old nodes genuinely cannot follow the chain.
//
// This test cannot see the source-level coupling directly, so it enforces the
// property that coupling would violate: the floor must be STRICTLY BELOW the
// version we speak whenever we have moved past version 1. That is exactly the
// window in which un-upgraded peers must still be accepted.
func TestProtocolFloorIsNotCoupledToCurrentVersion(t *testing.T) {
	if wire.ProtocolVersion == 1 {
		// Nothing has been bumped yet; the floor may legitimately equal it.
		if MinAcceptableProtocolVersion > wire.ProtocolVersion {
			t.Fatalf("floor %d is above the version we speak (%d) — we would "+
				"reject peers running our own software",
				MinAcceptableProtocolVersion, wire.ProtocolVersion)
		}
		return
	}

	if MinAcceptableProtocolVersion >= wire.ProtocolVersion {
		t.Fatalf(
			"MinAcceptableProtocolVersion (%d) was raised to meet "+
				"wire.ProtocolVersion (%d) in the same release.\n"+
				"That disconnects every peer that has not upgraded yet and "+
				"partitions the network.\n"+
				"Ship the version bump first with the floor left alone; raise "+
				"the floor only after the upgrade has activated.",
			MinAcceptableProtocolVersion, wire.ProtocolVersion)
	}
}

// The floor must never exceed the version we ourselves advertise, or we would
// refuse peers running identical software.
func TestProtocolFloorNotAboveOurOwnVersion(t *testing.T) {
	if MinAcceptableProtocolVersion > wire.ProtocolVersion {
		t.Fatalf("floor %d exceeds our own advertised version %d",
			MinAcceptableProtocolVersion, wire.ProtocolVersion)
	}
}
