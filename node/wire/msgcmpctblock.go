// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/aead/siphash"
	"github.com/modelos/modelos/node/chaincfg/chainhash"
)

// ShortIDLength is the length in bytes of a compact-block short transaction ID
// (BIP-152): the low 6 bytes (48 bits) of a keyed SipHash over the wtxid.
const ShortIDLength = 6

// maxCmpctShortIDs bounds the short-ID count so a malicious cmpctblock cannot
// exhaust memory. A short ID is 6 bytes; the block-tx ceiling is maxTxPerBlock.
const maxCmpctShortIDs = maxTxPerBlock

// ShortID is the 6-byte compact-block short transaction identifier.
type ShortID [ShortIDLength]byte

// PrefilledTx is a transaction the sender includes in full inside a cmpctblock
// (rather than as a short ID) because the receiver is unlikely to have it — the
// coinbase always, plus any tx the sender deems likely-missing. Index is the
// ABSOLUTE position of the tx in the block.
//
// NOTE: modelOS uses absolute indexes (not BIP-152's differential CompactSize)
// because compact blocks are only ever exchanged between modelOS nodes running
// this same code (feature-negotiated); a simpler self-consistent encoding lowers
// bug surface with no interop cost.
type PrefilledTx struct {
	Index uint32
	Tx    *MsgTx
}

// MsgCmpctBlock implements the Message interface and represents a cmpctblock
// message (a modelOS extension of BIP-152). It carries just enough to let a peer
// reconstruct a full block from its mempool: the block header, the AuxPoW proof
// (modelOS is merge-mined — without this the reconstructed block has no PoW and
// cannot validate), a nonce that keys the short IDs, the short IDs of every
// non-prefilled tx, and the prefilled txs in full.
//
// Wire layout (mirrors MsgBlock's non-transaction prefix exactly, so a
// reconstructed block is byte-identical to the original):
//
//	header       : MsgHeader = ZK certificate (proof-of-useful-work) + 80-byte block header
//	auxpow       : AuxPowData  (only if IsAuxPowBlock(header) — mirrors MsgBlock)
//	nonce        : uint64 LE   (keys the SipHash short IDs)
//	shortIDCount : varint
//	shortIDs     : ShortIDCount × 6 bytes
//	prefilledCnt : varint
//	prefilled    : prefilledCnt × { index: varint, tx: transaction }
//
// NOTE: the FULL MsgHeader (including the ZK certificate) is carried, not just
// the 80-byte block header — the certificate is modelOS's proof-of-useful-work
// and part of the block; without it the reconstructed block cannot validate.
type MsgCmpctBlock struct {
	Header    MsgHeader
	AuxPow    *AuxPowData
	Nonce     uint64
	ShortIDs  []ShortID
	Prefilled []PrefilledTx
}

// BlockHash returns the hash of the block this compact block represents. The
// block hash is defined over the 80-byte block header only (the certificate and
// AuxPoW do not affect the hash), matching MsgBlock.
func (msg *MsgCmpctBlock) BlockHash() chainhash.Hash {
	return msg.Header.BlockHeader.BlockHash()
}

// ShortIDKey derives the two SipHash keys (as a single 16-byte key) used to
// compute short IDs for this compact block: SHA256(header80 || nonceLE)[0:16].
// Both sender and receiver derive it identically from the 80-byte header + nonce
// (the AuxPoW is not part of the key — it does not affect tx identity).
func ShortIDKey(header *BlockHeader, nonce uint64) [16]byte {
	var b bytes.Buffer
	b.Grow(MaxBlockHeaderPayload + 8)
	_ = writeBlockHeader(&b, 0, header)
	var n [8]byte
	littleEndian.PutUint64(n[:], nonce)
	b.Write(n[:])
	sum := sha256.Sum256(b.Bytes())
	var key [16]byte
	copy(key[:], sum[:16])
	return key
}

// ComputeShortID computes the 6-byte short ID of a wtxid under the given key.
func ComputeShortID(wtxid *chainhash.Hash, key *[16]byte) ShortID {
	v := siphash.Sum64(wtxid[:], key) // 64-bit; take the low 48 bits
	var id ShortID
	id[0] = byte(v)
	id[1] = byte(v >> 8)
	id[2] = byte(v >> 16)
	id[3] = byte(v >> 24)
	id[4] = byte(v >> 32)
	id[5] = byte(v >> 40)
	return id
}

