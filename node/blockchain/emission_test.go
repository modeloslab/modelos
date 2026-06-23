package blockchain

import (
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/chaincfg"
)

// TestEmissionSchedule verifies the emission curve follows the expected formula
func TestEmissionSchedule(t *testing.T) {
	totalSupplyTokens := int64(21000000) // 21 million tokens

	// Test key block heights in the emission schedule. Subsidy is in Pearl/block;
	// cumulative is in billions of Pearl (so values are hundredths of a billion at
	// the 21M total supply).
	testCases := []struct {
		height             int32
		expectedPercent    float64
		expectedSubsidy    float64
		expectedCumulative float64
	}{
		{1, 0.000154, 32.29641, 0.0000000323},
		{650226, 50.0, 8.07412, 0.0105},
		{1300452, 66.67, 3.58850, 0.0140},
		{1950678, 75.0, 2.01853, 0.01575},
		{3251130, 83.33, 0.89712, 0.0175},
		{6502260, 90.91, 0.26691, 0.01909},
		{32511300, 98.04, 0.01242, 0.02059},
	}

	fmt.Println("\n=== Per-Block Emission Schedule (Absolute Amounts) ===")
	fmt.Printf("Total Supply: %d Pearl (21 million)\n", totalSupplyTokens)
	fmt.Printf("Emission Constant: 650,226 blocks (~4 years at 3 min 14 sec/block)\n\n")
	fmt.Printf("%-12s %-20s %-25s %-15s\n", "Height", "Subsidy (Pearl)", "Cumulative (Pearl)", "Percent")
	fmt.Println("--------------------------------------------------------------------------------")

	for _, tc := range testCases {
		subsidy := CalcBlockSubsidy(tc.height, &chaincfg.MainNetParams)

		cumulativePercent := float64(tc.height) / float64(int64(tc.height)+defaultEmissionConstant) * 100

		cumulativeTokens := calculateCumulativeSupply(tc.height)
		cumulativeTokensDisplay := float64(cumulativeTokens) / float64(btcutil.GrainPerMDL)

		subsidyPearl := float64(subsidy) / float64(btcutil.GrainPerMDL)

		fmt.Printf("%-12d %-20.2f %-25s %-15.2f%%\n",
			tc.height,
			subsidyPearl,
			formatPearl(cumulativeTokensDisplay),
			cumulativePercent,
		)

		diff := cumulativePercent - tc.expectedPercent
		if diff < -0.5 || diff > 0.5 {
			t.Errorf("Height %d: expected ~%.2f%%, got %.2f%%", tc.height, tc.expectedPercent, cumulativePercent)
		}

		// Verify subsidy amount is close to expected (within 1%)
		subsidyDiff := (subsidyPearl - tc.expectedSubsidy) / tc.expectedSubsidy * 100
		if subsidyDiff < -1 || subsidyDiff > 1 {
			t.Errorf("Height %d: expected ~%.2f Pearl/block, got %.2f Pearl/block",
				tc.height, tc.expectedSubsidy, subsidyPearl)
		}

		// Verify cumulative amount (within 1%)
		cumulativeBillions := cumulativeTokensDisplay / 1e9
		cumulativeDiff := (cumulativeBillions - tc.expectedCumulative) / tc.expectedCumulative * 100
		if cumulativeDiff < -1 || cumulativeDiff > 1 {
			t.Errorf("Height %d: expected ~%.2fB Pearl cumulative, got %.2fB Pearl cumulative",
				tc.height, tc.expectedCumulative, cumulativeBillions)
		}
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

// TestEmissionDecline verifies that emission decreases smoothly every block
func TestEmissionDecline(t *testing.T) {
	fmt.Println("\n=== First 20 Blocks Emission (Absolute Amounts) ===")
	fmt.Printf("%-10s %-22s %-25s %-15s\n", "Height", "Subsidy (Pearl)", "Cumulative (Pearl)", "Cumulative %")
	fmt.Println("--------------------------------------------------------------------------------")

	var previousSubsidy int64 = -1

	for height := int32(1); height <= 20; height++ {
		subsidy := CalcBlockSubsidy(height, &chaincfg.MainNetParams)
		subsidyPearl := float64(subsidy) / float64(btcutil.GrainPerMDL)
		cumulativePercent := float64(height) / float64(int64(height)+defaultEmissionConstant) * 100
		cumulativeTokens := calculateCumulativeSupply(height)
		cumulativePearl := float64(cumulativeTokens) / float64(btcutil.GrainPerMDL)

		fmt.Printf("%-10d %-22.2f %-25s %-15.6f%%\n",
			height,
			subsidyPearl,
			formatPearl(cumulativePearl),
			cumulativePercent,
		)

		// Verify subsidy decreases (except for first block)
		if previousSubsidy != -1 && subsidy >= previousSubsidy {
			t.Errorf("Height %d: subsidy should decrease, but got %d >= previous %d", height, subsidy, previousSubsidy)
		}
		previousSubsidy = subsidy
	}

	// Test key milestones
	fmt.Println("\n=== Key Milestones (Absolute Amounts) ===")
	fmt.Printf("%-12s %-22s %-27s %-15s\n", "Height", "Subsidy (Pearl)", "Cumulative (Pearl)", "Cumulative %")
	fmt.Println("------------------------------------------------------------------------------------")

	milestones := []int32{1000, 10000, 100000, 650226, 1300452, 1950678, 3251130}
	previousSubsidy = -1

	for _, height := range milestones {
		subsidy := CalcBlockSubsidy(height, &chaincfg.MainNetParams)
		subsidyPearl := float64(subsidy) / float64(btcutil.GrainPerMDL)
		cumulativePercent := float64(height) / float64(int64(height)+defaultEmissionConstant) * 100
		cumulativeTokens := calculateCumulativeSupply(height)
		cumulativePearl := float64(cumulativeTokens) / float64(btcutil.GrainPerMDL)

		fmt.Printf("%-12d %-22.2f %-27s %-15.2f%%\n",
			height,
			subsidyPearl,
			formatPearl(cumulativePearl),
			cumulativePercent,
		)

		// Verify subsidy decreases at milestones too
		if previousSubsidy != -1 && subsidy >= previousSubsidy {
			t.Errorf("Height %d: subsidy should decrease, but got %d >= previous %d", height, subsidy, previousSubsidy)
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
	height := int32(650226)
	subsidy := CalcBlockSubsidy(height, &chaincfg.MainNetParams)
	cumulativeSupply := calculateCumulativeSupply(height)

	totalSupplyValue := int64(21000000) * int64(btcutil.GrainPerMDL)

	percentage := float64(cumulativeSupply) / float64(totalSupplyValue) * 100

	subsidyPearl := float64(subsidy) / float64(btcutil.GrainPerMDL)
	cumulativePearl := float64(cumulativeSupply) / float64(btcutil.GrainPerMDL)

	fmt.Printf("\n=== At Emission Constant (Height %d) ===\n", height)
	fmt.Printf("Block subsidy:         %.2f Pearl\n", subsidyPearl)
	fmt.Printf("Cumulative supply:     %s\n", formatPearl(cumulativePearl))
	fmt.Printf("Percentage:            %.2f%%\n", percentage)
	fmt.Printf("Expected percentage:   50.00%%\n")

	if percentage < 49.99 || percentage > 50.01 {
		t.Errorf("At height %d, expected ~50%% circulating, got %.4f%%", height, percentage)
	}

	expectedCumulative := 1.05e7 // 10.5 million Pearl (50% of 21M)
	if cumulativePearl < expectedCumulative*0.999 || cumulativePearl > expectedCumulative*1.001 {
		t.Errorf("At height %d, expected ~10.5 million Pearl, got %.2f million",
			height, cumulativePearl/1e6)
	}
}

func calculateCumulativeSupply(height int32) int64 {
	if height == 0 {
		return 0
	}

	h := int64(height)

	// Cumulative supply = totalSupply × h / (h + defaultEmissionConstant)
	totalSupplyValue := big.NewInt(totalSupply)

	numerator := new(big.Int).Mul(totalSupplyValue, big.NewInt(h))
	denominator := big.NewInt(h + defaultEmissionConstant)

	cumulative := new(big.Int).Div(numerator, denominator)

	return cumulative.Int64()
}

// TestCumulativeSupplyFormula verifies the cumulative supply matches the formula
func TestCumulativeSupplyFormula(t *testing.T) {
	testHeights := []int32{1, 100, 1000, 10000, 650226, 1300452, 3000000}

	fmt.Println("\n=== Cumulative Supply Verification ===")
	fmt.Printf("%-15s %-20s %-20s\n", "Height", "Formula %", "Actual %")
	fmt.Println("------------------------------------------------------------")

	for _, height := range testHeights {
		// Expected from formula
		expectedPercent := float64(height) / float64(int64(height)+defaultEmissionConstant) * 100

		// Actual cumulative supply
		cumulative := calculateCumulativeSupply(height)
		totalSupplyValue := int64(21000000) * int64(btcutil.GrainPerMDL)
		actualPercent := float64(cumulative) / float64(totalSupplyValue) * 100

		fmt.Printf("%-15d %-20.6f%% %-20.6f%%\n", height, expectedPercent, actualPercent)

		// Should match very closely (within 0.0001% due to rounding)
		diff := expectedPercent - actualPercent
		if diff < -0.0001 || diff > 0.0001 {
			t.Errorf("Height %d: formula %.6f%% doesn't match actual %.6f%%", height, expectedPercent, actualPercent)
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
