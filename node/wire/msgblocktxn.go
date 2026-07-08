// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
)

// MsgBlockTxn implements the Message interface and represents a blocktxn
// message (BIP-152): the response to a getblocktxn, carrying the full
// transactions the requester was missing to complete a compact block.
type MsgBlockTxn struct {
	BlockHash    chainhash.Hash
	Transactions []*MsgTx
}

// PrlDecode decodes r using the wire protocol encoding into the receiver.
func (msg *MsgBlockTxn) PrlDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	if _, err := io.ReadFull(r, msg.BlockHash[:]); err != nil {
		return err
	}
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if count > uint64(maxCmpctShortIDs) {
		return messageError("MsgBlockTxn.PrlDecode",
			fmt.Sprintf("too many transactions [count %d, max %d]", count, maxCmpctShortIDs))
	}
	msg.Transactions = make([]*MsgTx, count)
	for i := uint64(0); i < count; i++ {
		tx := &MsgTx{}
		if err := tx.PrlDecode(r, pver, enc); err != nil {
			return err
		}
		msg.Transactions[i] = tx
	}
	return nil
}

// PrlEncode encodes the receiver to w using the wire protocol encoding.
func (msg *MsgBlockTxn) PrlEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	if _, err := w.Write(msg.BlockHash[:]); err != nil {
		return err
	}
	if err := WriteVarInt(w, pver, uint64(len(msg.Transactions))); err != nil {
		return err
	}
	for _, tx := range msg.Transactions {
		if err := tx.PrlEncode(w, pver, enc); err != nil {
			return err
		}
	}
	return nil
}

// Command returns the protocol command string for the message.
func (msg *MsgBlockTxn) Command() string {
	return CmdBlockTxn
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.
func (msg *MsgBlockTxn) MaxPayloadLength(pver uint32) uint32 {
	return MaxMessagePayload
}

// NewMsgBlockTxn returns a new blocktxn message.
func NewMsgBlockTxn(blockHash chainhash.Hash, txns []*MsgTx) *MsgBlockTxn {
	return &MsgBlockTxn{BlockHash: blockHash, Transactions: txns}
}