// PrlDecode decodes r using the wire protocol encoding into the receiver.
func (msg *MsgCmpctBlock) PrlDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	if err := msg.Header.PrlDecode(r, pver, buf); err != nil {
		return err
	}
	if IsAuxPowBlock(&msg.Header.BlockHeader) {
		msg.AuxPow = &AuxPowData{}
		if err := msg.AuxPow.Deserialise(r); err != nil {
			return messageError("MsgCmpctBlock.PrlDecode", "auxpow: "+err.Error())
		}
	}
	nonce, err := binarySerializer.Uint64(r, littleEndian)
	if err != nil {
		return err
	}
	msg.Nonce = nonce

	sidCount, err := ReadVarIntBuf(r, pver, buf)
	if err != nil {
		return err
	}
	if sidCount > uint64(maxCmpctShortIDs) {
		return messageError("MsgCmpctBlock.PrlDecode",
			fmt.Sprintf("too many short IDs [count %d, max %d]", sidCount, maxCmpctShortIDs))
	}
	msg.ShortIDs = make([]ShortID, sidCount)
	for i := uint64(0); i < sidCount; i++ {
		if _, err := io.ReadFull(r, msg.ShortIDs[i][:]); err != nil {
			return err
		}
	}

	preCount, err := ReadVarIntBuf(r, pver, buf)
	if err != nil {
		return err
	}
	if preCount > uint64(maxCmpctShortIDs) {
		return messageError("MsgCmpctBlock.PrlDecode",
			fmt.Sprintf("too many prefilled txns [count %d, max %d]", preCount, maxCmpctShortIDs))
	}
	msg.Prefilled = make([]PrefilledTx, preCount)
	for i := uint64(0); i < preCount; i++ {
		idx, err := ReadVarIntBuf(r, pver, buf)
		if err != nil {
			return err
		}
		if idx > uint64(maxCmpctShortIDs) {
			return messageError("MsgCmpctBlock.PrlDecode",
				fmt.Sprintf("prefilled index out of range [%d]", idx))
		}
		tx := &MsgTx{}
		if err := tx.PrlDecode(r, pver, enc); err != nil {
			return err
		}
		msg.Prefilled[i] = PrefilledTx{Index: uint32(idx), Tx: tx}
	}
	return nil
}

// PrlEncode encodes the receiver to w using the wire protocol encoding.
func (msg *MsgCmpctBlock) PrlEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	if err := msg.Header.PrlEncode(w, pver, buf); err != nil {
		return err
	}
	if IsAuxPowBlock(&msg.Header.BlockHeader) {
		if msg.AuxPow == nil {
			return messageError("MsgCmpctBlock.PrlEncode", "auxpow block missing AuxPowData")
		}
		if err := msg.AuxPow.Serialise(w); err != nil {
			return err
		}
	}
	if err := binarySerializer.PutUint64(w, littleEndian, msg.Nonce); err != nil {
		return err
	}
	if err := WriteVarInt(w, pver, uint64(len(msg.ShortIDs))); err != nil {
		return err
	}
	for i := range msg.ShortIDs {
		if _, err := w.Write(msg.ShortIDs[i][:]); err != nil {
			return err
		}
	}
	if err := WriteVarInt(w, pver, uint64(len(msg.Prefilled))); err != nil {
		return err
	}
	for i := range msg.Prefilled {
		if err := WriteVarInt(w, pver, uint64(msg.Prefilled[i].Index)); err != nil {
			return err
		}
		if err := msg.Prefilled[i].Tx.PrlEncode(w, pver, enc); err != nil {
			return err
		}
	}
	return nil
}

// Command returns the protocol command string for the message.
func (msg *MsgCmpctBlock) Command() string {
	return CmdCmpctBlock
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver. A compact block is bounded above by a full block plus the short-ID
// and framing overhead, so the max-message payload is a safe, simple bound.
func (msg *MsgCmpctBlock) MaxPayloadLength(pver uint32) uint32 {
	return MaxMessagePayload
}

// NewMsgCmpctBlock returns a new, empty cmpctblock message.
func NewMsgCmpctBlock() *MsgCmpctBlock {
	return &MsgCmpctBlock{}
}
