// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/big"
	"time"

	"github.com/modelos/modelos/node/blockchain/internal/workmath"
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/modelos/modelos/node/wire"
)

// HashToBig converts a chainhash.Hash into a big.Int that can be used to
// perform math comparisons.
func HashToBig(hash *chainhash.Hash) *big.Int {
	return workmath.HashToBig(hash)
}

// CompactToBig converts a compact representation of a whole number N to an
// unsigned 32-bit number.  The representation is similar to IEEE754 floating
// point numbers.
//
// Like IEEE754 floating point, there are three basic components: the sign,
// the exponent, and the mantissa.  They are broken out as follows:
//
// - the most significant 8 bits represent the unsigned base 256 exponent
// - bit 23 (the 24th bit) represents the sign bit
// - the least significant 23 bits represent the mantissa
//
//	-------------------------------------------------
//	|   Exponent     |    Sign    |    Mantissa     |
//	-------------------------------------------------
//	| 8 bits [31-24] | 1 bit [23] | 23 bits [22-00] |
//	-------------------------------------------------
//
// The formula to calculate N is:
//
//	N = (-1^sign) * mantissa * 256^(exponent-3)
//
// This compact form is used to encode unsigned 256-bit numbers
// which represent difficulty targets, thus there really is not a need for a
// sign bit, but it is implemented here to stay consistent with Bitcoin Core.
func CompactToBig(compact uint32) *big.Int {
	return workmath.CompactToBig(compact)
}

// BigToCompact converts a whole number N to a compact representation using
// an unsigned 32-bit number.  The compact representation only provides 23 bits
// of precision, so values larger than (2^23 - 1) only encode the most
// significant digits of the number.  See CompactToBig for details.
func BigToCompact(n *big.Int) uint32 {
	return workmath.BigToCompact(n)
}

// CalcWork calculates a work value from difficulty bits.  The protocol increases
// the difficulty for generating a block by decreasing the value which the
// generated hash must be less than.  This difficulty target is stored in each
// block header using a compact representation as described in the documentation
// for CompactToBig.  The main chain is selected by choosing the chain that has
// the most proof of work (highest difficulty).  Since a lower target difficulty
// value equates to higher actual difficulty, the work value which will be
// accumulated must be the inverse of the difficulty.  Also, in order to avoid
// potential division by zero and really small floating point numbers, the
// result adds 1 to the denominator and multiplies the numerator by 2^256.
func CalcWork(bits uint32) *big.Int {
	return workmath.CalcWork(bits)
}

// calcNextRequiredDifficulty calculates the required difficulty for the next
// block.  modelOS uses a two-phase approach:
//
//   - Phase 1 (blocks 0–15860): WTEMA — identical to Pearl, preserving the
//     cold-start treasury accumulation window. (See colossusAuxPowActivationBlock
//     in aserti.go.)
//   - Phase 2 (blocks 15861+): Colossus 2.0 = pure absolute ASERT (aserti3-2d,
//     integer math) — smooth, drift-free, no float in consensus. See aserti.go.
//     (Colossus 1.0, a Median-144 + ASERT hybrid, was removed: simulation showed
//     it telescoped and oscillated >1000% under hashrate steps — daa_sim.py.)
func calcNextRequiredDifficulty(lastNode chaincfg.HeaderCtx, newBlockTime time.Time,
	c ChainCtx) (uint32, error) {

	// Genesis block or no retargeting - use minimum difficulty (maximum target).
	if lastNode == nil || c.ChainParams().PoWNoRetargeting {
		return c.ChainParams().PowLimitBits, nil
	}

	// For networks that support it (panics on mainnet), allow special reduction of the
	// required difficulty once too much time has elapsed without
	// mining a block.
	if c.ChainParams().ReduceMinDifficulty {
		if c.ChainParams().Net == wire.MainNet {
			panic("ReduceMinDifficulty should not be true on mainnet")
		}

		reductionTime := int64(c.ChainParams().MinDiffReductionTime / time.Second)
		allowMinTime := lastNode.Timestamp() + reductionTime
		if newBlockTime.Unix() > allowMinTime {
			return c.ChainParams().PowLimitBits, nil
		}
	}

	// Phase 2: pure absolute ASERT (aserti3-2d) activates at colossusAuxPowActivationBlock
	// (15860). The block AT the activation height is the ASERT anchor (set by
	// Phase-1 WTEMA); ASERT governs every block after it, so the anchor always
	// exists when this runs. Replaces the earlier Colossus median+ASERT hybrid,
	// which simulation proved oscillated >1000% under hashrate steps (see aserti.go
	// and blockchain/daa_sim.py).
	if lastNode.Height() >= colossusAuxPowActivationBlock {
		newTarget := calcNextAsertTarget(lastNode, c.ChainParams().PowLimit)
		newTargetBits := BigToCompact(newTarget)
		log.Debugf("ASERT target %08x (%064x) at height %d",
			newTargetBits, newTarget, lastNode.Height()+1)
		return newTargetBits, nil
	}

	// Phase 1: Pearl-identical WTEMA.
	parentNode := lastNode.Parent()
	if parentNode == nil {
		return lastNode.Bits(), nil
	}

	t := lastNode.Timestamp() - parentNode.Timestamp()
	T := int64(c.ChainParams().TargetTimePerBlock / time.Second)
	halfLife := int64(c.ChainParams().WTEMAHalfLife / time.Second)

	oldTarget := CompactToBig(lastNode.Bits())
	adjustment := new(big.Int).Mul(big.NewInt(t-T), oldTarget)
	adjustment.Div(adjustment, big.NewInt(halfLife))
	newTarget := new(big.Int).Add(oldTarget, adjustment)

	if newTarget.Cmp(c.ChainParams().PowLimit) > 0 {
		newTarget.Set(c.ChainParams().PowLimit)
	}
	if newTarget.Sign() <= 0 {
		newTarget.SetInt64(1)
	}

	newTargetBits := BigToCompact(newTarget)
	log.Debugf("WTEMA difficulty adjustment at block height %d", lastNode.Height()+1)
	log.Debugf("Old target %08x (%064x)", lastNode.Bits(), oldTarget)
	log.Debugf("New target %08x (%064x)", newTargetBits, CompactToBig(newTargetBits))
	log.Debugf("Block time: %v seconds (target: %v seconds)", t, T)

	return newTargetBits, nil
}

