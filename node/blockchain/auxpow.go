// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/modelos/modelos/node/wire"
	"github.com/modelos/modelos/node/zkpow"
)

// auxPowActivationHeight is the block height at which AuxPoW support activates.
// 0 = active from genesis (fresh v2 chain — wipe + restart, no legacy v1 history to gate).
const auxPowActivationHeight = int32(0)

// VerifyAuxPow validates the AuxPoW data attached to a modelOS block.
//
// This mirrors the Dogecoin / Litecoin merged-mining verification exactly.
// Carrying the full PearlHeader (like Dogecoin's CAuxPow.parentBlock) means
// all checks are local and immediate — no sync-layer deferral.
//
//	Dogecoin verifies:                       We verify:
//	─────────────────                        ──────────
//	HASH(parentBlock) ≤ AUX (child) target →  ZK jackpot(PearlHeader, proof) ≤ modelOSTarget
//	child hash in parent coinbase      →   σ_modelos in Pearl coinbase (AuxPowMagic prefix)
//	Coinbase Merkle branch valid       →   CoinbaseBranch matches PearlHeader.MerkleRoot
//
// CANONICAL AuxPoW (the part a reader must NOT get wrong): the parent (Pearl) header does NOT
// need to meet its OWN (Pearl) target. Merged mining's whole point is that the child (MDL) chain
// accepts parent work that is BELOW the parent's difficulty — so we grade the parent's genuine
// proof against the CHILD (modelOS) target ONLY. Pearl's own target is irrelevant here; a real
// Pearl block (PRL) is produced separately, upstream, only when the same proof ALSO clears Pearl.
// The common case (clears MDL, not Pearl) is a VALID MDL AuxPoW block — not an error.
//
// All hash operations use the correct function for each sub-step:
//   - Transaction txids:      SHA256d  (Pearl uses Bitcoin-compatible tx hashing)
//   - Transaction Merkle:     SHA256d  (Pearl's transaction Merkle tree)
//   - Block identity hash:    BLAKE3   (Pearl ZK-PoW block hash)
//   - AuxPoW proof commitment:BLAKE3   (AuxPowProofCommitment)
//
// Steps:
//  1. ZK proof valid AND committed jackpot ≤ modelOSTarget (child) — the parent's real PoW
//     satisfies MDL. (Pearl need NOT meet its own Pearl target — canonical merged mining.)
//  3. ModelOSStateHash == block's prevBlock hash (σ_modelos).
//  4. PearlHeight > 0.
//  5. Pearl coinbase contains AuxPowMagic + σ_modelos.
//  6. CoinbaseBranch reconstructs PearlHeader.MerkleRoot from the coinbase txid.
func VerifyAuxPow(
	auxPow *wire.AuxPowData,
	modelOSBlockHeight int32,
	modelOSStateHash *chainhash.Hash,
	modelOSTarget *big.Int,
	network wire.PearlNet,
) error {
	if modelOSBlockHeight < auxPowActivationHeight {
		return fmt.Errorf("auxpow: block %d is below activation height %d",
			modelOSBlockHeight, auxPowActivationHeight)
	}
	if auxPow == nil {
		return errors.New("auxpow: AuxPowData is nil")
	}

	// 1 & 2. REAL parent proof-of-work + difficulty scaling.
	//
	// Pearl's PoW is a NoisyGEMM ZK proof (not a hash), so we inherit it the way
	// Dogecoin inherits Litecoin's scrypt PoW: verify the parent's genuine proof and
	// grade its committed jackpot against THIS chain's (easier) target. We reuse the
	// exact same audited verifier the node uses for native blocks — never re-implement
	// circuit checks here.
	//
	// Reconstruct the Pearl block header from the carried 108 bytes and verify it
	// round-trips byte-identically (guards a malformed header whose BlockHash() would
	// not match the cert).
	var pearlHeader wire.BlockHeader
	if err := pearlHeader.Deserialize(bytes.NewReader(auxPow.PearlHeader[:])); err != nil {
		return fmt.Errorf("auxpow: cannot parse Pearl header: %w", err)
	}
	var roundTrip bytes.Buffer
	if err := pearlHeader.Serialize(&roundTrip); err != nil {
		return fmt.Errorf("auxpow: Pearl header re-serialise: %w", err)
	}
	if !bytes.Equal(roundTrip.Bytes(), auxPow.PearlHeader[:]) {
		return errors.New("auxpow: Pearl header is non-canonical")
	}

	// Build the Pearl ZK certificate from the carried public+proof data. cert.Hash is
	// computed FROM the header (never trusted from the wire) so a forged hash cannot
	// bypass the bind check inside the verifier.
	pearlCert := &wire.ZKCertificate{
		Hash:       pearlHeader.BlockHash(),
		PublicData: auxPow.PearlPublicData,
		ProofData:  auxPow.PearlProofData,
	}

	// Verify the real plonky2 proof is valid AND bound to the Pearl header
	// (cert.Hash == header.BlockHash, header.ProofCommitment == cert.ProofCommitment),
	// AND grade the committed jackpot against modelOS's target (nbits override). Grading
	// the parent proof against the CHILD target is the AuxPoW semantic — deliberate, not
	// a difficulty bypass.
	mdlBits := BigToCompact(modelOSTarget)
	if err := zkpow.VerifyZKCertificateWithNbits(&pearlHeader, pearlCert, mdlBits); err != nil {
		return fmt.Errorf("auxpow: Pearl ZK proof invalid or below modelOS target: %w", err)
	}

	// 3. ModelOSStateHash must equal this block's prevBlock hash (σ_modelos, LE).
	if auxPow.ModelOSStateHash != *modelOSStateHash {
		return fmt.Errorf("auxpow: ModelOSStateHash mismatch: proof=%v block=%v",
			auxPow.ModelOSStateHash, *modelOSStateHash)
	}

	// 4. PearlHeight sanity.
	if auxPow.PearlHeight <= 0 {
		return fmt.Errorf("auxpow: PearlHeight %d must be positive", auxPow.PearlHeight)
	}

	// 5. σ_modelos must appear in the Pearl coinbase scriptSig (preceded by AuxPowMagic).
	if len(auxPow.PearlCoinbaseTx) == 0 {
		return errors.New("auxpow: PearlCoinbaseTx is missing — coinbase commitment unverifiable")
	}
	if wire.ContainsModelosCommitment(auxPow.PearlCoinbaseTx, auxPow.ModelOSStateHash) < 0 {
		return fmt.Errorf(
			"auxpow: σ_modelos %v not found in Pearl coinbase (magic 0x4d444c2a + hash)",
			auxPow.ModelOSStateHash)
	}

	// 6. CoinbaseBranch must reconstruct PearlHeader.MerkleRoot from the coinbase txid.
	// MerkleRoot is extracted directly from PearlHeader — no external input required.
	// Coinbase txid = SHA256d(PearlCoinbaseTx) in internal (LE) byte order.
	coinbaseTxid := chainhash.DoubleHashH(auxPow.PearlCoinbaseTx)
	computedRoot, err := wire.VerifyCoinbaseMerkleRoot(auxPow, coinbaseTxid)
	if err != nil {
		return fmt.Errorf("auxpow: coinbase Merkle branch error: %w", err)
	}
	pearlMerkleRoot := auxPow.PearlMerkleRoot()
	if computedRoot != pearlMerkleRoot {
		return fmt.Errorf(
			"auxpow: coinbase Merkle branch root %v does not match Pearl block MerkleRoot %v",
			computedRoot, pearlMerkleRoot)
	}

	return nil
}
