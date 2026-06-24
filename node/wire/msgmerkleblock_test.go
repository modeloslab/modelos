// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
	"time"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/stretchr/testify/require"
)

// TestMerkleBlock tests the MsgMerkleBlock API.
func TestMerkleBlock(t *testing.T) {
	pver := ProtocolVersion
	enc := BaseEncoding

	// Block 1 header.
	prevHash := &blockOne.BlockHeader().PrevBlock
	merkleHash := &blockOne.BlockHeader().MerkleRoot
	bits := blockOne.BlockHeader().Bits
	bh := NewBlockHeader(1, prevHash, merkleHash, bits)

	msg := NewMsgMerkleBlock(bh)
	require.Equal(t, "merkleblock", msg.Command())
	// Max payload updated to 4M to match MaxBlockPayload.
	require.Equal(t, uint32(4000000), msg.MaxPayloadLength(pver))

	// Load maxTxPerBlock hashes.
	data := make([]byte, 32)
	for i := 0; i < maxTxPerBlock; i++ {
		rand.Read(data)
		hash, err := chainhash.NewHash(data)
		require.NoError(t, err)
		err = msg.AddTxHash(hash)
		require.NoError(t, err)
	}

	// Add one more Tx to test failure.
	rand.Read(data)
	hash, err := chainhash.NewHash(data)
	require.NoError(t, err)
	err = msg.AddTxHash(hash)
	require.Error(t, err)

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err = msg.PrlEncode(&buf, pver, enc)
	require.NoError(t, err)

	// Test decode with latest protocol version.
	readmsg := MsgMerkleBlock{}
	err = readmsg.PrlDecode(&buf, pver, enc)
	require.NoError(t, err)

	// Force extra hash to test maxTxPerBlock.
	msg.Hashes = append(msg.Hashes, hash)
	err = msg.PrlEncode(&buf, pver, enc)
	require.Error(t, err)

	// Force too many flag bytes to test maxFlagsPerMerkleBlock.
	// Reset the number of hashes back to a valid value.
	msg.Hashes = msg.Hashes[len(msg.Hashes)-1:]
	msg.Flags = make([]byte, maxFlagsPerMerkleBlock+1)
	err = msg.PrlEncode(&buf, pver, enc)
	require.Error(t, err)
}

// TestMerkleBlockWire tests the MsgMerkleBlock wire encode and decode.
func TestMerkleBlockWire(t *testing.T) {
	var buf bytes.Buffer
	err := merkleBlockOne.PrlEncode(&buf, ProtocolVersion, BaseEncoding)
	require.NoError(t, err)
	require.Equal(t, merkleBlockOneBytes, buf.Bytes())

	var msg MsgMerkleBlock
	rbuf := bytes.NewReader(merkleBlockOneBytes)
	err = msg.PrlDecode(rbuf, ProtocolVersion, BaseEncoding)
	require.NoError(t, err)
	require.Equal(t, &merkleBlockOne, &msg)
}

// TestMerkleBlockWireErrors performs negative tests against wire encode and
// decode of MsgBlock to confirm error paths work correctly.
func TestMerkleBlockWireErrors(t *testing.T) {
	pver := ProtocolVersion

	tests := []struct {
		max      int
		writeErr error
		readErr  error
	}{
		{0, io.ErrShortWrite, io.EOF},               // Force error in version.
		{4, io.ErrShortWrite, io.EOF},               // Force error in prev block hash.
		{36, io.ErrShortWrite, io.EOF},              // Force error in merkle root.
		{68, io.ErrShortWrite, io.EOF},              // Force error in timestamp.
		{72, io.ErrShortWrite, io.EOF},              // Force error in difficulty bits.
		{76, io.ErrShortWrite, io.EOF},              // Force error at start of proof commitment.
		{92, io.ErrShortWrite, io.ErrUnexpectedEOF}, // Force error in middle of proof commitment.
		{108, io.ErrShortWrite, io.EOF},             // Force error in transaction count.
		{112, io.ErrShortWrite, io.EOF},             // Force error in num hashes.
		{113, io.ErrShortWrite, io.EOF},             // Force error in hashes.
		{145, io.ErrShortWrite, io.EOF},             // Force error in num flag bytes.
		{146, io.ErrShortWrite, io.EOF},             // Force error in flag bytes.
	}

	for i, test := range tests {
		w := newFixedWriter(test.max)
		err := merkleBlockOne.PrlEncode(w, pver, BaseEncoding)
		require.ErrorIs(t, err, test.writeErr, "encode test #%d", i)

		var msg MsgMerkleBlock
		r := newFixedReader(test.max, merkleBlockOneBytes)
		err = msg.PrlDecode(r, pver, BaseEncoding)
		require.ErrorIs(t, err, test.readErr, "decode test #%d", i)
	}
}

