package blockchain

import (
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/chaincfg"
)

// TestEmissionSchedule verifies the piecewise (inference-block) emission schedule:
// flat reward for the first inferenceBlocks blocks, then the standard curve resumes, with the
// 21,000,000 MDL total supply preserved.
func TestEmissionSchedule(t *testing.T) {
	// Phase 1 — inference blocks pay the flat reward.
	for _, h := range []int32{1, 100, inferenceBlocks} {
		if got := CalcBlockSubsidy(h, &chaincfg.MainNetParams); got != inferenceRewardGrains {
			t.Errorf("Height %d: expected inference reward %d grains, got %d", h, inferenceRewardGrains, got)
		}
	}

	// Phase 1 cumulative at the boundary = inferenceBlocks × reward (600,000 MDL for 240 × 2,500).
	wantPhase1 := int64(inferenceBlocks) * inferenceRewardGrains
	if c := calculateCumulativeSupply(inferenceBlocks); c != wantPhase1 {
		t.Errorf("cumulative at block %d: expected %d grains, got %d", inferenceBlocks, wantPhase1, c)
	}
	fmt.Printf("\n=== Inference-block schedule ===\n")
	fmt.Printf("Phase 1: %d blocks × %.0f MDL = %.0f MDL\n",
		inferenceBlocks, float64(inferenceRewardGrains)/float64(btcutil.GrainPerMDL),
		float64(wantPhase1)/float64(btcutil.GrainPerMDL))

	// Phase 2 — standard curve resumes: first block far below the inference reward, and > 0.
	first := CalcBlockSubsidy(inferenceBlocks+1, &chaincfg.MainNetParams)
	if first <= 0 || first >= inferenceRewardGrains {
		t.Errorf("Phase-2 first block subsidy %d should be >0 and far below the inference reward %d",
			first, inferenceRewardGrains)
	}
	fmt.Printf("Phase 2 starts at block %d: %.2f MDL/block\n",
		inferenceBlocks+1, float64(first)/float64(btcutil.GrainPerMDL))

	// Total supply preserved: cumulative never exceeds 21,000,000 MDL.
	if c := calculateCumulativeSupply(int32(math.MaxInt32)); c > int64(totalSupply) {
		t.Errorf("cumulative supply %d exceeds the 21M cap %d", c, int64(totalSupply))
	}
}

// formatPearl formats a Pearl amount for display
func formatPearl(amount float64) string {
	if amount >= 1e9 {
		return fmt.Sprintf("%.2f billion", amount/1e9)
	} else if amount >= 1e6 {
		return fmt.Sprintf("%.2f million", amount/1e6)
	} else if amount >= 1e3 {
		return fmt.Sprintf("%.2f thousand", amount/1e3)
	}
	return fmt.Sprintf("%.2f", amount)
}

// TestEmissionDecline verifies the inference phase is flat, then the curve never increases.
func TestEmissionDecline(t *testing.T) {
	// Phase 1 — inference blocks are FLAT at the inference reward.
	for height := int32(1); height <= inferenceBlocks; height++ {
		if got := CalcBlockSubsidy(height, &chaincfg.MainNetParams); got != inferenceRewardGrains {
			t.Fatalf("Height %d: inference phase should be flat %d, got %d", height, inferenceRewardGrains, got)
		}
	}

	// Phase 2 — standard curve resumes and must never increase block-to-block.
	previousSubsidy := CalcBlockSubsidy(inferenceBlocks+1, &chaincfg.MainNetParams)
	for height := inferenceBlocks + 2; height <= inferenceBlocks+5000; height++ {
		subsidy := CalcBlockSubsidy(height, &chaincfg.MainNetParams)
		if subsidy > previousSubsidy {
			t.Errorf("Height %d: phase-2 subsidy should not increase, got %d > previous %d", height, subsidy, previousSubsidy)
		}
		previousSubsidy = subsidy
	}

	// Milestones never increase either.
	milestones := []int32{1000, 10000, 100000, 650226, 1300452, 1950678, 3251130}
	previousSubsidy = -1
	for _, height := range milestones {
		subsidy := CalcBlockSubsidy(height, &chaincfg.MainNetParams)
		if previousSubsidy != -1 && subsidy > previousSubsidy {
			t.Errorf("Height %d: subsidy should not increase, got %d > previous %d", height, subsidy, previousSubsidy)
		}
		previousSubsidy = subsidy
	}
}

