// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil_test

import (
	"math"
	"testing"

	. "github.com/modelos/modelos/node/btcutil"
)

func TestAmountCreation(t *testing.T) {
	tests := []struct {
		name     string
		amount   float64
		valid    bool
		expected Amount
	}{
		// Positive tests.
		{
			name:     "zero",
			amount:   0,
			valid:    true,
			expected: 0,
		},
		{
			name:     "max producible",
			amount:   21e9,
			valid:    true,
			expected: MaxGrain,
		},
		{
			name:     "min producible",
			amount:   -21e9,
			valid:    true,
			expected: -MaxGrain,
		},
		{
			name:     "exceeds max producible",
			amount:   21e9 + 1,
			valid:    true,
			expected: MaxGrain + 1e8,
		},
		{
			name:     "exceeds min producible",
			amount:   -21e9 - 1,
			valid:    true,
			expected: -MaxGrain - 1e8,
		},
		{
			name:     "one hundred",
			amount:   100,
			valid:    true,
			expected: 100 * GrainPerMDL,
		},
		{
			name:     "fraction",
			amount:   0.01234567,
			valid:    true,
			expected: 1234567,
		},
		{
			name:     "rounding up",
			amount:   54.999999999999943157,
			valid:    true,
			expected: 55 * GrainPerMDL,
		},
		{
			name:     "rounding down",
			amount:   55.000000000000056843,
			valid:    true,
			expected: 55 * GrainPerMDL,
		},

		// Negative tests.
		{
			name:   "not-a-number",
			amount: math.NaN(),
			valid:  false,
		},
		{
			name:   "-infinity",
			amount: math.Inf(-1),
			valid:  false,
		},
		{
			name:   "+infinity",
			amount: math.Inf(1),
			valid:  false,
		},
	}

	for _, test := range tests {
		a, err := NewAmount(test.amount)
		switch {
		case test.valid && err != nil:
			t.Errorf("%v: Positive test Amount creation failed with: %v", test.name, err)
			continue
		case !test.valid && err == nil:
			t.Errorf("%v: Negative test Amount creation succeeded (value %v) when should fail", test.name, a)
			continue
		}

		if a != test.expected {
			t.Errorf("%v: Created amount %v does not match expected %v", test.name, a, test.expected)
			continue
		}
	}
}

func TestAmountUnitConversions(t *testing.T) {
	tests := []struct {
		name      string
		amount    Amount
		unit      AmountUnit
		converted float64
		s         string
	}{
		{
			name:      "MPRL",
			amount:    MaxGrain,
			unit:      AmountMegaPRL,
			converted: 21000,
			s:         "21000 MPRL",
		},
		{
			name:      "kPRL",
			amount:    44433322211100,
			unit:      AmountKiloPRL,
			converted: 444.33322211100,
			s:         "444.333222111 kPRL",
		},
		{
			name:      "PRL",
			amount:    44433322211100,
			unit:      AmountPRL,
			converted: 444333.222111,
			s:         "444333.22211100 PRL",
		},
		{
			name:      "a thousand grain as PRL",
			amount:    1000,
			unit:      AmountPRL,
			converted: 0.00001,
			s:         "0.00001000 PRL",
		},
		{
			name:      "a single grain as PRL",
			amount:    1,
			unit:      AmountPRL,
			converted: 0.00000001,
			s:         "0.00000001 PRL",
		},
		{
			name:      "amount with trailing zero but no decimals",
			amount:    1000000000,
			unit:      AmountPRL,
			converted: 10,
			s:         "10 PRL",
		},
		{
			name:      "mPRL",
			amount:    44433322211100,
			unit:      AmountMilliPRL,
			converted: 444333222.11100,
			s:         "444333222.111 mPRL",
		},
		{

			name:      "μPRL",
			amount:    44433322211100,
			unit:      AmountMicroPRL,
			converted: 444333222111.00,
			s:         "444333222111 μPRL",
		},
		{

			name:      "grain",
			amount:    44433322211100,
			unit:      AmountGrain,
			converted: 44433322211100,
			s:         "44433322211100 Grain",
		},
		{

			name:      "non-standard unit",
			amount:    44433322211100,
			unit:      AmountUnit(-1),
			converted: 4443332.2211100,
			s:         "4443332.22111 1e-1 PRL",
		},
	}

	for _, test := range tests {
		f := test.amount.ToUnit(test.unit)
		if f != test.converted {
			t.Errorf("%v: converted value %v does not match expected %v", test.name, f, test.converted)
			continue
		}

		s := test.amount.Format(test.unit)
		if s != test.s {
			t.Errorf("%v: format '%v' does not match expected '%v'", test.name, s, test.s)
			continue
		}

		// Verify that Amount.ToPRL works as advertised.
		f1 := test.amount.ToUnit(AmountPRL)
		f2 := test.amount.ToPRL()
		if f1 != f2 {
			t.Errorf("%v: ToPRL does not match ToUnit(AmountPRL): %v != %v", test.name, f1, f2)
		}

		// Verify that Amount.String works as advertised.
		s1 := test.amount.Format(AmountPRL)
		s2 := test.amount.String()
		if s1 != s2 {
			t.Errorf("%v: String does not match Format(AmountPRL): %v != %v", test.name, s1, s2)
		}
	}
}

