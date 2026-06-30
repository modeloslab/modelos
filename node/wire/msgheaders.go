// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"
)

// MaxBlockHeadersPerMsg is the maximum number of block headers that can be in
// a single headers message.
// https://en.bitcoin.it/wiki/Protocol_documentation#getheaders
// 2000 is too many for our Block Header size, so we limit it to 100.
const MaxBlockHeadersPerMsg = 100

type MsgHeader struct {
	MsgCertificate MsgCertificate
	BlockHeader    BlockHeader
}

func (mh *MsgHeader) BlockCertificate() BlockCertificate {
	return mh.MsgCertificate.Certificate
}

func (mh *MsgHeader) PrlDecode(r io.Reader, pver uint32, buf []byte) error {
	if err := mh.MsgCertificate.PrlDecode(r, pver); err != nil {
		return err
	}

	return readBlockHeaderBuf(r, pver, &mh.BlockHeader, buf)
}

func (mh *MsgHeader) PrlEncode(w io.Writer, pver uint32, buf []byte) error {
	if err := mh.MsgCertificate.PrlEncode(w, pver); err != nil {
		return err
	}

	return writeBlockHeaderBuf(w, pver, &mh.BlockHeader, buf)
}

func (mh *MsgHeader) SerializeSize() int {
	return mh.BlockHeader.SerializeSize() + mh.MsgCertificate.SerializeSize()
}

// Serialize encodes the MsgHeader (certificate + header) to w.
func (mh *MsgHeader) Serialize(w io.Writer) error {
	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)
	return mh.PrlEncode(w, 0, buf)
}

// MsgHeaders implements the Message interface and represents a headers
// message.  It is used to deliver block header information in response
// to a getheaders message (MsgGetHeaders).  The maximum number of block headers
// per message is currently 2000.  See MsgGetHeaders for details on requesting
// the headers.
type MsgHeaders struct {
	Headers []MsgHeader
}

// AddBlockHeader adds a new block header to the message.
func (msg *MsgHeaders) AddBlockHeader(bh BlockHeader, cert BlockCertificate) error {
	if len(msg.Headers)+1 > MaxBlockHeadersPerMsg {
		str := fmt.Sprintf("too many block headers in message [max %v]",
			MaxBlockHeadersPerMsg)
		return messageError("MsgHeaders.AddBlockHeader", str)
	}

	msg.Headers = append(msg.Headers, MsgHeader{
		BlockHeader:    bh,
		MsgCertificate: MsgCertificate{Certificate: cert},
	})
	return nil
}

// PrlDecode decodes r using the wire protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgHeaders) PrlDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	count, err := ReadVarIntBuf(r, pver, buf)
	if err != nil {
		return err
	}

	// Limit to max block headers per message.
	if count > MaxBlockHeadersPerMsg {
		str := fmt.Sprintf("too many block headers for message "+
			"[count %v, max %v]", count, MaxBlockHeadersPerMsg)
		return messageError("MsgHeaders.PrlDecode", str)
	}

	// Create a contiguous slice of headers to deserialize into in order to
	// reduce the number of allocations.
	headers := make([]MsgHeader, count)
	msg.Headers = make([]MsgHeader, 0, count)
	for i := uint64(0); i < count; i++ {
		mh := &headers[i]
		err := mh.PrlDecode(r, pver, buf)
		if err != nil {
			return err
		}
		msg.Headers = append(msg.Headers, *mh)
	}

	if HasInconsistentCertificates(msg.Headers) {
		return messageError("MsgHeaders.PrlDecode",
			"header certificate presence does not match its AuxPoW status")
	}

	return nil
}

// PrlEncode encodes the receiver to w using the wire protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgHeaders) PrlEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	// Limit to max block headers per message.
	count := len(msg.Headers)
	if count > MaxBlockHeadersPerMsg {
		str := fmt.Sprintf("too many block headers for message "+
			"[count %v, max %v]", count, MaxBlockHeadersPerMsg)
		return messageError("MsgHeaders.PrlEncode", str)
	}

	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	err := WriteVarIntBuf(w, pver, uint64(count), buf)
	if err != nil {
		return err
	}

	for i := range msg.Headers {
		err := msg.Headers[i].PrlEncode(w, pver, buf)
		if err != nil {
			return err
		}
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgHeaders) Command() string {
	return CmdHeaders
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgHeaders) MaxPayloadLength(pver uint32) uint32 {
	// Num headers (varInt) + max allowed headers (header length + certificate).
	return MaxVarIntPayload + ((MaxBlockHeaderPayload + CertificateMaxSize) * MaxBlockHeadersPerMsg)
}

// NewMsgHeaders returns a new headers message that conforms to the
// Message interface.  See MsgHeaders for details.
func NewMsgHeaders() *MsgHeaders {
	return &MsgHeaders{
		Headers: make([]MsgHeader, 0, MaxBlockHeadersPerMsg),
	}
}

// HasInconsistentCertificates reports whether any header in the batch has a
// certificate presence that contradicts its AuxPoW status — the per-header
// invariant the consensus rules enforce (see blockchain.checkBlockSanity):
//
//   - An AuxPoW (merged-mined) block carries a NULL certificate; its proof-of-work
//     is the embedded Pearl proof, verified by VerifyAuxPow.
//   - A native block carries a non-null ZK certificate.
//
// Crucially, AuxPoW and native blocks are interleaved arbitrarily along the chain
// (each block independently is one or the other), so a HEADERS batch legitimately
// contains a MIX of certified and uncertified headers in any order. The previous
// "a batch must not mix certified and uncertified headers" rule was therefore wrong
// on an AuxPoW chain: it rejected every real batch that spanned both block kinds,
// which is exactly what wedged fresh syncers (e.g. stuck at the first AuxPoW block).
//
// We validate the correct, order-independent invariant instead: cert == nil IFF the
// header is an AuxPoW block. A peer that sends a non-AuxPoW header with no cert, or
// an AuxPoW header carrying a cert, is malformed and rejected. Full certificate /
// proof verification happens downstream in the blockchain layer.
func HasInconsistentCertificates(headers []MsgHeader) bool {
	for i := range headers {
		isAuxPow := IsAuxPowBlock(&headers[i].BlockHeader)
		hasCert := headers[i].BlockCertificate() != nil
		if isAuxPow == hasCert {
			// AuxPoW must have NO cert; non-AuxPoW must HAVE a cert.
			return true
		}
	}
	return false
}
