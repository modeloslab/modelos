// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"testing"
	"time"
)

// The peer quality gate exists to keep hostile peers from driving our block
// download. These tests pin the properties that stop it turning on honest peers
// during a fork — the feedback loop that made a chain with a real fork rate
// progressively worse at syncing:
//
//	fork → peers struck for relaying the losing block → strike limit reached
//	     → gated out of block download → slower propagation → MORE forks → …

// An inbound peer must not START gated. Beginning at lowQualityStrikeLimit made
// every unsolicited connection low-quality on arrival, and since the gate skips
// block requests from low-quality peers, it could not earn its way back — which
// is why two ordinary nodes struggled to follow the tip together while a node
// talking to a seed (outbound, ungated) was fine.
func TestInboundPeerDoesNotStartGated(t *testing.T) {
	if inboundInitialStrikes >= lowQualityStrikeLimit {
		t.Fatalf("inbound peers start gated: inboundInitialStrikes=%d >= limit=%d",
			inboundInitialStrikes, lowQualityStrikeLimit)
	}

	state := &peerSyncState{
		nonTipStrikes:   inboundInitialStrikes,
		lastStrikeDecay: time.Now(),
	}
	if !isPeerHighQuality(state) {
		t.Fatal("a freshly connected inbound peer is already gated out of block download")
	}
}

// An inbound peer must still be MORE suspect than an outbound one: it should
// reach the limit after fewer faults. The handicap is the point.
func TestInboundPeerKeepsItsHandicap(t *testing.T) {
	inbound := &peerSyncState{nonTipStrikes: inboundInitialStrikes, lastStrikeDecay: time.Now()}
	outbound := &peerSyncState{nonTipStrikes: 0, lastStrikeDecay: time.Now()}

	faults := 0
	for isPeerHighQuality(inbound) && faults < 100 {
		inbound.strikeNonTip()
		faults++
	}
	inboundFaults := faults

	faults = 0
	for isPeerHighQuality(outbound) && faults < 100 {
		outbound.strikeNonTip()
		faults++
	}
	if inboundFaults >= faults {
		t.Fatalf("inbound peer tolerated %d faults, outbound %d — handicap lost",
			inboundFaults, faults)
	}
}

// Strikes must decay, so a peer that hit a bad patch during a reorg storm
// recovers instead of staying downgraded for the life of the connection.
func TestStrikesDecayOverTime(t *testing.T) {
	state := &peerSyncState{}
	for i := 0; i < lowQualityStrikeLimit; i++ {
		state.strikeNonTip()
	}
	if isPeerHighQuality(state) {
		t.Fatal("precondition: peer should be gated after hitting the limit")
	}

	// Rewind the decay clock past the limit's worth of intervals.
	state.lastStrikeDecay = time.Now().Add(
		-time.Duration(lowQualityStrikeLimit+1) * strikeDecayInterval)

	if !isPeerHighQuality(state) {
		t.Fatalf("strikes did not decay: still %d after %d intervals",
			state.nonTipStrikes, lowQualityStrikeLimit+1)
	}
	if state.nonTipStrikes != 0 {
		t.Fatalf("expected all strikes forgiven, %d remain", state.nonTipStrikes)
	}
}

// Decay must be gradual, not all-or-nothing: one interval forgives one strike.
func TestStrikeDecayIsGradual(t *testing.T) {
	state := &peerSyncState{}
	for i := 0; i < lowQualityStrikeLimit; i++ {
		state.strikeNonTip()
	}
	state.lastStrikeDecay = time.Now().Add(-2 * strikeDecayInterval)

	got := state.decayedStrikes()
	if want := lowQualityStrikeLimit - 2; got != want {
		t.Fatalf("after 2 intervals strikes = %d, want %d", got, want)
	}
}

// Decay must not run backwards: repeated reads inside one interval must not
// forgive anything further.
func TestStrikeDecayDoesNotOverForgive(t *testing.T) {
	state := &peerSyncState{}
	state.strikeNonTip()
	state.strikeNonTip()
	state.lastStrikeDecay = time.Now().Add(-strikeDecayInterval)

	first := state.decayedStrikes()
	second := state.decayedStrikes()
	third := state.decayedStrikes()
	if first != second || second != third {
		t.Fatalf("repeated reads changed the count: %d, %d, %d", first, second, third)
	}
	if first != 1 {
		t.Fatalf("one interval should forgive exactly one strike, got %d remaining", first)
	}
}

// A tip-extending block clears the record outright.
func TestTipExtendingBlockClearsStrikes(t *testing.T) {
	state := &peerSyncState{nonTipStrikes: lowQualityStrikeLimit, lastStrikeDecay: time.Now()}
	state.nonTipStrikes = 0 // what handleBlockMsg does on isMainChain
	if !isPeerHighQuality(state) {
		t.Fatal("peer still gated after delivering a tip-extending block")
	}
}

// A peer that never faults must never be gated, no matter how long it is
// connected — the gate is for misbehaviour, not for age.
func TestCleanPeerIsNeverGated(t *testing.T) {
	state := &peerSyncState{lastStrikeDecay: time.Now().Add(-24 * time.Hour)}
	if !isPeerHighQuality(state) {
		t.Fatal("a peer with no faults was gated")
	}
	if state.decayedStrikes() != 0 {
		t.Fatalf("clean peer accumulated %d strikes", state.nonTipStrikes)
	}
}
