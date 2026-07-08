// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/zeebo/blake3"
)

// AuxPowVersion is the canonical block version word for AuxPoW blocks.
//   - Bit  0  (0x0001): base version 1
//   - Bit  8  (0x0100): AuxPoW flag  (Dogecoin/Namecoin convention)
//   - Bit 12  (0x1000): ZK chain marker (Pearl/modelOS specific)
const AuxPowVersion = int32(0x1101)

// PearlHeaderSize is the byte length of a serialized Pearl block header.
// Layout: Version(4) + PrevBlock(32) + MerkleRoot(32) + Timestamp(4) + Bits(4) + ProofCommitment(32).
const PearlHeaderSize = 108

// pearl header field offsets within PearlHeaderSize bytes.
const (
	pearlHdrMerkleRootOffset = 36
	pearlHdrBitsOffset       = 72
)

// auxPowMask is the bitmask checked by IsAuxPowBlock.
const auxPowMask = int32(0x1101)

// AuxPowMagic is the 4-byte marker embedded in the Pearl coinbase scriptSig
// before the σ_modelos commitment, identifying this as a modelOS merged-mining
// coinbase.  Analogous to Dogecoin's 0xfabe6d6d merged-mining magic.
// "MDL*" in ASCII.
var AuxPowMagic = [4]byte{0x4d, 0x44, 0x4c, 0x2a}

// MaxCoinbaseBranchDepth caps the depth of the coinbase Merkle branch.
// A Pearl block with up to 2^32 transactions needs depth 32.  Practical
// blocks are far shallower; this bound prevents DoS via forced hashing.
const MaxCoinbaseBranchDepth = 32

// maxPearlHeight is the upper bound on the PearlHeight field.
const maxPearlHeight = int32(1_000_000_000)

// blake3Sum32 returns the 32-byte BLAKE3 digest of data.
// This is the canonical hash for all AuxPoW operations — identical to the
// CUDA kernel's κ = BLAKE3(…) formula.
func blake3Sum32(data []byte) [32]byte {
	return blake3.Sum256(data)
}

// AuxPowData is the Auxiliary Proof-of-Work data carried by modelOS blocks
// whose version satisfies `Version & 0x1101 == 0x1101`.
//
// Architecture (mirrors Dogecoin / Litecoin AuxPoW exactly):
//
//  1. Before mining, the modelOS pool embeds σ_modelos (modelOS prevBlock hash)
//     into the Pearl block's COINBASE transaction scriptSig, exactly as
//     Dogecoin embeds its block hash into the Litecoin coinbase:
//
//     [AuxPowMagic 4 bytes] [σ_modelos 32 bytes]
//
//  2. The coinbase is the first transaction in the Pearl block.  Its txid is
//     the leftmost leaf of the Pearl transaction Merkle tree.  The Pearl block
//     header's MerkleRoot commits to it.  MerkleRoot is in the Pearl block
//     header's incomplete_header_bytes which drives the CUDA key derivation:
//
//     key = BLAKE3(incomplete_header_bytes || mining_config)
//
//     σ_modelos is thus committed in the ZK proof's key — NO kernel change
//     needed.  The commitment propagates automatically through the Merkle tree.
//
//  3. When a solution is found the pool assembles AuxPowData, including the
//     full 108-byte Pearl block header, and submits to BOTH chains simultaneously.
//
// Verification by modelOS nodes (all local, no Pearl node required):
//
//	a. BLAKE3(PearlHeader) ≤ CompactToBig(PearlHeader.Bits)      [Pearl PoW]
//	b. CompactToBig(PearlHeader.Bits) ≤ modelOSTarget            [difficulty]
//	c. AuxPowMagic + σ_modelos appears in PearlCoinbaseTx        [commitment]
//	d. SHA256d(PearlCoinbaseTx) up CoinbaseBranch
//	   == PearlHeader.MerkleRoot                                 [Merkle proof]
//	e. header.ProofCommitment ==
//	   BLAKE3(networkMagic || CertVersionNull || AuxPowData)     [binding]
//
// Wire layout:
//
//	PearlHeader       [108]  full Pearl block header (Version+PrevBlock+MerkleRoot+Timestamp+Bits+ProofCommitment)
//	ModelOSStateHash   [32]  σ_modelos — embedded in Pearl coinbase and in κ
//	PearlHeight         [4]  Pearl block height > 0 (LE int32)
//	CoinbaseTxLen       [4]  byte length of PearlCoinbaseTx (LE uint32)
//	PearlCoinbaseTx   [var]  serialised Pearl coinbase transaction
//	BranchLen           [1]  number of nodes in CoinbaseBranch (0–32)
//	CoinbaseBranch    [N×33] Merkle siblings: hash (32) + IsLeft flag (1)
type AuxPowData struct {
	// PearlHeader is the full 108-byte Pearl block header.
	// Carrying the full header (like Dogecoin's CAuxPow.parentBlock) enables
	// immediate local verification of both Pearl PoW and the coinbase Merkle root.
	PearlHeader [PearlHeaderSize]byte

	// ModelOSStateHash is σ_modelos (little-endian prevBlock hash of the
	// modelOS block being mined).  It must appear in PearlCoinbaseTx preceded
	// by AuxPowMagic.
	ModelOSStateHash chainhash.Hash

	// PearlHeight is the Pearl block height.  Must be > 0.
	PearlHeight int32

	// PearlPublicData is the committed public-data prefix of the real Pearl ZK
	// certificate (PublicDataSize bytes). Together with PearlProofData it lets a
	// modelOS node verify the parent's genuine NoisyGEMM proof-of-useful-work —
	// the Pearl header's ProofCommitment must equal SHA256d(CertVersion||PublicData).
	PearlPublicData [PublicDataSize]byte

	// PearlProofData is the plonky2 proof blob of the real Pearl ZK certificate
	// (≤ MaxZKProofSize). This is the actual parent proof-of-work that the
	// modelOS chain inherits — NOT a re-hash of the header.
	PearlProofData []byte

	// PearlCoinbaseTx is the serialised Pearl coinbase transaction.
	// It must contain AuxPowMagic + ModelOSStateHash in its scriptSig.
	PearlCoinbaseTx []byte

	// CoinbaseBranch is the Merkle branch from the Pearl coinbase txid to
	// PearlHeader.MerkleRoot.  Empty when the Pearl block has only one
	// transaction (coinbase = Merkle root).
	CoinbaseBranch []AuxPowMerkleNode
}

