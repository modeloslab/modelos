// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
)

// MsgGetBlockTxn implements the Message interface and represents a getblocktxn
// message (BIP-152). After receiving a cmpctblock, a peer that could not
// reconstruct the full block from its mempool requests the missing transactions
// by their ABSOLUTE index within the block.
//
// NOTE: modelOS uses absolute indexes (not BIP-152 differential CompactSize) —
// see the note in MsgCmpctBlock. Compact blocks are only exchanged between
// modelOS nodes running this code, so a simpler self-consistent encoding is fine.
type MsgGetBlockTxn struct {
	BlockHash chainhash.Hash
	Indexes   []uint32
}

// PrlDecode decodes r using the wire protocol encoding into the receiver.
func (msg *MsgGetBlockTxn) PrlDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	if _, err := io.ReadFull(r, msg.BlockHash[:]); err != nil {
		return err
	}
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if count > uint64(maxCmpctShortIDs) {
		return messageError("MsgGetBlockTxn.PrlDecode",
			fmt.Sprintf("too many indexes [count %d, max %d]", count, maxCmpctShortIDs))
	}
	msg.Indexes = make([]uint32, count)
	for i := uint64(0); i < count; i++ {
		idx, err := ReadVarInt(r, pver)
		if err != nil {
			return err
		}
		if idx > uint64(maxCmpctShortIDs) {
			return messageError("MsgGetBlockTxn.PrlDecode",
				fmt.Sprintf("index out of range [%d]", idx))
		}
		msg.Indexes[i] = uint32(idx)
	}
	return nil
}

// PrlEncode encodes the receiver to w using the wire protocol encoding.
func (msg *MsgGetBlockTxn) PrlEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	if _, err := w.Write(msg.BlockHash[:]); err != nil {
		return err
	}
	if err := WriteVarInt(w, pver, uint64(len(msg.Indexes))); err != nil {
		return err
	}
	for _, idx := range msg.Indexes {
		if err := WriteVarInt(w, pver, uint64(idx)); err != nil {
			return err
		}
	}
	return nil
}

// Command returns the protocol command string for the message.
func (msg *MsgGetBlockTxn) Command() string {
	return CmdGetBlockTxn
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.
func (msg *MsgGetBlockTxn) MaxPayloadLength(pver uint32) uint32 {
	return MaxMessagePayload
}

// NewMsgGetBlockTxn returns a new getblocktxn message.
func NewMsgGetBlockTxn(blockHash chainhash.Hash, indexes []uint32) *MsgGetBlockTxn {
	return &MsgGetBlockTxn{BlockHash: blockHash, Indexes: indexes}
}
