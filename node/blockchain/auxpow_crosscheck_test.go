// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/modelos/modelos/node/wire"
)

// auxPowFixture mirrors testdata/auxpow_verify_fixture.json, produced by
// gen_auxpow_fixture.py using the production pool's modelos_pool.auxpow_build.
type auxPowFixture struct {
	AuxPowHex           string `json:"auxpow_hex"`
	ModelOSHeight       int32  `json:"modelos_height"`
	ModelOSStateHashHex string `json:"modelos_state_hash_hex"`
	ModelOSBits         string `json:"modelos_bits"`
}

func loadAuxPowFixture(t *testing.T) (*wire.AuxPowData, auxPowFixture) {
	t.Helper()
	raw, err := os.ReadFile("testdata/auxpow_verify_fixture.json")
	if err != nil {
		t.Fatalf("read fixture (run gen_auxpow_fixture.py): %v", err)
	}
	var fx auxPowFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	auxBytes, err := hex.DecodeString(fx.AuxPowHex)
	if err != nil {
		t.Fatalf("decode auxpow_hex: %v", err)
	}
	var auxPow wire.AuxPowData
	if err := auxPow.Deserialise(bytes.NewReader(auxBytes)); err != nil {
		t.Fatalf("deserialise AuxPowData: %v", err)
	}
	return &auxPow, fx
}

func fixtureTarget(t *testing.T, fx auxPowFixture) (uint32, *chainhash.Hash) {
	t.Helper()
	bitsU64, err := strconv.ParseUint(fx.ModelOSBits, 16, 32)
	if err != nil {
		t.Fatalf("parse modelos_bits: %v", err)
	}
	shBytes, err := hex.DecodeString(fx.ModelOSStateHashHex)
	if err != nil {
		t.Fatalf("decode state hash: %v", err)
	}
	stateHash, err := chainhash.NewHash(shBytes)
	if err != nil {
		t.Fatalf("state hash: %v", err)
	}
	return uint32(bitsU64), stateHash
}

// TestVerifyAuxPow_PoolConstructedFixture is the belt-and-suspenders cross-language
// check: AuxPowData built by the production pool's auxpow_build (Python) must be
// accepted by the node's real VerifyAuxPow (Go). If the two implementations ever
// drift — coinbase commitment layout, 108-byte header, merkle binding, PoW byte
// order — this fails.
func TestVerifyAuxPow_PoolConstructedFixture(t *testing.T) {
	auxPow, fx := loadAuxPowFixture(t)
	bits, stateHash := fixtureTarget(t, fx)
	modelOSTarget := CompactToBig(bits)

	if err := VerifyAuxPow(auxPow, fx.ModelOSHeight, stateHash, modelOSTarget, wire.MainNet); err != nil {
		t.Fatalf("VerifyAuxPow rejected pool-constructed AuxPoW: %v", err)
	}
}

// TestVerifyAuxPow_RejectsWrongStateHash confirms the cross-check has teeth: a
// mismatched σ_modelos must be rejected (VerifyAuxPow step 3).
func TestVerifyAuxPow_RejectsWrongStateHash(t *testing.T) {
	auxPow, fx := loadAuxPowFixture(t)
	bits, _ := fixtureTarget(t, fx)
	modelOSTarget := CompactToBig(bits)

	var wrong chainhash.Hash // all-zero, != σ_modelos in the fixture
	if err := VerifyAuxPow(auxPow, fx.ModelOSHeight, &wrong, modelOSTarget, wire.MainNet); err == nil {
		t.Fatal("VerifyAuxPow accepted a mismatched ModelOSStateHash — cross-check has no teeth")
	}
}

// TestVerifyAuxPow_RejectsBelowActivation confirms the activation-height gate.
func TestVerifyAuxPow_RejectsBelowActivation(t *testing.T) {
	auxPow, fx := loadAuxPowFixture(t)
	bits, stateHash := fixtureTarget(t, fx)
	modelOSTarget := CompactToBig(bits)

	if err := VerifyAuxPow(auxPow, auxPowActivationHeight-1, stateHash, modelOSTarget, wire.MainNet); err == nil {
		t.Fatal("VerifyAuxPow accepted a block below the AuxPoW activation height")
	}
}