// TestGenesisBlock verifies genesis block has no subsidy
func TestGenesisBlock(t *testing.T) {
	subsidy := CalcBlockSubsidy(0, &chaincfg.MainNetParams)
	if subsidy != 0 {
		t.Errorf("Genesis block (height 0) should have 0 subsidy, got %d", subsidy)
	}
}

func Test50PercentAtEmissionConstant(t *testing.T) {
	// With the inference-block offset, the standard curve's 50% point (E curve-blocks) lands at
	// actual height E − inferencePhase2Offset.
	height := int32(defaultEmissionConstant - inferencePhase2Offset)
	cumulativeSupply := calculateCumulativeSupply(height)

	totalSupplyValue := totalSupply
	percentage := float64(cumulativeSupply) / float64(totalSupplyValue) * 100
	cumulativePearl := float64(cumulativeSupply) / float64(btcutil.GrainPerMDL)

	fmt.Printf("\n=== 50%% point (Height %d) ===\n", height)
	fmt.Printf("Cumulative supply:     %s\n", formatPearl(cumulativePearl))
	fmt.Printf("Percentage:            %.4f%%\n", percentage)

	if percentage < 49.9 || percentage > 50.1 {
		t.Errorf("At height %d, expected ~50%% circulating, got %.4f%%", height, percentage)
	}
	if cumulativePearl < 1.05e7*0.999 || cumulativePearl > 1.05e7*1.001 {
		t.Errorf("At height %d, expected ~10.5 million MDL, got %.4f million",
			height, cumulativePearl/1e6)
	}
}

// curveCumulative returns the standard-curve cumulative supply at curve-height n:
// totalSupply × n / (n + defaultEmissionConstant).
func curveCumulative(n int64) int64 {
	if n <= 0 {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(totalSupply), big.NewInt(n))
	denominator := big.NewInt(n + defaultEmissionConstant)
	return new(big.Int).Div(numerator, denominator).Int64()
}

// calculateCumulativeSupply reflects the piecewise (inference-block) schedule:
// flat reward for the first inferenceBlocks blocks, then the standard curve resumed at the
// inferencePhase2Offset so the 21M total is preserved.
func calculateCumulativeSupply(height int32) int64 {
	if height == 0 {
		return 0
	}
	if height <= inferenceBlocks {
		return int64(height) * inferenceRewardGrains
	}
	phase1 := int64(inferenceBlocks) * inferenceRewardGrains
	boundary := int64(inferenceBlocks) + inferencePhase2Offset
	phase2 := curveCumulative(int64(height)+inferencePhase2Offset) - curveCumulative(boundary)
	return phase1 + phase2
}

// TestCumulativeSupplyFormula verifies the cumulative-supply helper matches the actual
// block-by-block sum of CalcBlockSubsidy under the piecewise schedule.
func TestCumulativeSupplyFormula(t *testing.T) {
	testHeights := []int32{1, 100, inferenceBlocks, inferenceBlocks + 1, 1000, 10000}

	fmt.Println("\n=== Cumulative Supply Verification (helper vs summed) ===")
	fmt.Printf("%-15s %-22s %-22s\n", "Height", "Helper (MDL)", "Summed (MDL)")
	fmt.Println("------------------------------------------------------------")

	tol := int64(btcutil.GrainPerMDL) // within 1 MDL (closed-form tail vs summed integer rounding)
	for _, height := range testHeights {
		var summed int64
		for h := int32(1); h <= height; h++ {
			summed += CalcBlockSubsidy(h, &chaincfg.MainNetParams)
		}
		helper := calculateCumulativeSupply(height)

		fmt.Printf("%-15d %-22.4f %-22.4f\n",
			height, float64(helper)/float64(btcutil.GrainPerMDL), float64(summed)/float64(btcutil.GrainPerMDL))

		diff := helper - summed
		if diff < 0 {
			diff = -diff
		}
		if diff > tol {
			t.Errorf("Height %d: helper %d != summed %d (diff %d grains)", height, helper, summed, diff)
		}
	}
}

