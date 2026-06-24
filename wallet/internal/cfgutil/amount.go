// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package cfgutil

import (
	"strconv"
	"strings"

	"github.com/modelos/modelos/node/btcutil"
)

// AmountFlag embeds a btcutil.Amount and implements the flags.Marshaler and
// Unmarshaler interfaces so it can be used as a config struct field.
type AmountFlag struct {
	btcutil.Amount
}

// NewAmountFlag creates an AmountFlag with a default btcutil.Amount.
func NewAmountFlag(defaultValue btcutil.Amount) *AmountFlag {
	return &AmountFlag{defaultValue}
}

// MarshalFlag satisfies the flags.Marshaler interface.
func (a *AmountFlag) MarshalFlag() (string, error) {
	return a.Amount.String(), nil
}

// UnmarshalFlag satisfies the flags.Unmarshaler interface.
func (a *AmountFlag) UnmarshalFlag(value string) error {
	value = strings.TrimSuffix(value, " PRL")
	valueF64, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return err
	}
	amount, err := btcutil.NewAmount(valueF64)
	if err != nil {
		return err
	}
	a.Amount = amount
	return nil
}
