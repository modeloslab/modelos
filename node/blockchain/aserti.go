// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/big"

	"github.com/modelos/modelos/node/chaincfg"
)

// Colossus 2.0 (= absolute ASERT) constants. "Colossus 1.0" was a median+ASERT
// hybrid that simulation proved unstable; it has been removed. The activation
// height name is kept because AuxPoW + the DAA activate together at this block.
const (
	// colossusTargetBlockTime is the desired seconds per block.
	colossusTargetBlockTime = int64(194)

	// colossusASERTHalflife is the ASERT half-life in seconds (~5.2h: 96×194). A
	// sustained hashrate error is halved roughly every half-life.
	colossusASERTHalflife = int64(96 * 194)

	// colossusAuxPowActivationBlock is the height at which the ASERT DAA (Colossus
	// 2.0) takes over from Phase-1 WTEMA. The block AT this height is the anchor;
	// ASERT governs every block AFTER it (dispatch: ASERT when lastNode.Height() >=
	// this value, so block (this+1) is the first ASERT block).
	//
	// Activation 15860: Phase-1 WTEMA governs blocks 0–15860, then ASERT from 15861.
	// HARD FORK: every node must run this exact value before the chain reaches it.
	colossusAuxPowActivationBlock = int32(15860)

	// colossusReanchorBlock re-anchors the ASERT schedule at a later height to discard
	// bootstrap time-debt. 0 = disabled (no re-anchor).
	colossusReanchorBlock = int32(17500)
)

// applyTargetFloor clamps a target to [1, powLimit] — i.e. difficulty into
// [1, max]. powLimit is the easiest allowed target (minimum difficulty).
func applyTargetFloor(target, powLimit *big.Int) *big.Int {
	if target.Sign() <= 0 {
		return big.NewInt(1)
	}
	if target.Cmp(powLimit) > 0 {
		return new(big.Int).Set(powLimit)
	}
	return target
}

// Phase-2 difficulty algorithm: pure absolute ASERT (aserti3-2d) — the BCH/Nexa
// DAA. This REPLACES the earlier "Colossus" median+ASERT hybrid on the consensus
// path (difficulty.go now calls calcNextAsertTarget).
//
// Why the swap: simulation (blockchain/daa_sim.py) showed the hybrid telescoped
// the cumulative-from-anchor exponent onto the RUNNING target — newTarget =
// prevTarget × 2^(dev_n/HL) compounds to prevTarget×2^(Σdev_k/HL) — making it an
// undamped integrator that oscillated by >1000% under a hashrate step (median +
// ±10 cap did NOT tame it). Absolute ASERT applies the exponent to a FIXED anchor
// target, so it is drift-free and provably non-oscillating, and it uses INTEGER
// math (a cubic 2^x approximation) so every node computes a bit-identical result —
// no floating point anywhere in consensus.
//
//	next = anchorTarget · 2^((timeDiff − T·(heightDiff+1)) / halflife)
//	  anchorTarget : target of the ASERT anchor block (the activation block)
//	  timeDiff     : lastNode.time − anchorParent.time  (committed parent data only)
//	  heightDiff   : lastNode.height − anchorHeight
//
// The candidate block's own (miner-controlled) timestamp is intentionally NOT used.
func calcNextAsertTarget(lastNode chaincfg.HeaderCtx, powLimit *big.Int) *big.Int {
	// Anchor = the activation block. (colossusReanchorBlock can move it later if ever set > 0;
	// disabled by default, so the schedule is anchored at activation.)
	anchorHeight := colossusAuxPowActivationBlock
	if colossusReanchorBlock > 0 && lastNode.Height() >= colossusReanchorBlock {
		anchorHeight = colossusReanchorBlock
	}

	steps := lastNode.Height() - anchorHeight
	if steps < 0 {
		// Should not occur (gated by calcNextRequiredDifficulty). Hold difficulty.
		return applyTargetFloor(CompactToBig(lastNode.Bits()), powLimit)
	}

	// Anchor = the block at the activation height. ASERT governs every block after
	// it; the anchor block itself is set by Phase-1 WTEMA, so it always exists here.
	anchor := lastNode
	if steps > 0 {
		anchor = lastNode.RelativeAncestorCtx(steps)
	}
	if anchor == nil {
		return applyTargetFloor(CompactToBig(lastNode.Bits()), powLimit)
	}

	// Anchor against the block BEFORE the anchor (BCH convention) so the anchor
	// block counts as the first scheduled block. Fall back to the anchor's own
	// time if it is the genesis block.
	anchorTime := anchor.Timestamp()
	if ap := anchor.Parent(); ap != nil {
		anchorTime = ap.Timestamp()
	}

	return calcASERT(
		CompactToBig(anchor.Bits()),              // anchorTarget (FIXED)
		colossusTargetBlockTime,                  // ideal spacing (T)
		lastNode.Timestamp()-anchorTime,          // timeDiff
		int64(lastNode.Height()-anchor.Height()), // heightDiff
		powLimit,
		colossusASERTHalflife,
	)
}

// calcASERT is the integer aserti3-2d core. The result depends ONLY on the schedule
// deviation and the FIXED anchorTarget — never on the running target — so there is
// no telescoping and no drift. 2^x is a cubic fixed-point approximation (max error
// < 0.013%) computed entirely in integers, identical on every node and CPU.
//
// Reference: BCH aserti3-2d (Bitcoin Cash, Nov 2020) / Nexa.
func calcASERT(anchorTarget *big.Int, spacing, timeDiff, heightDiff int64,
	powLimit *big.Int, halfLife int64) *big.Int {

	// exponent in 2^16 fixed point. Go integer division truncates toward zero,
	// matching the reference C implementation.
	exponent := ((timeDiff - spacing*(heightDiff+1)) * 65536) / halfLife

	// Decompose into integer shifts and a 16-bit fraction. The two's-complement
	// low 16 bits give frac ∈ [0, 65536) even when exponent is negative.
	shifts := exponent >> 16
	frac := uint64(uint16(exponent))

	// factor = 65536 · 2^(frac/65536) via the BCH cubic approximation.
	factor := int64(65536) + int64((195766423245049*frac+
		971821376*frac*frac+5127*frac*frac*frac+(uint64(1)<<47))>>48)

	next := new(big.Int).Mul(anchorTarget, big.NewInt(factor))

	// factor carried a 2^16 scale; the net shift is (shifts − 16).
	shifts -= 16
	if shifts < 0 {
		next.Rsh(next, uint(-shifts))
	} else {
		next.Lsh(next, uint(shifts))
	}

	return applyTargetFloor(next, powLimit)
}