// PearlBlockHash returns BLAKE3(PearlHeader), the canonical Pearl block hash.
func (a *AuxPowData) PearlBlockHash() chainhash.Hash {
	sum := blake3Sum32(a.PearlHeader[:])
	var h chainhash.Hash
	copy(h[:], sum[:])
	return h
}

// PearlMerkleRoot extracts the MerkleRoot field from PearlHeader.
func (a *AuxPowData) PearlMerkleRoot() chainhash.Hash {
	var h chainhash.Hash
	copy(h[:], a.PearlHeader[pearlHdrMerkleRootOffset:pearlHdrMerkleRootOffset+32])
	return h
}

// PearlHeaderBits extracts the compact difficulty target from PearlHeader.
func (a *AuxPowData) PearlHeaderBits() uint32 {
	return binary.LittleEndian.Uint32(a.PearlHeader[pearlHdrBitsOffset : pearlHdrBitsOffset+4])
}

// AuxPowMerkleNode is one sibling in a Merkle branch.
type AuxPowMerkleNode struct {
	Hash   chainhash.Hash
	IsLeft bool // true when this sibling is the left child
}

// SerialiseSize returns the wire byte count of AuxPowData.
func (a *AuxPowData) SerialiseSize() int {
	// PearlHeader + ModelOSStateHash(32) + PearlHeight(4) + PearlPublicData +
	// PearlProofLen(4) + PearlProofData + CoinbaseTxLen(4) + coinbase + BranchLen(1) + branch.
	return PearlHeaderSize + 32 + 4 + PublicDataSize + 4 + len(a.PearlProofData) +
		4 + len(a.PearlCoinbaseTx) + 1 + len(a.CoinbaseBranch)*33
}

