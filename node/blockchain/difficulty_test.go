// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHeaderCtx implements chaincfg.HeaderCtx for testing.
type mockHeaderCtx struct {
	hash      chainhash.Hash
	height    int32
	bits      uint32
	timestamp int64
	parent    *mockHeaderCtx
}

func (m *mockHeaderCtx) Hash() chainhash.Hash {
	return m.hash
}

func (m *mockHeaderCtx) Height() int32 {
	return m.height
}

func (m *mockHeaderCtx) Bits() uint32 {
	return m.bits
}

func (m *mockHeaderCtx) Timestamp() int64 {
	return m.timestamp
}

func (m *mockHeaderCtx) Parent() chaincfg.HeaderCtx {
	if m.parent == nil {
		return nil
	}
	return m.parent
}

func (m *mockHeaderCtx) RelativeAncestorCtx(distance int32) chaincfg.HeaderCtx {
	node := m
	for i := int32(0); i < distance && node != nil; i++ {
		node = node.parent
	}
	if node == nil {
		return nil
	}
	return node
}

// mockChainCtx implements ChainCtx for testing.
type mockChainCtx struct {
	params *chaincfg.Params
}

func (m *mockChainCtx) ChainParams() *chaincfg.Params {
	return m.params
}

func (m *mockChainCtx) VerifyCheckpoint(height int32, hash *chainhash.Hash) bool {
	return true
}

func (m *mockChainCtx) FindPreviousCheckpoint() (chaincfg.HeaderCtx, error) {
	return nil, nil
}

// newTestChainCtx creates a mock chain context for testing.
func newTestChainCtx() *mockChainCtx {
	// Use simnet params for testing (easy PoW limit).
	params := &chaincfg.Params{
		PowLimit:             new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(1)),
		PowLimitBits:         0x207fffff,
		PoWNoRetargeting:     false,
		ReduceMinDifficulty:  false,
		TargetTimePerBlock:   (time.Minute * 3) + (time.Second * 14), // 3 Minutes and 14 seconds
		WTEMAHalfLife:        time.Hour * 48,                         // 2 days
		MinDiffReductionTime: (time.Minute * 6) + (time.Second * 28), // TargetTimePerBlock * 2
	}
	return &mockChainCtx{
		params: params,
	}
}

