// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
)

// MaxGetCFiltersReqRange the maximum number of filters that may be requested in
// a getcfheaders message.
const MaxGetCFiltersReqRange = 1000

// MsgGetCFilters implements the Message interface and represents a
// getcfilters message. It is used to request committed filters for a range of
// blocks.
type MsgGetCFilters struct {
	FilterType  FilterType
	StartHeight uint32
	StopHash    chainhash.Hash
}

// PrlDecode decodes r using the wire protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgGetCFilters) PrlDecode(r io.Reader, pver uint32, _ MessageEncoding) error {
	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return err
	}
	msg.FilterType = FilterType(buf[0])

	if _, err := io.ReadFull(r, buf[:4]); err != nil {
		return err
	}
	msg.StartHeight = littleEndian.Uint32(buf[:4])

	_, err := io.ReadFull(r, msg.StopHash[:])
	return err
}

// PrlEncode encodes the receiver to w using the wire protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetCFilters) PrlEncode(w io.Writer, pver uint32, _ MessageEncoding) error {
	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	buf[0] = byte(msg.FilterType)
	if _, err := w.Write(buf[:1]); err != nil {
		return err
	}

	littleEndian.PutUint32(buf[:4], msg.StartHeight)
	if _, err := w.Write(buf[:4]); err != nil {
		return err
	}

	_, err := w.Write(msg.StopHash[:])
	return err
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetCFilters) Command() string {
	return CmdGetCFilters
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetCFilters) MaxPayloadLength(pver uint32) uint32 {
	// Filter type + uint32 + block hash
	return 1 + 4 + chainhash.HashSize
}

// NewMsgGetCFilters returns a new getcfilters message that conforms to
// the Message interface using the passed parameters and defaults for the
// remaining fields.
func NewMsgGetCFilters(filterType FilterType, startHeight uint32,
	stopHash *chainhash.Hash) *MsgGetCFilters {
	return &MsgGetCFilters{
		FilterType:  filterType,
		StartHeight: startHeight,
		StopHash:    *stopHash,
	}
}