// Serialise writes AuxPowData to w in canonical wire format.
func (a *AuxPowData) Serialise(w io.Writer) error {
	if _, err := w.Write(a.PearlHeader[:]); err != nil {
		return err
	}
	if _, err := w.Write(a.ModelOSStateHash[:]); err != nil {
		return err
	}
	// PearlHeight(4)
	var heightBuf [4]byte
	binary.LittleEndian.PutUint32(heightBuf[:], uint32(a.PearlHeight))
	if _, err := w.Write(heightBuf[:]); err != nil {
		return err
	}
	// Real Pearl ZK certificate: PublicData(PublicDataSize) + ProofLen(4) + ProofData.
	if _, err := w.Write(a.PearlPublicData[:]); err != nil {
		return err
	}
	var proofLenBuf [4]byte
	binary.LittleEndian.PutUint32(proofLenBuf[:], uint32(len(a.PearlProofData)))
	if _, err := w.Write(proofLenBuf[:]); err != nil {
		return err
	}
	if len(a.PearlProofData) > 0 {
		if _, err := w.Write(a.PearlProofData); err != nil {
			return err
		}
	}
	// CoinbaseTxLen(4)
	var cbLenBuf [4]byte
	binary.LittleEndian.PutUint32(cbLenBuf[:], uint32(len(a.PearlCoinbaseTx)))
	if _, err := w.Write(cbLenBuf[:]); err != nil {
		return err
	}
	if len(a.PearlCoinbaseTx) > 0 {
		if _, err := w.Write(a.PearlCoinbaseTx); err != nil {
			return err
		}
	}
	// BranchLen (1 byte)
	if _, err := w.Write([]byte{uint8(len(a.CoinbaseBranch))}); err != nil {
		return err
	}
	for _, node := range a.CoinbaseBranch {
		if _, err := w.Write(node.Hash[:]); err != nil {
			return err
		}
		isLeft := byte(0)
		if node.IsLeft {
			isLeft = 1
		}
		if _, err := w.Write([]byte{isLeft}); err != nil {
			return err
		}
	}
	return nil
}

// Deserialise reads and validates AuxPowData from r.
func (a *AuxPowData) Deserialise(r io.Reader) error {
	if _, err := io.ReadFull(r, a.PearlHeader[:]); err != nil {
		return err
	}
	// Validate the bits field embedded in the header.
	bits := a.PearlHeaderBits()
	if bits == 0 {
		return errors.New("auxpow: PearlBits (from header) is zero")
	}
	if bits&0x00800000 != 0 {
		return errors.New("auxpow: PearlBits has sign bit set (negative compact target invalid)")
	}

	if _, err := io.ReadFull(r, a.ModelOSStateHash[:]); err != nil {
		return err
	}
	var heightBuf [4]byte
	if _, err := io.ReadFull(r, heightBuf[:]); err != nil {
		return err
	}
	a.PearlHeight = int32(binary.LittleEndian.Uint32(heightBuf[:]))
	if a.PearlHeight <= 0 {
		return fmt.Errorf("auxpow: PearlHeight %d must be positive", a.PearlHeight)
	}
	if a.PearlHeight > maxPearlHeight {
		return fmt.Errorf("auxpow: PearlHeight %d exceeds maximum", a.PearlHeight)
	}

	// Real Pearl ZK certificate: PublicData(PublicDataSize) + ProofLen(4) + ProofData.
	if _, err := io.ReadFull(r, a.PearlPublicData[:]); err != nil {
		return err
	}
	var proofLenBuf [4]byte
	if _, err := io.ReadFull(r, proofLenBuf[:]); err != nil {
		return err
	}
	proofLen := binary.LittleEndian.Uint32(proofLenBuf[:])
	if proofLen == 0 {
		return errors.New("auxpow: PearlProofData missing — parent proof-of-work unverifiable")
	}
	if proofLen > MaxZKProofSize {
		return fmt.Errorf("auxpow: PearlProofData length %d exceeds max %d", proofLen, MaxZKProofSize)
	}
	a.PearlProofData = make([]byte, proofLen)
	if _, err := io.ReadFull(r, a.PearlProofData); err != nil {
		return err
	}

	// CoinbaseTxLen
	var cbLenBuf [4]byte
	if _, err := io.ReadFull(r, cbLenBuf[:]); err != nil {
		return err
	}
	cbLen := binary.LittleEndian.Uint32(cbLenBuf[:])
	if cbLen > 1_000_000 {
		return fmt.Errorf("auxpow: PearlCoinbaseTx length %d exceeds 1 MB", cbLen)
	}
	if cbLen > 0 {
		a.PearlCoinbaseTx = make([]byte, cbLen)
		if _, err := io.ReadFull(r, a.PearlCoinbaseTx); err != nil {
			return err
		}
	}

	// BranchLen
	var branchLenBuf [1]byte
	if _, err := io.ReadFull(r, branchLenBuf[:]); err != nil {
		return err
	}
	branchLen := int(branchLenBuf[0])
	if branchLen > MaxCoinbaseBranchDepth {
		return fmt.Errorf("auxpow: CoinbaseBranch depth %d exceeds maximum %d",
			branchLen, MaxCoinbaseBranchDepth)
	}
	a.CoinbaseBranch = make([]AuxPowMerkleNode, branchLen)
	for i := range a.CoinbaseBranch {
		var nodeBuf [33]byte
		if _, err := io.ReadFull(r, nodeBuf[:]); err != nil {
			return err
		}
		copy(a.CoinbaseBranch[i].Hash[:], nodeBuf[0:32])
		a.CoinbaseBranch[i].IsLeft = nodeBuf[32] != 0
	}
	return nil
}

