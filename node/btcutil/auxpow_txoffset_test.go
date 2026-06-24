// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/modelos/modelos/node/wire"
)

// TestAuxPowBlockTxOffset is a regression guard for the merged-mining bug that
// rejected every valid AuxPoW MDL block with a bogus "block merkle root is
// invalid".
//
// Root cause: Block.Transactions() computed each transaction's raw-byte offset
// inside the serialized block as MsgHeader.SerializeSize() + varint(txCount),
// which OMITTED the variable-length AuxPowData that sits between the header and
// the transaction list. The cached raw-byte slice for the coinbase was therefore
// taken from inside AuxPowData (the Pearl header / proof region). Tx.Hash() hashes
// that cached slice when present (skipping msgTx.TxHash() for speed), so the
// coinbase txid — and thus CalcMerkleRoot — was computed over the wrong bytes and
// never matched the header's MerkleRoot.
//
// The coinbase here is SEGWIT (carries a witness commitment), exactly like the
// pool-built MDL coinbase, because Tx.Hash()'s raw-byte path has a separate
// witness-stripping branch that must also operate on the correct bytes.
func TestAuxPowBlockTxOffset(t *testing.T) {
	// Segwit coinbase: null prevout, a scriptSig, two outputs (payout + witness
	// commitment), and a 32-byte witness reserved value.
	coinbase := wire.NewMsgTx(1)
	prevOut := wire.NewOutPoint(&chainhash.Hash{}, wire.MaxPrevOutIndex)
	txIn := wire.NewTxIn(prevOut, []byte{0x03, 0x07, 0x00, 0x00, 0x4d, 0x44, 0x4c, 0x2a}, nil)
	txIn.Witness = wire.TxWitness{make([]byte, 32)} // segwit commitment reserved value
	coinbase.AddTxIn(txIn)
	coinbase.AddTxOut(wire.NewTxOut(50_00000000, append([]byte{0x51, 0x20}, make([]byte, 32)...)))
	coinbase.AddTxOut(wire.NewTxOut(0, append([]byte{0x6a, 0x24, 0xaa, 0x21, 0xa9, 0xed}, make([]byte, 32)...)))

	if !coinbase.HasWitness() {
		t.Fatal("test coinbase must carry witness data to exercise the strip path")
	}
	wantTxid := coinbase.TxHash()

	// AuxPoW data — large enough that an omitted offset lands deep inside it.
	aux := &wire.AuxPowData{
		PearlHeight:     1,
		PearlProofData:  make([]byte, 4096),
		PearlCoinbaseTx: make([]byte, 200),
	}
	// PearlHeader.Bits must be non-zero with the sign bit clear, or Deserialise rejects it.
	binary.LittleEndian.PutUint32(aux.PearlHeader[72:76], 0x1d00ffff)

	blk := &wire.MsgBlock{
		MsgHeader: wire.MsgHeader{
			BlockHeader: wire.BlockHeader{Version: wire.AuxPowVersion, Bits: 0x1d00ffff},
		},
		AuxPow:       aux,
		Transactions: []*wire.MsgTx{coinbase},
	}
	if !wire.IsAuxPowBlock(&blk.MsgHeader.BlockHeader) {
		t.Fatal("test block must be recognized as AuxPoW")
	}

	var buf bytes.Buffer
	if err := blk.Serialize(&buf); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	b, err := btcutil.NewBlockFromBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("NewBlockFromBytes: %v", err)
	}

	got := b.Transactions()[0].Hash()
	if *got != wantTxid {
		t.Fatalf("coinbase txid taken from wrong offset (AuxPowData not skipped):\n"+
			" got  %v\n want %v\nThis is the 'block merkle root is invalid' AuxPoW bug.",
			got, wantTxid)
	}
}
