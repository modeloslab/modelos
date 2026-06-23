// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package txscript implements transaction scripts for the Pearl
// blockchain. This file (pkscript.go) provides functionality for parsing
// and reconstructing transaction output scripts.
//
// CHANGES FROM UPSTREAM BTCD:
//
// 1. WITNESS-ONLY ENFORCEMENT:
//   - Supports witness version 1 (Taproot) and version 2 (P2MR/BIP 360)
//   - All legacy script types (P2PKH, P2SH, P2WPKH, P2WSH) are rejected
//   - Both formats are 34 bytes: OP_<version> OP_DATA_32 <32-byte-key/root>
//
// 2. COMPUTEPKSCRIPT BEHAVIOR:
//   - Follows btcsuite/btcd#1767 recommendation to return ErrUnsupportedScriptType
//     for witnesses that cannot be reliably reconstructed
//   - This is critical for block filter validation in neutrino-style clients
//   - Callers (rpcwebsocket.go, spv/verification.go) handle this gracefully
//
// 3. FILTER MATCHING IMPORTANCE:
//   - While all Taproot scripts have the same format (OP_1 <32-byte-key>),
//     the specific 32-byte key is unique per address and crucial for:
//   - BIP 158 compact block filters
//   - Address-based transaction filtering
//   - Light client synchronization
//
// 4. REMOVED FUNCTIONALITY:
//   - Legacy script reconstruction logic
//   - Test-only functions (ParsePkScript, Class(), String(), etc.)
//   - Complex witness analysis that doesn't work reliably with Taproot
//
// The remaining code provides essential compatibility for existing callers
// while properly handling the limitations of Taproot script reconstruction.
// TODO Or: remove this file once we have a proper solution for handling Taproot scripts.
// This is mostly used by neutrino, so it should be updated accordingly to deprecate it.
package txscript

import (
	"errors"

	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/wire"
)

// Script length constants for witness program outputs.
const (
	// witnessV1TaprootLen is the length of a P2TR script:
	// OP_1 (1 byte) + OP_DATA_32 (1 byte) + 32-byte key = 34 bytes
	witnessV1TaprootLen = 34

	// witnessV2MerkleRootLen is the length of a P2MR script:
	// OP_2 (1 byte) + OP_DATA_32 (1 byte) + 32-byte merkle root = 34 bytes
	witnessV2MerkleRootLen = 34
)

var (
	// ErrUnsupportedScriptType is returned when attempting to parse an unsupported
	// script type. In Taproot-only mode, only witness v1 scripts are supported.
	ErrUnsupportedScriptType = errors.New("unsupported script type")
)

// ComputePkScript computes the script of an output by looking at the spending
// input's signature script or witness.
//
// For Taproot script-spend cases, this attempts to reconstruct the output script
// by extracting the output key from the control block. For key-spend cases,
// reconstruction is not possible since the witness only contains a signature.
func ComputePkScript(sigScript []byte, witness wire.TxWitness) (PkScript, error) {
	switch {
	case len(sigScript) > 0:
		return computeNonWitnessPkScript(sigScript)
	case len(witness) > 0:
		return computeWitnessPkScript(witness)
	default:
		return PkScript{}, ErrUnsupportedScriptType
	}
}

// computeNonWitnessPkScript computes the script of an output by looking at the
// spending input's signature script. Non-witness scripts are not supported.
func computeNonWitnessPkScript(sigScript []byte) (PkScript, error) {
	// Non-witness scripts (P2PKH, P2SH) are not supported in Taproot-only mode
	return PkScript{}, ErrUnsupportedScriptType
}

// computeWitnessPkScript computes the script of an output by looking at the
// spending input's witness. Following btcsuite/btcd#1767 recommendation,
// this returns ErrUnsupportedScriptType for Taproot witnesses since they
// cannot be reliably reconstructed from witness data alone.
func computeWitnessPkScript(witness wire.TxWitness) (PkScript, error) {
	// As per btcsuite/btcd#1767, ComputePkScript should return
	// ErrUnsupportedScriptType for any witness that cannot be reliably
	// identified. This is especially important for Taproot (witness_v1_taproot)
	// where the output script format is uniform but the specific 32-byte key
	// cannot be reconstructed from witness data alone.
	//
	// This ensures proper behavior in block filter validation (neutrino-style)
	// where callers expect ErrUnsupportedScriptType and handle it gracefully
	// by skipping transactions they can't process.
	return PkScript{}, ErrUnsupportedScriptType
}

// PkScript is a wrapper that exists for compatibility with existing
// code that expects this type. It stores the actual witness output script
// with its unique 32-byte key/root for filter matching.
//
// Supported formats:
//   - Taproot:  OP_1 OP_DATA_32 <32-byte-key>
//   - P2MR:    OP_2 OP_DATA_32 <32-byte-merkle-root>
type PkScript struct {
	class ScriptClass

	// script contains the actual script bytes (34 bytes for both P2TR and P2MR).
	script [witnessV1TaprootLen]byte
}

// Script returns the script as a byte slice.
func (s PkScript) Script() []byte {
	switch s.class {
	case WitnessV1TaprootTy, WitnessV2MerkleRootTy:
		script := make([]byte, witnessV1TaprootLen)
		copy(script, s.script[:])
		return script
	default:
		return nil
	}
}

// Address encodes the script into an address for the given chain.
func (s PkScript) Address(chainParams *chaincfg.Params) (btcutil.Address, error) {
	switch s.class {
	case WitnessV1TaprootTy, WitnessV2MerkleRootTy:
		// ExtractPkScriptAddrs handles both P2TR and P2MR
	default:
		return nil, ErrUnsupportedScriptType
	}

	_, addrs, _, err := ExtractPkScriptAddrs(s.Script(), chainParams)
	if err != nil {
		return nil, err
	}

	if len(addrs) == 0 {
		return nil, errors.New("no addresses found in script")
	}

	return addrs[0], nil
}