func TestAmountMulF64(t *testing.T) {
	tests := []struct {
		name string
		amt  Amount
		mul  float64
		res  Amount
	}{
		{
			name: "Multiply 0.1 PRL by 2",
			amt:  100e5, // 0.1 PRL
			mul:  2,
			res:  200e5, // 0.2 PRL
		},
		{
			name: "Multiply 0.2 PRL by 0.02",
			amt:  200e5, // 0.2 PRL
			mul:  1.02,
			res:  204e5, // 0.204 PRL
		},
		{
			name: "Multiply 0.1 PRL by -2",
			amt:  100e5, // 0.1 PRL
			mul:  -2,
			res:  -200e5, // -0.2 PRL
		},
		{
			name: "Multiply 0.2 PRL by -0.02",
			amt:  200e5, // 0.2 PRL
			mul:  -1.02,
			res:  -204e5, // -0.204 PRL
		},
		{
			name: "Multiply -0.1 PRL by 2",
			amt:  -100e5, // -0.1 PRL
			mul:  2,
			res:  -200e5, // -0.2 PRL
		},
		{
			name: "Multiply -0.2 PRL by 0.02",
			amt:  -200e5, // -0.2 PRL
			mul:  1.02,
			res:  -204e5, // -0.204 PRL
		},
		{
			name: "Multiply -0.1 PRL by -2",
			amt:  -100e5, // -0.1 PRL
			mul:  -2,
			res:  200e5, // 0.2 PRL
		},
		{
			name: "Multiply -0.2 PRL by -0.02",
			amt:  -200e5, // -0.2 PRL
			mul:  -1.02,
			res:  204e5, // 0.204 PRL
		},
		{
			name: "Round down",
			amt:  49, // 49 Grains
			mul:  0.01,
			res:  0,
		},
		{
			name: "Round up",
			amt:  50, // 50 Grains
			mul:  0.01,
			res:  1, // 1 Grain
		},
		{
			name: "Multiply by 0.",
			amt:  1e8, // 1 PRL
			mul:  0,
			res:  0, // 0 PRL
		},
		{
			name: "Multiply 1 by 0.5.",
			amt:  1, // 1 Grain
			mul:  0.5,
			res:  1, // 1 Grain
		},
		{
			name: "Multiply 100 by 66%.",
			amt:  100, // 100 Grains
			mul:  0.66,
			res:  66, // 66 Grains
		},
		{
			name: "Multiply 100 by 66.6%.",
			amt:  100, // 100 Grains
			mul:  0.666,
			res:  67, // 67 Grains
		},
		{
			name: "Multiply 100 by 2/3.",
			amt:  100, // 100 Grains
			mul:  2.0 / 3,
			res:  67, // 67 Grains
		},
	}

	for _, test := range tests {
		a := test.amt.MulF64(test.mul)
		if a != test.res {
			t.Errorf("%v: expected %v got %v", test.name, test.res, a)
		}
	}
}
