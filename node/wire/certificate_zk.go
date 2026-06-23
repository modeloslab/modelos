// Copyright (c) 2025-2026 The Pearl Research Labs developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
)

// PublicDataSize is the size of the committed PublicData prefix.
// config(52) + hash_a(32) + hash_b(32) + hash_jackpot(32) + m(4) + n(4) + t_rows(4) + t_cols(4)
// Must match PublicProofParams::PUBLICDATA_SIZE in zk-pow/src/api/proof_utils.rs.
const PublicDataSize = 164

// ZKCertificate contains a ZK proof for production networks.
type ZKCertificate struct {
	// Block Hash
	Hash chainhash.Hash

	// PublicData contains the committed public fields.
	PublicData [PublicDataSize]byte

	// ProofData contains the plonky2 proof.
	ProofData []byte
}

func (c *ZKCertificate) Version() CertificateVersion {
	return CertificateVersionZK
}

func (c *ZKCertificate) BlockHash() chainhash.Hash {
	return c.Hash
}

// ProofCommitment computes SHA256d(CertificateVersion_LE(4) || PublicData(164)).
func (c *ZKCertificate) ProofCommitment() chainhash.Hash {
	var buf [4 + PublicDataSize]byte
	binary.LittleEndian.PutUint32(buf[:4], uint32(c.Version()))
	copy(buf[4:], c.PublicData[:])
	return chainhash.DoubleHashH(buf[:])
}

// Serialize: BlockHash(32) + PublicData(164) + ProofLen(4) + ProofData
// Version excluded - handled by MsgCertificate.
func (c *ZKCertificate) Serialize(w io.Writer) error {
	if _, err := w.Write(c.Hash[:]); err != nil {
		return err
	}
	if _, err := w.Write(c.PublicData[:]); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, uint32(len(c.ProofData))); err != nil {
		return err
	}
	if _, err := w.Write(c.ProofData); err != nil {
		return err
	}

	return nil
}

// Deserialize: BlockHash(32) + PublicData(164) + ProofLen(4) + ProofData
// Version excluded - handled by MsgCertificate.
func (c *ZKCertificate) Deserialize(r io.Reader) error {
	if _, err := io.ReadFull(r, c.Hash[:]); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, c.PublicData[:]); err != nil {
		return err
	}

	var proofLen uint32
	if err := binary.Read(r, binary.LittleEndian, &proofLen); err != nil {
		return err
	}
	if proofLen > MaxZKProofSize {
		return fmt.Errorf("proof data too large: %d bytes (max %d)", proofLen, MaxZKProofSize)
	}

	c.ProofData = make([]byte, proofLen)
	if _, err := io.ReadFull(r, c.ProofData); err != nil {
		return err
	}

	return nil
}

// SerializedSize returns the number of bytes needed to serialize the certificate fields.
// Format: BlockHash(32) + PublicData(164) + ProofLen(4) + ProofData
// Note: Does NOT include version (4 bytes) - that's handled by MsgCertificate.
func (c *ZKCertificate) SerializedSize() int {
	return 32 + PublicDataSize + 4 + len(c.ProofData)
}
