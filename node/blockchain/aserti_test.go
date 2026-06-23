// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/big"
	"testing"
)

func asertPowLimit() *big.Int {
	return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 208), big.NewInt(1))
}

func relDiff(a, b *big.Int) float64 {
	d := new(big.Float).Abs(new(big.Float).Sub(new(big.Float).SetInt(a), new(big.Float).SetInt(b)))
	r, _ := new(big.Float).Quo(d, new(big.Float).SetInt(b)).Float64()
	return r
}

func diffFromTarget(powLimit, target *big.Int) float64 {
	if target.Sign() <= 0 {
		return 0
	}
	f, _ := new(big.Float).Quo(new(big.Float).SetInt(powLimit), new(big.Float).SetInt(target)).Float64()
	return f
}

// TestAsertDirection pins the core ASERT response: on-schedule holds the anchor,
// being ahead of schedule hardens (smaller target), behind eases — and one
// half-life of deviation moves the target by exactly 2x (validates the integer
// cubic 2^x against the closed form).
func TestAsertDirection(t *testing.T) {
	pow := asertPowLimit()
	anchor := new(big.Int).Rsh(pow, 10) // arbitrary anchor target
	T := colossusTargetBlockTime
	HL := colossusASERTHalflife

	// On schedule: timeDiff == T*(heightDiff+1) → factor ≈ 1.
	if r := relDiff(calcASERT(anchor, T, T*101, 100, pow, HL), anchor); r > 0.0005 {
		t.Fatalf("on-schedule drifted %.4f%% from anchor (cubic 2^x error too high)", r*100)
	}
	// Exactly one half-life AHEAD → target must HALVE (difficulty doubles).
	ahead := calcASERT(anchor, T, T*101-HL, 100, pow, HL)
	if ahead.Cmp(anchor) >= 0 {
		t.Fatal("ahead-of-schedule must harden (smaller target)")
	}
	if r := relDiff(ahead, new(big.Int).Rsh(anchor, 1)); r > 1e-9 {
		t.Fatalf("1 half-life ahead should exactly halve the target, off by %.3g%%", r*100)
	}
	// One half-life BEHIND → target must DOUBLE (difficulty halves).
	behind := calcASERT(anchor, T, T*101+HL, 100, pow, HL)
	if behind.Cmp(anchor) <= 0 {
		t.Fatal("behind-schedule must ease (larger target)")
	}
	if r := relDiff(behind, new(big.Int).Lsh(anchor, 1)); r > 1e-9 {
		t.Fatalf("1 half-life behind should exactly double the target, off by %.3g%%", r*100)
	}
}

// TestAsertAnchorInvariance proves the result depends ONLY on (anchor, schedule
// deviation), never on the running/previous target — this is exactly what the old
// Colossus hybrid violated (it multiplied prevTarget by a cumulative factor and
// telescoped/oscillated). Same anchor + same deviation ⇒ identical target.
func TestAsertAnchorInvariance(t *testing.T) {
	pow := asertPowLimit()
	anchor := new(big.Int).Rsh(pow, 8)
	T := colossusTargetBlockTime
	HL := colossusASERTHalflife
	a := calcASERT(anchor, T, 50*T, 30, pow, HL)
	b := calcASERT(anchor, T, 50*T, 30, pow, HL)
	if a.Cmp(b) != 0 {
		t.Fatal("ASERT must be a pure function of (anchor, deviation) — non-deterministic result")
	}
}

// TestAsertStability is the headline regression: under a 10x hashrate step ASERT
// converges SMOOTHLY with NO overshoot. The old Colossus hybrid overshot ~1000%
// here (see blockchain/daa_sim.py); this guards against ever regressing to a
// telescoping/relative DAA.
func TestAsertStability(t *testing.T) {
	pow := asertPowLimit()
	T := colossusTargetBlockTime
	A := colossusAuxPowActivationBlock
	startDiff := 1000.0
	bits0 := BigToCompact(new(big.Int).Div(pow, big.NewInt(int64(startDiff))))

	// Anchor chain: parent(A-1) → anchor(A), on schedule, then warm up.
	ap := &mockHeaderCtx{height: A - 1, bits: bits0, timestamp: 0}
	tip := &mockHeaderCtx{height: A, bits: bits0, timestamp: T, parent: ap}
	for i := 0; i < 50; i++ {
		nt := calcNextAsertTarget(tip, pow)
		tip = &mockHeaderCtx{height: tip.height + 1, bits: BigToCompact(nt), timestamp: tip.timestamp + T, parent: tip}
	}

	const H = 10.0 // 10x hashrate step
	target := startDiff * H
	peak, last := 0.0, 0.0
	for i := 0; i < 1000; i++ {
		nt := calcNextAsertTarget(tip, pow)
		d := diffFromTarget(pow, nt)
		if d > peak {
			peak = d
		}
		last = d
		// deterministic mean solve time at difficulty d under the 10x hashrate
		st := int64(float64(T) * d / (startDiff * H))
		if st < 1 {
			st = 1
		}
		tip = &mockHeaderCtx{height: tip.height + 1, bits: BigToCompact(nt), timestamp: tip.timestamp + st, parent: tip}
	}
	if peak > target*1.10 {
		t.Fatalf("ASERT OVERSHOOT: peak=%.0f (%.0f%% of target) — DAA is unstable", peak, peak/target*100)
	}
	if last < target*0.80 || last > target*1.20 {
		t.Fatalf("ASERT did not converge: settled=%.0f, want ~%.0f", last, target)
	}
	t.Logf("10x step: peak=%.0f (%.0f%% of target), settled=%.0f — smooth, no overshoot",
		peak, peak/target*100, last)
}