// TestMerkleBlockOverflowErrors performs tests to ensure encoding and decoding
// merkle blocks that are intentionally crafted to use large values for the
// number of hashes and flags are handled properly.
func TestMerkleBlockOverflowErrors(t *testing.T) {
	pver := ProtocolVersion

	// Create bytes for a merkle block that claims to have more than the max
	// allowed tx hashes.
	var buf bytes.Buffer
	WriteVarInt(&buf, pver, maxTxPerBlock+1)
	// numHashesOffset = header(108) + txcount(4) = 112
	numHashesOffset := 112
	exceedMaxHashes := make([]byte, numHashesOffset)
	copy(exceedMaxHashes, merkleBlockOneBytes[:numHashesOffset])
	exceedMaxHashes = append(exceedMaxHashes, buf.Bytes()...)

	// Create bytes for a merkle block that claims to have more than the max
	// allowed flag bytes.
	buf.Reset()
	WriteVarInt(&buf, pver, maxFlagsPerMerkleBlock+1)
	// numFlagBytesOffset = header(108) + txcount(4) + numhashes(1) + hash(32) = 145
	numFlagBytesOffset := 145
	exceedMaxFlagBytes := make([]byte, numFlagBytesOffset)
	copy(exceedMaxFlagBytes, merkleBlockOneBytes[:numFlagBytesOffset])
	exceedMaxFlagBytes = append(exceedMaxFlagBytes, buf.Bytes()...)

	tests := []struct {
		buf []byte
		err error
	}{
		{exceedMaxHashes, &MessageError{}},
		{exceedMaxFlagBytes, &MessageError{}},
	}

	for i, test := range tests {
		var msg MsgMerkleBlock
		r := bytes.NewReader(test.buf)
		err := msg.PrlDecode(r, pver, BaseEncoding)
		require.IsType(t, test.err, err, "test #%d", i)
	}
}

// merkleBlockOne is a merkle block created from block one of the block chain
// where the first transaction matches.
var merkleBlockOne = MsgMerkleBlock{
	Header: BlockHeader{
		Version: 1,
		PrevBlock: chainhash.Hash([chainhash.HashSize]byte{
			0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
			0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
			0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
			0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00,
		}),
		MerkleRoot: chainhash.Hash([chainhash.HashSize]byte{
			0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
			0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
			0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
			0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e,
		}),
		Timestamp: time.Unix(0x4966bc61, 0), // 2009-01-08 20:54:25 -0600 CST
		Bits:      0x1d00ffff,               // 486604799
	},
	Transactions: 1,
	Hashes: []*chainhash.Hash{
		(*chainhash.Hash)(&[chainhash.HashSize]byte{
			0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
			0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
			0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
			0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e,
		}),
	},
	Flags: []byte{0x80},
}

// merkleBlockOneBytes is the serialized bytes for a merkle block.
var merkleBlockOneBytes = []byte{
	0x01, 0x00, 0x00, 0x00, // Version 1
	0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
	0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
	0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
	0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // PrevBlock
	0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
	0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
	0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
	0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e, // MerkleRoot
	0x61, 0xbc, 0x66, 0x49, // Timestamp
	0xff, 0xff, 0x00, 0x1d, // Bits
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ProofCommitment (32 zero bytes)
	0x01, 0x00, 0x00, 0x00, // TxnCount
	0x01, // Num hashes
	0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
	0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
	0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
	0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e, // Hash
	0x01, // Num flag bytes
	0x80, // Flags
}
