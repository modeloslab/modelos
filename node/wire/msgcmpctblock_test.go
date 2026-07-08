// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
)

// testHeader returns a full MsgHeader (ZK certificate + block header) — the
// compact block carries the whole thing, so tests must too.
func testHeader(version int32) MsgHeader {
	prev := chainhash.Hash{0x01, 0x02, 0x03}
	merkle := chainhash.Hash{0x0a, 0x0b, 0x0c}
	bh := NewBlockHeader(version, &prev, &merkle, 0x1d00ffff)
	return MsgHeader{
		BlockHeader: *bh,
		MsgCertificate: MsgCertificate{
			Certificate: &ZKCertificate{
				Hash:      chainhash.Hash{0x21, 0x22, 0x23, 0x24},
				ProofData: []byte{0xca, 0xfe, 0xba, 0xbe, 0x01, 0x02},
			},
		},
	}
}

func testTx(lockTime uint32) *MsgTx {
	tx := NewMsgTx(1)
	tx.LockTime = lockTime
	tx.AddTxIn(&TxIn{
		PreviousOutPoint: OutPoint{Hash: chainhash.Hash{byte(lockTime)}, Index: 0},
		SignatureScript:  []byte{0x51},
		Sequence:         0xffffffff,
	})
	tx.AddTxOut(&TxOut{Value: 1000, PkScript: []byte{0x51, 0x20, 0x01}})
	return tx
}

func TestMsgSendCmpctRoundTrip(t *testing.T) {
	for _, ann := range []bool{true, false} {
		orig := NewMsgSendCmpct(ann, CompactBlocksVersion)
		var buf bytes.Buffer
		if err := orig.PrlEncode(&buf, 0, BaseEncoding); err != nil {
			t.Fatalf("encode: %v", err)
		}
		if buf.Len() != int(orig.MaxPayloadLength(0)) {
			t.Fatalf("sendcmpct size %d != MaxPayloadLength %d", buf.Len(), orig.MaxPayloadLength(0))
		}
		var got MsgSendCmpct
		if err := got.PrlDecode(bytes.NewReader(buf.Bytes()), 0, BaseEncoding); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got != *orig {
			t.Fatalf("sendcmpct mismatch: got %+v want %+v", got, *orig)
		}
	}
}

// TestMsgCmpctBlockRoundTripNonAuxPow exercises the non-AuxPoW path: a plain
// header, short IDs, and prefilled txns.
func TestMsgCmpctBlockRoundTripNonAuxPow(t *testing.T) {
	msg := NewMsgCmpctBlock()
	msg.Header = testHeader(1) // version 1 → NOT an auxpow block
	if IsAuxPowBlock(&msg.Header.BlockHeader) {
		t.Fatal("test header should not be an auxpow block")
	}
	msg.Nonce = 0xdeadbeef
	msg.ShortIDs = []ShortID{{1, 2, 3, 4, 5, 6}, {9, 9, 9, 9, 9, 9}}
	msg.Prefilled = []PrefilledTx{{Index: 0, Tx: testTx(7)}}

	assertCmpctRoundTrip(t, msg)
}