// TestGrainPrecision verifies that subsidies are always whole grains (no fractions)
func TestGrainPrecision(t *testing.T) {
	fmt.Println("\n=== Grain Precision Test ===")
	fmt.Printf("Verifying that all subsidies are whole grains (integers)\n")
	fmt.Printf("1 Pearl = 100,000,000 grains\n\n")

	// Test various heights to ensure subsidy is always an integer
	testHeights := []int32{1, 10, 100, 1000, 10000, 100000, 650226, 1300452, 3000000, 10000000}

	fmt.Printf("%-15s %-25s %-20s\n", "Height", "Subsidy (grains)", "Subsidy (Pearl)")
	fmt.Println("---------------------------------------------------------------")

	for _, height := range testHeights {
		subsidy := CalcBlockSubsidy(height, &chaincfg.MainNetParams)
		formatPearl := float64(subsidy) / float64(btcutil.GrainPerMDL)

		fmt.Printf("%-15d %-25d %-20.8f\n", height, subsidy, formatPearl)

		// Verify subsidy is a whole number (this should always pass since it's an int64)
		if subsidy < 0 {
			t.Errorf("Height %d: subsidy should never be negative, got %d", height, subsidy)
		}
	}

	fmt.Println("\n✓ All subsidies are whole grains (no fractional values)")
}

