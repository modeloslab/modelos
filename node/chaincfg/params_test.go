// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/modelos/modelos/node/wire"
	"github.com/stretchr/testify/require"
)

// TestInvalidHashStr ensures the newShaHashFromStr function panics when used to
// with an invalid hash string.
func TestInvalidHashStr(t *testing.T) {
	require.Panics(t, func() {
		newHashFromStr("banana")
	}, "Expected panic for invalid hash")
}

// TestMustRegisterPanic ensures the mustRegister function panics when used to
// register an invalid network.
func TestMustRegisterPanic(t *testing.T) {
	t.Parallel()

	// Intentionally try to register duplicate params to force a panic.
	require.Panics(t, func() {
		mustRegister(&MainNetParams)
	}, "mustRegister did not panic as expected")
}

func TestRegisterHDKeyID(t *testing.T) {
	t.Parallel()

	// Ref: https://github.com/satoshilabs/slips/blob/master/slip-0132.md
	hdKeyIDZprv := []byte{0x02, 0xaa, 0x7a, 0x99}
	hdKeyIDZpub := []byte{0x02, 0xaa, 0x7e, 0xd3}

	err := RegisterHDKeyID(hdKeyIDZpub, hdKeyIDZprv)
	require.NoError(t, err, "RegisterHDKeyID")

	got, err := HDPrivateKeyToPublicKeyID(hdKeyIDZprv)
	require.NoError(t, err, "HDPrivateKeyToPublicKeyID")
	require.Equal(t, hdKeyIDZpub, got, "HDPrivateKeyToPublicKeyID result mismatch")
}

func TestInvalidHDKeyID(t *testing.T) {
	t.Parallel()

	prvValid := []byte{0x02, 0xaa, 0x7a, 0x99}
	pubValid := []byte{0x02, 0xaa, 0x7e, 0xd3}
	prvInvalid := []byte{0x00}
	pubInvalid := []byte{0x00}

	err := RegisterHDKeyID(pubInvalid, prvValid)
	require.ErrorIs(t, err, ErrInvalidHDKeyID)

	err = RegisterHDKeyID(pubValid, prvInvalid)
	require.ErrorIs(t, err, ErrInvalidHDKeyID)

	err = RegisterHDKeyID(pubInvalid, prvInvalid)
	require.ErrorIs(t, err, ErrInvalidHDKeyID)

	// FIXME: The error type should be changed to ErrInvalidHDKeyID.
	_, err = HDPrivateKeyToPublicKeyID(prvInvalid)
	require.ErrorIs(t, err, ErrUnknownHDKeyID)
}

func TestSigNetPowLimit(t *testing.T) {
	// sigNetPowLimit should be 2^228 - 1 (7 leading hex zeros followed by 57 f's)
	expectedPowLimitHex, err := hex.DecodeString(
		"0000000fffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	)
	require.NoError(t, err)
	expectedPowLimit := new(big.Int).SetBytes(expectedPowLimitHex)
	require.Equal(t, 0, sigNetPowLimit.Cmp(expectedPowLimit),
		"Signet PoW limit (%s) not equal to expected 2^228-1 (%s)",
		sigNetPowLimit.Text(16), expectedPowLimit.Text(16))

	// The genesis block Bits (0x1d0fffff) is the compact representation.
	// Compact format has limited precision (24-bit mantissa), so it yields
	// 0x0fffff000... rather than 0x0ffff...fff. Verify the expected compact value.
	expectedBitsTargetHex, err := hex.DecodeString(
		"0000000fffff0000000000000000000000000000000000000000000000000000",
	)
	require.NoError(t, err)
	expectedBitsTarget := new(big.Int).SetBytes(expectedBitsTargetHex)
	actualBitsTarget := compactToBig(sigNetGenesisBlock.BlockHeader().Bits)
	require.Equal(t, 0, actualBitsTarget.Cmp(expectedBitsTarget),
		"Signet genesis Bits target (%s) not equal to expected (%s)",
		actualBitsTarget.Text(16), expectedBitsTarget.Text(16))
}

// TestSigNetMagic makes sure that the default signet has the expected Pearl
// network magic.
func TestSigNetMagic(t *testing.T) {
	require.Equal(t, wire.SigNet, SigNetParams.Net)
}

// compactToBig is a copy of the blockchain.CompactToBig function. We copy it
// here so we don't run into a circular dependency just because of a test.
func compactToBig(compact uint32) *big.Int {
	// Extract the mantissa, sign bit, and exponent.
	mantissa := compact & 0x007fffff
	isNegative := compact&0x00800000 != 0
	exponent := uint(compact >> 24)

	// Since the base for the exponent is 256, the exponent can be treated
	// as the number of bytes to represent the full 256-bit number.  So,
	// treat the exponent as the number of bytes and shift the mantissa
	// right or left accordingly.  This is equivalent to:
	// N = mantissa * 256^(exponent-3)
	var bn *big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		bn = big.NewInt(int64(mantissa))
	} else {
		bn = big.NewInt(int64(mantissa))
		bn.Lsh(bn, 8*(exponent-3))
	}

	// Make it negative if the sign bit is set.
	if isNegative {
		bn = bn.Neg(bn)
	}

	return bn
}
