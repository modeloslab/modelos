// Copyright (c) 2013, 2014 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// AmountUnit describes a method of converting an Amount to something
// other than the base unit of a pearl.  The value of the AmountUnit
// is the exponent component of the decadic multiple to convert from
// an amount in pearls to an amount counted in units.
type AmountUnit int

// These constants define various units used when describing a pearl
// monetary amount.
const (
	AmountMegaPRL  AmountUnit = 6
	AmountKiloPRL  AmountUnit = 3
	AmountPRL      AmountUnit = 0
	AmountMilliPRL AmountUnit = -3
	AmountMicroPRL AmountUnit = -6
	AmountGrain    AmountUnit = -8
)

// String returns the unit as a string.  For recognized units, the SI
// prefix is used, or "Grain" for the base unit.  For all unrecognized
// units, "1eN PRL" is returned, where N is the AmountUnit.
func (u AmountUnit) String() string {
	switch u {
	case AmountMegaPRL:
		return "MPRL"
	case AmountKiloPRL:
		return "kPRL"
	case AmountPRL:
		return "PRL"
	case AmountMilliPRL:
		return "mPRL"
	case AmountMicroPRL:
		return "μPRL"
	case AmountGrain:
		return "Grain"
	default:
		return "1e" + strconv.FormatInt(int64(u), 10) + " PRL"
	}
}

// Amount represents the base monetary unit (colloquially referred
// to as a `Grain').  A single Amount is equal to 1e-8 of a pearl.
type Amount int64

// round converts a floating point number, which may or may not be representable
// as an integer, to the Amount integer type by rounding to the nearest integer.
// This is performed by adding or subtracting 0.5 depending on the sign, and
// relying on integer truncation to round the value to the nearest Amount.
func round(f float64) Amount {
	if f < 0 {
		return Amount(f - 0.5)
	}
	return Amount(f + 0.5)
}

// NewAmount creates an Amount from a floating point value representing
// some value in pearls.  NewAmount errors if f is NaN or +-Infinity, but
// does not check that the amount is within the total amount of pearls
// producible as f may not refer to an amount at a single moment in time.
//
// NewAmount is specifically for converting PRL to Grain.
// For creating a new Amount with an int64 value which denotes a quantity of Grain,
// do a simple type conversion from type int64 to Amount.
// See GoDoc for example: https://pkg.go.dev/github.com/modelos/modelos/node/btcutil#example-Amount
func NewAmount(f float64) (Amount, error) {
	// The amount is only considered invalid if it cannot be represented
	// as an integer type.  This may happen if f is NaN or +-Infinity.
	switch {
	case math.IsNaN(f):
		fallthrough
	case math.IsInf(f, 1):
		fallthrough
	case math.IsInf(f, -1):
		return 0, errors.New("invalid pearl amount")
	}

	return round(f * GrainPerMDL), nil
}

// ToUnit converts a monetary amount counted in base units to a
// floating point value representing an amount of pearls.
func (a Amount) ToUnit(u AmountUnit) float64 {
	return float64(a) / math.Pow10(int(u+8))
}

// ToPRL is the equivalent of calling ToUnit with AmountPRL.
func (a Amount) ToPRL() float64 {
	return a.ToUnit(AmountPRL)
}

// Format formats a monetary amount counted in base units as a
// string for a given unit.  The conversion will succeed for any unit,
// however, known units will be formatted with an appended label describing
// the units with SI notation, or "Grain" for the base unit.
func (a Amount) Format(u AmountUnit) string {
	units := " " + u.String()
	formatted := strconv.FormatFloat(a.ToUnit(u), 'f', -int(u+8), 64)

	// When formatting full PRL, add trailing zeroes for numbers
	// with decimal point to ease reading of grain amount.
	if u == AmountPRL {
		if strings.Contains(formatted, ".") {
			return fmt.Sprintf("%.8f%s", a.ToUnit(u), units)
		}
	}
	return formatted + units
}

// String is the equivalent of calling Format with AmountPRL.
func (a Amount) String() string {
	return a.Format(AmountPRL)
}

// MulF64 multiplies an Amount by a floating point value.  While this is not
// an operation that must typically be done by a full node or wallet, it is
// useful for services that build on top of Pearl (for example, calculating
// a fee by multiplying by a percentage).
func (a Amount) MulF64(f float64) Amount {
	return round(float64(a) * f)
}
