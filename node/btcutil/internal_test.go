// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
This test file is part of the btcutil package rather than than the
btcutil_test package so it can bridge access to the internals to properly test
cases which are either not possible or can't reliably be tested via the public
interface. The functions are only exported while the tests are being run.
*/

package btcutil

import (
	"github.com/modelos/modelos/node/btcutil/bech32"
)

// SetBlockBytes sets the internal serialized block byte buffer to the passed
// buffer.  It is used to inject errors and is only available to the test
// package.
func (b *Block) SetBlockBytes(buf []byte) {
	b.serializedBlock = buf
}

// TstAppDataDir makes the internal appDataDir function available to the test
// package.
func TstAppDataDir(goos, appName string, roaming bool) string {
	return appDataDir(goos, appName, roaming)
}

// TstAddressTaproot creates an AddressTaproot, initiating the fields as given.
// Only Taproot addresses are supported (version must be 1).
func TstAddressTaproot(version byte, program [32]byte,
	hrp string) *AddressTaproot {

	if version != 1 {
		panic("only Taproot (witness version 1) addresses are supported")
	}

	addr, err := newAddressTaproot(hrp, program[:])
	if err != nil {
		panic(err)
	}
	return addr
}

// TstAddressTaprootSAddr returns the expected witness program bytes for a
// bech32m encoded P2TR Pearl address. Only Taproot addresses are supported.
func TstAddressTaprootSAddr(addr string) []byte {
	_, data, err := bech32.Decode(addr)
	if err != nil {
		return []byte{}
	}

	// First byte is version, rest is base 32 encoded data.
	data, err = bech32.ConvertBits(data[1:], 5, 8, false)
	if err != nil {
		return []byte{}
	}
	return data
}