// AuxPowProofCommitment computes the value required in BlockHeader.ProofCommitment
// for an AuxPoW block.
//
//	BLAKE3(networkMagic_LE4 || CertVersionNull_LE4 || AuxPowData_serialised)
//
// The network magic acts as a domain separator preventing cross-network replay.
// All hashing uses BLAKE3 — no SHA256d anywhere in this path.
func AuxPowProofCommitment(auxPow *AuxPowData, network PearlNet) (chainhash.Hash, error) {
	var buf bytes.Buffer
	var magic [4]byte
	binary.LittleEndian.PutUint32(magic[:], uint32(network))
	buf.Write(magic[:])
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00}) // CertificateVersionNull
	if err := auxPow.Serialise(&buf); err != nil {
		return chainhash.Hash{}, fmt.Errorf("auxpow: proof commitment serialise: %w", err)
	}
	sum := blake3Sum32(buf.Bytes())
	var h chainhash.Hash
	copy(h[:], sum[:])
	return h, nil
}

// VerifyCoinbaseMerkleRoot traverses the CoinbaseBranch upward from the
// Pearl coinbase txid and returns the computed Merkle root.
// The result must equal PearlHeader.MerkleRoot for the proof to be valid.
//
// txid must be the SHA256d (double-SHA256) of the serialised Pearl coinbase
// transaction in internal byte order (little-endian).
func VerifyCoinbaseMerkleRoot(auxPow *AuxPowData, coinbaseTxid chainhash.Hash) (chainhash.Hash, error) {
	current := coinbaseTxid
	for _, node := range auxPow.CoinbaseBranch {
		var combined [64]byte
		if node.IsLeft {
			copy(combined[0:32], node.Hash[:])
			copy(combined[32:64], current[:])
		} else {
			copy(combined[0:32], current[:])
			copy(combined[32:64], node.Hash[:])
		}
		// Pearl uses SHA256d for its transaction Merkle tree (Bitcoin compatibility).
		current = chainhash.DoubleHashH(combined[:])
	}
	return current, nil
}

// IsAuxPowBlock reports whether a block carries AuxPoW data.
// All three marker bits (0, 8, 12) must be set.
func IsAuxPowBlock(header *BlockHeader) bool {
	return header.Version&auxPowMask == auxPowMask
}

// ContainsModelosCommitment reports whether raw coinbase scriptSig bytes
// contain the AuxPowMagic marker followed by the expected σ_modelos hash.
// Returns the byte offset of the magic on success, -1 if not found.
//
// Security: the AuxPowMagic marker MUST appear exactly once in the scriptSig.
// If it appears more than once the commitment is rejected (-1).  This mirrors
// the standard merged-mining guard (Dogecoin/Namecoin "multiple merged-mining
// headers" attack), which prevents a miner from embedding several aux-chain
// commitments in a single coinbase and claiming the parent's work for whichever
// one is convenient.
func ContainsModelosCommitment(scriptSig []byte, sigmaModelos chainhash.Hash) int {
	found := -1
	magicCount := 0
	for i := 0; i <= len(scriptSig)-4; i++ {
		if scriptSig[i] == AuxPowMagic[0] &&
			scriptSig[i+1] == AuxPowMagic[1] &&
			scriptSig[i+2] == AuxPowMagic[2] &&
			scriptSig[i+3] == AuxPowMagic[3] {
			magicCount++
			if magicCount > 1 {
				// More than one merged-mining marker — reject outright.
				return -1
			}
			// Record the offset only when the magic is followed by the exact
			// expected σ_modelos hash (32 bytes must be available after it).
			if i+36 <= len(scriptSig) && bytes.Equal(scriptSig[i+4:i+36], sigmaModelos[:]) {
				found = i
			}
		}
	}
	return found
}