// calcEasiestDifficulty calculates the easiest possible difficulty that a block
// can have given starting difficulty bits and a duration. It is used in
// ProcessBlock to verify that claimed proof of work is sane compared to a
// known good checkpoint, before the block is fully validated or cached as an
// orphan.
//
// For WTEMA, the maximum target growth over duration D is bounded by
// exp(D / halfLife). Each block multiplies the target by (1 + (t-T)/HL), and
// the attacker-optimal distribution of block times converges to exp(D/HL) in
// the continuous limit. We approximate this per half-life using the rational
// upper bound 87/32 = 2.71875, which exceeds e = 2.71828... by 0.017%.
// Ceiling integer division ensures we never underestimate the true bound.
//
// This mirrors the structure of btcd's original calcEasiestDifficulty, which
// iterated with a 4x multiplier per retarget period for Bitcoin's 2016-block
// difficulty adjustment. Here the period is WTEMAHalfLife and the multiplier
// is 87/32 instead.
func (b *BlockChain) calcEasiestDifficulty(bits uint32, duration time.Duration) uint32 {
	durationVal := int64(duration / time.Second)

	// Test networks allow minimum-difficulty blocks after a prolonged gap.
	// If the elapsed time exceeds that threshold, any difficulty is
	// reachable so return the easiest possible value immediately.
	if b.chainParams.ReduceMinDifficulty {
		reductionTime := int64(b.chainParams.MinDiffReductionTime /
			time.Second)
		if durationVal > reductionTime {
			return b.chainParams.PowLimitBits
		}
	}

	halfLifeSec := int64(b.chainParams.WTEMAHalfLife / time.Second)
	newTarget := CompactToBig(bits)

	if durationVal > 0 {
		// Number of half-life periods, rounded up so any partial period
		// is conservatively counted as full.
		periods := (durationVal + halfLifeSec - 1) / halfLifeSec

		// (87/32)^178 > e^178 > 2^256, which overflows any target.
		// Short-circuit to avoid computing a huge exponent for nothing.
		if periods > 177 {
			return b.chainParams.PowLimitBits
		}

		// newTarget = newTarget * (87/32)^periods.
		// 87/32 = 2.71875 > e = 2.71828, so this exceeds the true
		// continuous-time bound exp(D/halfLife) at every period count.
		// The floor from the right-shift loses < 1 part in 2^200 for
		// any realistic target, well within the 0.017% margin of 87/32
		// over e.
		pow87 := new(big.Int).Exp(big.NewInt(87), big.NewInt(periods), nil)
		newTarget.Mul(newTarget, pow87)
		newTarget.Rsh(newTarget, uint(5*periods)) // ÷ 32^periods
	}

	if newTarget.Cmp(b.chainParams.PowLimit) > 0 {
		newTarget.Set(b.chainParams.PowLimit)
	}

	return BigToCompact(newTarget)
}

// CalcNextRequiredDifficulty calculates the required difficulty for the block
// after the end of the current best chain based on the WTEMA difficulty
// adjustment algorithm.
//
// This function is safe for concurrent access.
func (b *BlockChain) CalcNextRequiredDifficulty(timestamp time.Time) (uint32, error) {
	b.chainLock.Lock()
	difficulty, err := calcNextRequiredDifficulty(b.bestChain.Tip(), timestamp, b)
	b.chainLock.Unlock()
	return difficulty, err
}
