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

// HasInconsistentCertificates reports whether any header in the batch carries a
// certificate combination the consensus rules would reject outright — used to drop
// obviously-malformed HEADERS without deferring to full block validation.
//
// It mirrors blockchain.checkBlockSanity EXACTLY, and no more strictly: the ONLY
// invalid combination there is a NON-AuxPoW header with NO certificate (validate.go
// returns ErrCertificateMissing). Everything else is permitted —
//   - native block + ZK certificate (the normal case),
//   - AuxPoW block + null certificate (its PoW is the embedded Pearl proof), and
//   - AuxPoW block + certificate (consensus allows it; the cert version check is
//     simply skipped for AuxPoW).
//
// AuxPoW and native blocks are interleaved arbitrarily along the chain, so a HEADERS
// batch legitimately MIXES certified and uncertified headers in any order. The old
// "a batch must not mix certified and uncertified headers" rule was wrong on an
// AuxPoW chain — it rejected every real batch spanning both kinds and wedged fresh
// syncers at the first AuxPoW block. We must also not over-correct: rejecting a
// header the chain would ACCEPT (e.g. AuxPoW-with-cert) would itself wedge sync, so
// we reject only the single combination consensus does.
func HasInconsistentCertificates(headers []MsgHeader) bool {
	for i := range headers {
		isAuxPow := IsAuxPowBlock(&headers[i].BlockHeader)
		hasCert := headers[i].BlockCertificate() != nil
		if !isAuxPow && !hasCert {
			// Non-AuxPoW header with no certificate — the one combination the
			// consensus layer rejects (ErrCertificateMissing).
			return true
		}
	}
	return false
}