// TestWTEMADifficultyGenesis tests that the genesis block returns PowLimitBits.
func TestWTEMADifficultyGenesis(t *testing.T) {
	ctx := newTestChainCtx()

	// Genesis block case - no previous node.
	bits, err := calcNextRequiredDifficulty(nil, time.Now(), ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bits != ctx.params.PowLimitBits {
		t.Errorf("genesis block bits = %d, want %d", bits, ctx.params.PowLimitBits)
	}
}

// TestWTEMADifficultyFirstBlock tests the first block after genesis.
func TestWTEMADifficultyFirstBlock(t *testing.T) {
	ctx := newTestChainCtx()

	// Create genesis block (no parent).
	genesis := &mockHeaderCtx{
		height:    0,
		bits:      ctx.params.PowLimitBits,
		timestamp: time.Now().Unix(),
		parent:    nil,
	}

	// First block after genesis should return genesis difficulty.
	bits, err := calcNextRequiredDifficulty(genesis, time.Now(), ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bits != genesis.bits {
		t.Errorf("first block bits = %d, want %d", bits, genesis.bits)
	}
}

// TestWTEMADifficultyExactTarget tests when block time equals target time.
func TestWTEMADifficultyExactTarget(t *testing.T) {
	ctx := newTestChainCtx()
	T := int64(ctx.params.TargetTimePerBlock / time.Second) // 600 seconds

	genesis := &mockHeaderCtx{
		height:    0,
		bits:      0x1d00ffff, // Some difficulty
		timestamp: 1000000,
		parent:    nil,
	}

	block1 := &mockHeaderCtx{
		height:    1,
		bits:      0x1d00ffff,
		timestamp: genesis.timestamp + T, // Exactly target time
		parent:    genesis,
	}

	// When t = T, the formula gives: new_target = ((1-C) + C*1) * target = target
	// So difficulty should remain roughly the same.
	bits, err := calcNextRequiredDifficulty(block1, time.Unix(block1.timestamp+T, 0), ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the difficulty is approximately the same (within compact representation precision).
	oldTarget := CompactToBig(block1.bits)
	newTarget := CompactToBig(bits)

	// Allow 0.1% difference due to integer arithmetic.
	diff := new(big.Int).Sub(newTarget, oldTarget)
	diff.Abs(diff)
	tolerance := new(big.Int).Div(oldTarget, big.NewInt(1000))
	if diff.Cmp(tolerance) > 0 {
		t.Errorf("difficulty changed too much: old=%x, new=%x", oldTarget, newTarget)
	}
}

// TestWTEMADifficultySlowBlock tests when block time is longer than target.
func TestWTEMADifficultySlowBlock(t *testing.T) {
	ctx := newTestChainCtx()
	T := int64(ctx.params.TargetTimePerBlock / time.Second) // 600 seconds

	genesis := &mockHeaderCtx{
		height:    0,
		bits:      0x1d00ffff,
		timestamp: 1000000,
		parent:    nil,
	}

	block1 := &mockHeaderCtx{
		height:    1,
		bits:      0x1d00ffff,
		timestamp: genesis.timestamp + 2*T, // Double target time (slow block)
		parent:    genesis,
	}

	bits, err := calcNextRequiredDifficulty(block1, time.Unix(block1.timestamp+T, 0), ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// When t > T, difficulty should decrease (target increases).
	oldTarget := CompactToBig(block1.bits)
	newTarget := CompactToBig(bits)

	if newTarget.Cmp(oldTarget) <= 0 {
		t.Errorf("expected target to increase for slow block: old=%x, new=%x", oldTarget, newTarget)
	}
}

// TestWTEMADifficultyFastBlock tests when block time is shorter than target.
func TestWTEMADifficultyFastBlock(t *testing.T) {
	ctx := newTestChainCtx()
	T := int64(ctx.params.TargetTimePerBlock / time.Second) // 600 seconds

	genesis := &mockHeaderCtx{
		height:    0,
		bits:      0x1d00ffff,
		timestamp: 1000000,
		parent:    nil,
	}

	block1 := &mockHeaderCtx{
		height:    1,
		bits:      0x1d00ffff,
		timestamp: genesis.timestamp + T/2, // Half target time (fast block)
		parent:    genesis,
	}

	bits, err := calcNextRequiredDifficulty(block1, time.Unix(block1.timestamp+T, 0), ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// When t < T, difficulty should increase (target decreases).
	oldTarget := CompactToBig(block1.bits)
	newTarget := CompactToBig(bits)

	if newTarget.Cmp(oldTarget) >= 0 {
		t.Errorf("expected target to decrease for fast block: old=%x, new=%x", oldTarget, newTarget)
	}
}

// TestWTEMADifficultyPowLimit tests that difficulty never exceeds PowLimit.
func TestWTEMADifficultyPowLimit(t *testing.T) {
	ctx := newTestChainCtx()
	T := int64(ctx.params.TargetTimePerBlock / time.Second)

	genesis := &mockHeaderCtx{
		height:    0,
		bits:      ctx.params.PowLimitBits, // Already at minimum difficulty
		timestamp: 1000000,
		parent:    nil,
	}

	block1 := &mockHeaderCtx{
		height:    1,
		bits:      ctx.params.PowLimitBits,
		timestamp: genesis.timestamp + 100*T, // Very slow block
		parent:    genesis,
	}

	bits, err := calcNextRequiredDifficulty(block1, time.Unix(block1.timestamp+T, 0), ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Target should not exceed PowLimit.
	newTarget := CompactToBig(bits)
	if newTarget.Cmp(ctx.params.PowLimit) > 0 {
		t.Errorf("target exceeds PowLimit: got=%x, limit=%x", newTarget, ctx.params.PowLimit)
	}
}

// TestWTEMADifficultyNoRetargeting tests regtest behavior.
func TestWTEMADifficultyNoRetargeting(t *testing.T) {
	ctx := newTestChainCtx()
	ctx.params.PoWNoRetargeting = true

	genesis := &mockHeaderCtx{
		height:    0,
		bits:      0x1d00ffff,
		timestamp: 1000000,
		parent:    nil,
	}

	block1 := &mockHeaderCtx{
		height:    1,
		bits:      0x1d00ffff,
		timestamp: genesis.timestamp + 1, // Very fast block
		parent:    genesis,
	}

	bits, err := calcNextRequiredDifficulty(block1, time.Now(), ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With no retargeting, should return PowLimitBits.
	if bits != ctx.params.PowLimitBits {
		t.Errorf("no retarget bits = %d, want %d", bits, ctx.params.PowLimitBits)
	}
}

// TestWTEMADifficultyMinTimestamp tests minimum timestamp enforcement.
func TestWTEMADifficultyMinTimestamp(t *testing.T) {
	ctx := newTestChainCtx()

	genesis := &mockHeaderCtx{
		height:    0,
		bits:      0x1d00ffff,
		timestamp: 1000000,
		parent:    nil,
	}

	// Block with 0-second interval (should be clamped to 1 second).
	block1 := &mockHeaderCtx{
		height:    1,
		bits:      0x1d00ffff,
		timestamp: genesis.timestamp, // Same timestamp
		parent:    genesis,
	}

	// Should not panic or error.
	bits, err := calcNextRequiredDifficulty(block1, time.Now(), ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Target should decrease (faster mining requires more difficulty).
	oldTarget := CompactToBig(block1.bits)
	newTarget := CompactToBig(bits)

	if newTarget.Cmp(oldTarget) >= 0 {
		t.Errorf("expected target to decrease for fast block: old=%x, new=%x", oldTarget, newTarget)
	}
}

// TestWTEMADifficultyAdjustment tests that WTEMA correctly adjusts in both directions.
// Note: WTEMA is inherently asymmetric - doubling block time doesn't produce the
// same magnitude change as halving it. This is expected behavior for exponential filters.
func TestWTEMADifficultyAdjustment(t *testing.T) {
	ctx := newTestChainCtx()
	T := int64(ctx.params.TargetTimePerBlock / time.Second)

	genesis := &mockHeaderCtx{
		height:    0,
		bits:      0x1c00ffff, // Higher difficulty than PowLimit
		timestamp: 1000000,
		parent:    nil,
	}

	// Slow block: t = 2*T should increase target (decrease difficulty)
	slowBlock := &mockHeaderCtx{
		height:    1,
		bits:      genesis.bits,
		timestamp: genesis.timestamp + 2*T,
		parent:    genesis,
	}

	slowBits, err := calcNextRequiredDifficulty(slowBlock, time.Unix(slowBlock.timestamp+T, 0), ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Fast block: t = T/2 should decrease target (increase difficulty)
	fastBlock := &mockHeaderCtx{
		height:    1,
		bits:      genesis.bits,
		timestamp: genesis.timestamp + T/2,
		parent:    genesis,
	}

	fastBits, err := calcNextRequiredDifficulty(fastBlock, time.Unix(fastBlock.timestamp+T, 0), ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	slowTarget := CompactToBig(slowBits)
	fastTarget := CompactToBig(fastBits)
	origTarget := CompactToBig(genesis.bits)

	// Verify slow block increased target (easier)
	if slowTarget.Cmp(origTarget) <= 0 {
		t.Errorf("slow block should increase target: orig=%x, slow=%x", origTarget, slowTarget)
	}

	// Verify fast block decreased target (harder)
	if fastTarget.Cmp(origTarget) >= 0 {
		t.Errorf("fast block should decrease target: orig=%x, fast=%x", origTarget, fastTarget)
	}
}

func newTestBlockChain(t *testing.T, params *chaincfg.Params) *BlockChain {
	t.Helper()
	return &BlockChain{chainParams: params}
}

// simulateWTEMAChain builds a chain of blocks using calcNextRequiredDifficulty,
// each separated by blockTimeSec seconds, starting from startBits. Returns the
// final difficulty bits and total elapsed seconds.
func simulateWTEMAChain(t *testing.T, ctx *mockChainCtx, startBits uint32, blockTimeSec int64, numBlocks int) (uint32, int64) {
	t.Helper()

	parentOfPrev := &mockHeaderCtx{
		height:    0,
		bits:      startBits,
		timestamp: 0,
		parent:    nil,
	}
	prev := &mockHeaderCtx{
		height:    1,
		bits:      startBits,
		timestamp: blockTimeSec,
		parent:    parentOfPrev,
	}

	bits := startBits
	for i := 1; i < numBlocks; i++ {
		nextTime := time.Unix(prev.timestamp+blockTimeSec, 0)
		var err error
		bits, err = calcNextRequiredDifficulty(prev, nextTime, ctx)
		require.NoError(t, err)
		prev = &mockHeaderCtx{
			height:    prev.height + 1,
			bits:      bits,
			timestamp: prev.timestamp + blockTimeSec,
			parent:    prev,
		}
	}

	totalElapsed := prev.timestamp - parentOfPrev.timestamp
	return bits, totalElapsed
}

func TestCalcEasiestDifficulty(t *testing.T) {
	ctx := newTestChainCtx()
	bc := newTestBlockChain(t, ctx.params)
	startBits := uint32(0x1c00ffff)

	t.Run("monotonicity", func(t *testing.T) {
		durations := []time.Duration{
			0,
			ctx.params.WTEMAHalfLife / 4,
			ctx.params.WTEMAHalfLife / 2,
			ctx.params.WTEMAHalfLife,
			2 * ctx.params.WTEMAHalfLife,
			5 * ctx.params.WTEMAHalfLife,
		}
		prevTarget := new(big.Int)
		for _, d := range durations {
			got := bc.calcEasiestDifficulty(startBits, d)
			gotTarget := CompactToBig(got)
			assert.True(t, gotTarget.Cmp(prevTarget) >= 0,
				"target must not decrease: at %v got %x, prev %x",
				d, gotTarget, prevTarget)
			prevTarget.Set(gotTarget)
		}
	})

	t.Run("identity at zero and negative duration", func(t *testing.T) {
		assert.Equal(t, startBits, bc.calcEasiestDifficulty(startBits, 0))
		assert.Equal(t, startBits, bc.calcEasiestDifficulty(startBits, -time.Hour))
	})

	t.Run("caps at PowLimit", func(t *testing.T) {
		got := bc.calcEasiestDifficulty(startBits, 100*ctx.params.WTEMAHalfLife)
		assert.Equal(t, ctx.params.PowLimitBits, got)
	})

	t.Run("ReduceMinDifficulty above threshold", func(t *testing.T) {
		p := *ctx.params
		p.ReduceMinDifficulty = true
		bc := newTestBlockChain(t, &p)
		got := bc.calcEasiestDifficulty(startBits, p.MinDiffReductionTime+time.Second)
		assert.Equal(t, p.PowLimitBits, got)
	})

	t.Run("ReduceMinDifficulty below threshold still grows", func(t *testing.T) {
		p := *ctx.params
		p.ReduceMinDifficulty = true
		bc := newTestBlockChain(t, &p)
		got := bc.calcEasiestDifficulty(startBits, p.MinDiffReductionTime-time.Second)
		assert.NotEqual(t, p.PowLimitBits, got, "should not shortcut to PowLimitBits")
		gotTarget := CompactToBig(got)
		startTarget := CompactToBig(startBits)
		assert.True(t, gotTarget.Cmp(startTarget) > 0, "target should grow")
	})
}

// TestCalcEasiestDifficulty_BoundsWTEMAChains verifies the core invariant:
// no chain produced by calcNextRequiredDifficulty (under any block-timing
// strategy) can yield a target exceeding the calcEasiestDifficulty bound for
// the same elapsed time. Everything is derived from chain params, so changing
// WTEMAHalfLife or TargetTimePerBlock will break this if the bound is invalid.
func TestCalcEasiestDifficulty_BoundsWTEMAChains(t *testing.T) {
	ctx := newTestChainCtx()
	bc := newTestBlockChain(t, ctx.params)
	T := int64(ctx.params.TargetTimePerBlock / time.Second)
	HL := int64(ctx.params.WTEMAHalfLife / time.Second)

	startBits := uint32(0x1b00ffff)

	tests := []struct {
		name      string
		blockTime int64
		numBlocks int
	}{
		{"slow 10x target", 10 * T, 20},
		{"slow 50x target", 50 * T, 5},
		{"one HL per block", HL, 3},
		{"exact target time", T, 100},
		{"optimal region 3x", 3 * T, int(HL / T)},
		{"single massive block", 5 * HL, 2},
		{"minimum delta spam", MinTimestampDeltaSeconds, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bits, elapsed := simulateWTEMAChain(t, ctx, startBits, tt.blockTime, tt.numBlocks)
			require.Greater(t, elapsed, int64(0))

			chainTarget := CompactToBig(bits)
			boundBits := bc.calcEasiestDifficulty(startBits, time.Duration(elapsed)*time.Second)
			boundTarget := CompactToBig(boundBits)

			assert.True(t, chainTarget.Cmp(boundTarget) <= 0,
				"chain target %x exceeds bound %x (elapsed=%ds, %.2f half-lives)",
				chainTarget, boundTarget, elapsed, float64(elapsed)/float64(HL))
		})
	}
}

// TestCalcEasiestDifficulty_Consistency verifies that the per-half-life
// multiplier used inside calcEasiestDifficulty exceeds the true maximum
// WTEMA growth over one half-life. The true maximum is computed numerically
// from the configured TargetTimePerBlock and WTEMAHalfLife, so any change
// to those params re-validates the bound automatically.
func TestCalcEasiestDifficulty_Consistency(t *testing.T) {
	ctx := newTestChainCtx()
	bc := newTestBlockChain(t, ctx.params)

	T := int64(ctx.params.TargetTimePerBlock / time.Second)
	HL := int64(ctx.params.WTEMAHalfLife / time.Second)

	// Numerically find the worst-case discrete growth over one half-life:
	//   max over N of (1 + (HL/N - T)/HL)^N
	// This approaches e from below for T << HL.
	worstGrowth := 0.0
	for N := int64(1); N <= HL/T+1; N++ {
		perBlock := 1.0 + (float64(HL)/float64(N)-float64(T))/float64(HL)
		if perBlock <= 0 {
			continue
		}
		growth := math.Pow(perBlock, float64(N))
		if growth > worstGrowth {
			worstGrowth = growth
		}
	}

	// Compute the effective multiplier by running calcEasiestDifficulty for
	// exactly one half-life and measuring the ratio.
	startBits := uint32(0x1a00ffff)
	startTarget := new(big.Float).SetInt(CompactToBig(startBits))
	resultTarget := new(big.Float).SetInt(CompactToBig(
		bc.calcEasiestDifficulty(startBits, ctx.params.WTEMAHalfLife),
	))
	effectiveMultiplier, _ := new(big.Float).Quo(resultTarget, startTarget).Float64()

	assert.Greater(t, effectiveMultiplier, worstGrowth,
		"effective multiplier (%.5f) must exceed worst-case WTEMA growth "+
			"(%.5f) for T=%ds, HL=%ds", effectiveMultiplier, worstGrowth, T, HL)

	assert.Greater(t, effectiveMultiplier, math.E,
		"effective multiplier (%.5f) must exceed e (%.5f) since "+
			"exp(D/HL) is the continuous-time upper bound",
		effectiveMultiplier, math.E)

	// Sanity: the multiplier shouldn't be wildly over-conservative either.
	assert.Less(t, effectiveMultiplier, math.E*1.10,
		"effective multiplier (%.5f) is more than 10%% above e; "+
			"consider tightening", effectiveMultiplier)

	t.Logf("worst discrete growth: %.5f, effective multiplier: %.5f, e: %.5f",
		worstGrowth, effectiveMultiplier, math.E)
}
