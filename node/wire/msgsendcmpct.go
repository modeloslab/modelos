// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"
)

// CompactBlocksVersion is the compact-block protocol version modelOS speaks.
// Version 2 keys short IDs off the transaction WITNESS hash (wtxid), which is
// correct for a Taproot/segwit chain. Only modelOS nodes negotiate this (see
// docs/BIP-152 extension); every message is feature-negotiated via sendcmpct so
// non-supporting (older) peers are never sent compact blocks and are unaffected.
const CompactBlocksVersion uint64 = 2

// MsgSendCmpct implements the Message interface and represents a sendcmpct
// message (BIP-152). A peer sends it to announce whether it wants to receive
// new blocks as compact blocks (Announce=true → high-bandwidth mode: the sender
// may push cmpctblock unsolicited) and which compact-block version it supports.
type MsgSendCmpct struct {
	// Announce selects high-bandwidth mode when true: the receiving peer may
	// send us new blocks as an unsolicited cmpctblock. When false (low
	// bandwidth) the peer should announce via headers/inv first, as before.
	Announce bool

	// Version is the compact-block protocol version (see CompactBlocksVersion).
	Version uint64
}

// PrlDecode decodes r using the wire protocol encoding into the receiver.
func (msg *MsgSendCmpct) PrlDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	var b [1]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return err
	}
	msg.Announce = b[0] != 0
	ver, err := binarySerializer.Uint64(r, littleEndian)
	if err != nil {
		return err
	}
	msg.Version = ver
	return nil
}

// PrlEncode encodes the receiver to w using the wire protocol encoding.
func (msg *MsgSendCmpct) PrlEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	var b [1]byte
	if msg.Announce {
		b[0] = 1
	}
	if _, err := w.Write(b[:]); err != nil {
		return err
	}
	return binarySerializer.PutUint64(w, littleEndian, msg.Version)
}

// Command returns the protocol command string for the message.
func (msg *MsgSendCmpct) Command() string {
	return CmdSendCmpct
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver: 1 byte (announce) + 8 bytes (version).
func (msg *MsgSendCmpct) MaxPayloadLength(pver uint32) uint32 {
	return 9
}

// NewMsgSendCmpct returns a new sendcmpct message that conforms to the Message
// interface.
func NewMsgSendCmpct(announce bool, version uint64) *MsgSendCmpct {
	return &MsgSendCmpct{Announce: announce, Version: version}
}