// TestMsgCmpctBlockRoundTripAuxPow is the important one: a compact block for a
// merge-mined (AuxPoW) modelOS block. The AuxPowData must survive the round trip
// intact so the receiver can reconstruct a block with a valid parent PoW proof.
func TestMsgCmpctBlockRoundTripAuxPow(t *testing.T) {
	msg := NewMsgCmpctBlock()
	msg.Header = testHeader(0x1101) // version bits → IsAuxPowBlock
	if !IsAuxPowBlock(&msg.Header.BlockHeader) {
		t.Fatal("test header should be an auxpow block (version 0x1101)")
	}
	msg.AuxPow = newTestAuxPowData()
	msg.Nonce = 0x0123456789abcdef
	msg.ShortIDs = []ShortID{{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}
	msg.Prefilled = []PrefilledTx{
		{Index: 0, Tx: testTx(1)}, // coinbase always prefilled
		{Index: 3, Tx: testTx(2)},
	}

	got := assertCmpctRoundTrip(t, msg)

	// Emphasis: the decoded AuxPowData must equal the original exactly — this is
	// what guarantees the reconstructed block's parent proof-of-work is intact.
	if got.AuxPow == nil {
		t.Fatal("decoded compact block dropped the AuxPowData")
	}
	if !reflect.DeepEqual(*msg.AuxPow, *got.AuxPow) {
		t.Fatalf("AuxPowData mismatch after compact-block round trip:\n orig=%+v\n got =%+v",
			*msg.AuxPow, *got.AuxPow)
	}
	if got.AuxPow.PearlHeight != msg.AuxPow.PearlHeight ||
		!bytes.Equal(got.AuxPow.PearlProofData, msg.AuxPow.PearlProofData) {
		t.Fatal("AuxPoW parent proof-of-work not preserved through the compact block")
	}
	// The block hash the receiver computes must match the sender's (header intact).
	if got.BlockHash() != msg.BlockHash() {
		t.Fatal("reconstructed block hash differs — header not preserved")
	}
}

// TestMsgCmpctBlockAuxPowMissingErrors ensures encoding an auxpow-versioned
// header without AuxPowData is a clean error, not a silent malformed block.
func TestMsgCmpctBlockAuxPowMissingErrors(t *testing.T) {
	msg := NewMsgCmpctBlock()
	msg.Header = testHeader(0x1101)
	msg.AuxPow = nil // omitted — should error
	var buf bytes.Buffer
	if err := msg.PrlEncode(&buf, 0, BaseEncoding); err == nil {
		t.Fatal("expected error encoding auxpow header with nil AuxPowData")
	}
}

func assertCmpctRoundTrip(t *testing.T, msg *MsgCmpctBlock) *MsgCmpctBlock {
	t.Helper()
	var buf bytes.Buffer
	if err := msg.PrlEncode(&buf, 0, BaseEncoding); err != nil {
		t.Fatalf("encode: %v", err)
	}
	got := NewMsgCmpctBlock()
	if err := got.PrlDecode(bytes.NewReader(buf.Bytes()), 0, BaseEncoding); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Nonce != msg.Nonce {
		t.Errorf("nonce: got %x want %x", got.Nonce, msg.Nonce)
	}
	if len(got.ShortIDs) != len(msg.ShortIDs) {
		t.Fatalf("shortID count: got %d want %d", len(got.ShortIDs), len(msg.ShortIDs))
	}
	for i := range msg.ShortIDs {
		if got.ShortIDs[i] != msg.ShortIDs[i] {
			t.Errorf("shortID[%d] mismatch", i)
		}
	}
	if len(got.Prefilled) != len(msg.Prefilled) {
		t.Fatalf("prefilled count: got %d want %d", len(got.Prefilled), len(msg.Prefilled))
	}
	for i := range msg.Prefilled {
		if got.Prefilled[i].Index != msg.Prefilled[i].Index {
			t.Errorf("prefilled[%d].Index: got %d want %d", i, got.Prefilled[i].Index, msg.Prefilled[i].Index)
		}
		if got.Prefilled[i].Tx.TxHash() != msg.Prefilled[i].Tx.TxHash() {
			t.Errorf("prefilled[%d] tx mismatch", i)
		}
	}
	return got
}

// TestShortIDDeterminism verifies short IDs are reproducible and nonce-sensitive.
func TestShortIDDeterminism(t *testing.T) {
	h := testHeader(1)
	tx := testTx(5)
	wtxid := tx.WitnessHash()

	key1 := ShortIDKey(&h.BlockHeader, 1)
	key1b := ShortIDKey(&h.BlockHeader, 1)
	if key1 != key1b {
		t.Fatal("ShortIDKey not deterministic for same header+nonce")
	}
	id1 := ComputeShortID(&wtxid, &key1)
	id1b := ComputeShortID(&wtxid, &key1b)
	if id1 != id1b {
		t.Fatal("ComputeShortID not deterministic")
	}

	// Different nonce → (almost certainly) different key and short ID.
	key2 := ShortIDKey(&h.BlockHeader, 2)
	if key1 == key2 {
		t.Fatal("ShortIDKey should differ for different nonces")
	}
	id2 := ComputeShortID(&wtxid, &key2)
	if id1 == id2 {
		t.Fatal("short ID should differ under a different nonce (grinding resistance)")
	}
}

func TestMsgGetBlockTxnRoundTrip(t *testing.T) {
	orig := NewMsgGetBlockTxn(chainhash.Hash{0x0f, 0x0e, 0x0d}, []uint32{0, 2, 5, 100})
	var buf bytes.Buffer
	if err := orig.PrlEncode(&buf, 0, BaseEncoding); err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got MsgGetBlockTxn
	if err := got.PrlDecode(bytes.NewReader(buf.Bytes()), 0, BaseEncoding); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.BlockHash != orig.BlockHash || !reflect.DeepEqual(got.Indexes, orig.Indexes) {
		t.Fatalf("getblocktxn mismatch: got %+v want %+v", got, *orig)
	}
}

func TestMsgBlockTxnRoundTrip(t *testing.T) {
	orig := NewMsgBlockTxn(chainhash.Hash{0xab}, []*MsgTx{testTx(1), testTx(2), testTx(3)})
	var buf bytes.Buffer
	if err := orig.PrlEncode(&buf, 0, BaseEncoding); err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got MsgBlockTxn
	if err := got.PrlDecode(bytes.NewReader(buf.Bytes()), 0, BaseEncoding); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.BlockHash != orig.BlockHash {
		t.Fatal("blocktxn block hash mismatch")
	}
	if len(got.Transactions) != len(orig.Transactions) {
		t.Fatalf("tx count: got %d want %d", len(got.Transactions), len(orig.Transactions))
	}
	for i := range orig.Transactions {
		if got.Transactions[i].TxHash() != orig.Transactions[i].TxHash() {
			t.Errorf("tx[%d] mismatch", i)
		}
	}
}

// TestCmpctBlockRegistered ensures the four commands decode via the message
// registry (makeEmptyMessage) — i.e. they are wired into the wire protocol.
func TestCmpctBlockRegistered(t *testing.T) {
	for _, cmd := range []string{CmdSendCmpct, CmdCmpctBlock, CmdGetBlockTxn, CmdBlockTxn} {
		if _, err := makeEmptyMessage(cmd); err != nil {
			t.Errorf("command %q not registered: %v", cmd, err)
		}
	}
}
