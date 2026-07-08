// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
)

// newTestAuxPowData builds a minimal but VALID AuxPowData for round-trip tests:
// a non-zero PearlBits in the header (required by Deserialise), a positive
// height, a non-empty proof blob, a coinbase tx, and a one-node Merkle branch.
// It intentionally mirrors the real structure without needing a live parent
// block, so tests stay simple. This is the shared fixture used by the AuxPoW
// round-trip tests AND the compact-block AuxPoW tests.
func newTestAuxPowData() *AuxPowData {
	a := &AuxPowData{
		ModelOSStateHash: chainhash.Hash{0xaa, 0xbb, 0xcc, 0xdd, 0x01},
		PearlHeight:      12345,
		PearlProofData:   []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03},
		PearlCoinbaseTx:  []byte{0x01, 0x00, 0x00, 0x00, 0xfa, 0xce},
		CoinbaseBranch: []AuxPowMerkleNode{
			{Hash: chainhash.Hash{0x11, 0x22, 0x33}, IsLeft: true},
			{Hash: chainhash.Hash{0x44, 0x55, 0x66}, IsLeft: false},
		},
	}
	// Set a non-zero, positive-sign compact bits field in the Pearl header
	// (Deserialise rejects a zero or sign-bit-set PearlBits).
	binary.LittleEndian.PutUint32(a.PearlHeader[pearlHdrBitsOffset:pearlHdrBitsOffset+4], 0x1d00ffff)
	// A distinctive Pearl merkle root so PearlMerkleRoot() is non-trivial.
	copy(a.PearlHeader[pearlHdrMerkleRootOffset:pearlHdrMerkleRootOffset+32],
		bytes.Repeat([]byte{0x7e}, 32))
	// Some non-zero bytes across the rest of the 108-byte header.
	for i := 0; i < 32; i++ {
		a.PearlHeader[i] = byte(i + 1)
	}
	copy(a.PearlPublicData[:], bytes.Repeat([]byte{0x5a}, len(a.PearlPublicData)))
	return a
}

// TestAuxPowDataRoundTrip is the core guarantee that compact blocks rely on:
// AuxPowData serialises and deserialises to an identical value, unchanged. The
// compact-block code reuses THESE EXACT methods, so if this holds, an AuxPoW
// block reconstructed from a compact block is byte-identical to the original.
func TestAuxPowDataRoundTrip(t *testing.T) {
	orig := newTestAuxPowData()

	var buf bytes.Buffer
	if err := orig.Serialise(&buf); err != nil {
		t.Fatalf("Serialise: %v", err)
	}

	// SerialiseSize must match the actual number of bytes written.
	if buf.Len() != orig.SerialiseSize() {
		t.Fatalf("SerialiseSize %d != bytes written %d", orig.SerialiseSize(), buf.Len())
	}

	var got AuxPowData
	if err := got.Deserialise(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Deserialise: %v", err)
	}

	if !reflect.DeepEqual(*orig, got) {
		t.Fatalf("AuxPowData round-trip mismatch:\n orig=%+v\n got =%+v", *orig, got)
	}

	// Re-serialise the decoded value; the bytes must be identical (determinism).
	var buf2 bytes.Buffer
	if err := got.Serialise(&buf2); err != nil {
		t.Fatalf("re-Serialise: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), buf2.Bytes()) {
		t.Fatalf("AuxPowData re-serialisation differs from original bytes")
	}
}

// TestAuxPowDataFieldsPreserved checks each field survives the round trip — an
// explicit, human-readable guard that no AuxPoW detail (parent proof, coinbase,
// merkle branch, height, state hash) is dropped or altered.
func TestAuxPowDataFieldsPreserved(t *testing.T) {
	orig := newTestAuxPowData()
	var buf bytes.Buffer
	if err := orig.Serialise(&buf); err != nil {
		t.Fatalf("Serialise: %v", err)
	}
	var got AuxPowData
	if err := got.Deserialise(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Deserialise: %v", err)
	}

	if got.PearlHeight != orig.PearlHeight {
		t.Errorf("PearlHeight: got %d want %d", got.PearlHeight, orig.PearlHeight)
	}
	if got.ModelOSStateHash != orig.ModelOSStateHash {
		t.Errorf("ModelOSStateHash mismatch")
	}
	if !bytes.Equal(got.PearlProofData, orig.PearlProofData) {
		t.Errorf("PearlProofData mismatch (parent proof-of-work must be preserved)")
	}
	if !bytes.Equal(got.PearlCoinbaseTx, orig.PearlCoinbaseTx) {
		t.Errorf("PearlCoinbaseTx mismatch")
	}
	if got.PearlHeader != orig.PearlHeader {
		t.Errorf("PearlHeader mismatch")
	}
	if got.PearlPublicData != orig.PearlPublicData {
		t.Errorf("PearlPublicData mismatch")
	}
	if len(got.CoinbaseBranch) != len(orig.CoinbaseBranch) {
		t.Fatalf("CoinbaseBranch length: got %d want %d", len(got.CoinbaseBranch), len(orig.CoinbaseBranch))
	}
	for i := range orig.CoinbaseBranch {
		if got.CoinbaseBranch[i] != orig.CoinbaseBranch[i] {
			t.Errorf("CoinbaseBranch[%d] mismatch", i)
		}
	}
	// Derived accessors must agree, proving the header bytes are intact.
	if got.PearlMerkleRoot() != orig.PearlMerkleRoot() {
		t.Errorf("PearlMerkleRoot derived mismatch")
	}
	if got.PearlHeaderBits() != orig.PearlHeaderBits() {
		t.Errorf("PearlHeaderBits derived mismatch")
	}
}