// TestSubsidyBecomesZero verifies when the subsidy drops below 1 grain
func TestSubsidyBecomesZero(t *testing.T) {
	fmt.Println("\n=== Finding When Subsidy Becomes Zero ===")
	fmt.Println("Searching for the block height where subsidy drops to 0...")

	// The subsidy formula is: totalSupply × emissionConstant / [(h + emissionConstant) × (h + emissionConstant - 1)]
	// It becomes 0 when: totalSupply × emissionConstant < (h + emissionConstant) × (h + emissionConstant - 1)
	// For a rough estimate, when h is very large: h² ≈ totalSupply × emissionConstant
	// h ≈ sqrt(totalSupply × emissionConstant)

	totalSupplyValue := int64(21000000) * int64(btcutil.GrainPerMDL)
	emissionConstValue := defaultEmissionConstant

	// Approximate starting point (limited to int32 max)
	approxHeight := int64(math.Sqrt(float64(totalSupplyValue) * float64(emissionConstValue)))
	fmt.Printf("Theoretical height estimate: %d\n", approxHeight)

	// Since int32 max is ~2.1B, we need to search within that range
	// Start by testing if subsidy is still positive at int32 max
	maxInt32 := int32(math.MaxInt32)
	subsidyAtMax := CalcBlockSubsidy(maxInt32, &chaincfg.MainNetParams)

	fmt.Printf("Subsidy at int32 max (%d): %d grains\n", maxInt32, subsidyAtMax)

	if subsidyAtMax == 0 {
		// Binary search from 1 to int32 max
		low := int32(1)
		high := maxInt32
		lastNonZeroHeight := int32(0)

		iterations := 0
		for low <= high {
			mid := low + (high-low)/2
			subsidy := CalcBlockSubsidy(mid, &chaincfg.MainNetParams)
			iterations++

			if subsidy > 0 {
				lastNonZeroHeight = mid
				low = mid + 1
			} else {
				high = mid - 1
			}

			if iterations%5 == 0 {
				fmt.Printf("  Iteration %d: testing height %d (subsidy: %d grains)\n", iterations, mid, subsidy)
			}
		}

		fmt.Printf("Search completed in %d iterations\n", iterations)

		// Check a few blocks around the boundary
		fmt.Printf("\n%-15s %-25s %-20s\n", "Height", "Subsidy (grains)", "Subsidy (Pearl)")
		fmt.Println("---------------------------------------------------------------")

		for offset := int32(-2); offset <= 2; offset++ {
			height := lastNonZeroHeight + offset
			if height < 1 {
				continue
			}
			subsidy := CalcBlockSubsidy(height, &chaincfg.MainNetParams)
			subsidyPearl := float64(subsidy) / float64(btcutil.GrainPerMDL)

			marker := ""
			if subsidy > 0 && CalcBlockSubsidy(height+1, &chaincfg.MainNetParams) == 0 {
				marker = " <- Last non-zero subsidy"
			} else if subsidy == 0 && CalcBlockSubsidy(height-1, &chaincfg.MainNetParams) > 0 {
				marker = " <- First zero subsidy"
			}

			fmt.Printf("%-15d %-25d %-20.8f%s\n", height, subsidy, subsidyPearl, marker)
		}

		// Calculate what percentage this represents
		percentage := float64(lastNonZeroHeight) / float64(int64(lastNonZeroHeight)+defaultEmissionConstant) * 100
		cumulative := calculateCumulativeSupply(lastNonZeroHeight)
		cumulativePearl := float64(cumulative) / float64(btcutil.GrainPerMDL)

		fmt.Printf("\nLast non-zero subsidy at height: %d\n", lastNonZeroHeight)
		fmt.Printf("Circulating supply at that point: %s (%.6f%%)\n",
			formatPearl(cumulativePearl), percentage)

		// Calculate remaining supply that will never be mined
		remainingSupply := totalSupplyValue - cumulative
		remainingPearl := float64(remainingSupply) / float64(btcutil.GrainPerMDL)
		remainingPercent := float64(remainingSupply) / float64(totalSupplyValue) * 100

		fmt.Printf("Supply that will never be mined: %s (%.6f%%)\n",
			formatPearl(remainingPearl), remainingPercent)

		// Verify the last non-zero height is positive
		if lastNonZeroHeight <= 0 {
			t.Errorf("Expected to find a positive height with non-zero subsidy")
		}
	} else {
		// Subsidy is still positive at int32 max, so emission continues beyond int32 range
		lastNonZeroHeight := maxInt32
		fmt.Printf("\nNote: Subsidy is still positive at int32 max!\n")
		fmt.Printf("Emission continues beyond block height %d\n", maxInt32)

		// Show subsidy at various heights approaching max
		fmt.Printf("\n%-15s %-25s %-20s\n", "Height", "Subsidy (grains)", "Subsidy (Pearl)")
		fmt.Println("---------------------------------------------------------------")

		testHeights := []int32{
			maxInt32 - 1000000000,
			maxInt32 - 100000000,
			maxInt32 - 10000000,
			maxInt32 - 1000000,
			maxInt32,
		}

		for _, height := range testHeights {
			if height < 1 {
				continue
			}
			subsidy := CalcBlockSubsidy(height, &chaincfg.MainNetParams)
			subsidyPearl := float64(subsidy) / float64(btcutil.GrainPerMDL)

			fmt.Printf("%-15d %-25d %-20.8f\n", height, subsidy, subsidyPearl)
		}

		// Calculate what percentage this represents
		percentage := float64(lastNonZeroHeight) / float64(int64(lastNonZeroHeight)+defaultEmissionConstant) * 100
		cumulative := calculateCumulativeSupply(lastNonZeroHeight)
		cumulativePearl := float64(cumulative) / float64(btcutil.GrainPerMDL)

		fmt.Printf("\nAt int32 max height: %d\n", lastNonZeroHeight)
		fmt.Printf("Circulating supply: %s (%.6f%%)\n",
			formatPearl(cumulativePearl), percentage)

		// Note: No "supply that will never be mined" since emission continues
		fmt.Printf("\nEmission continues indefinitely beyond int32 range.\n")
	}
}
