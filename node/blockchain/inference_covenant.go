// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/binary"

	"github.com/modelos/modelos/node/btcec/schnorr"
	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/modelos/modelos/node/wire"
	"golang.org/x/crypto/sha3"
)

// ──────────────────────────────────────────────────────────────────────────────
// Inference bounty covenant (marketplace v1, HARD FORK)
//
// An inference_tx (version 3) locks its fee in TxOut[0] under a SELF-CONTAINED
// covenant script.  The node enforces, when that output is spent:
//
//	within InferenceProofWindowBlocks of the inference_tx's confirmation:
//	    spendable ONLY by a CLAIM — a tx carrying a valid InferenceResultProof
//	    whose commitment binds to THIS request (prompt_hash/nonce/model), whose
//	    worker BIP-340 signature verifies, AND whose coordinator BIP-340
//	    attestation verifies against the CoordinatorPubKey the requester named.
//	    The requester CANNOT key-spend during the window (no front-running).
//
//	after the window:
//	    spendable ONLY by the requester (REFUND) — a BIP-340 signature by the
//	    requester key over the refund sighash bound to this specific bounty.
//
// The output is self-contained (it carries every parameter the covenant needs)
// so the node never has to look the original inference_tx back up from the
// chain when validating a spend — only the confirm height (from the UTXO entry)
// and the confirming block hash (from the block index) are needed externally.
//
// The 90/10 worker/coordinator split is enforced by co-signing off-chain (the
// worker won't sign without its 90% output, the coordinator won't attest
// without its 10%), so the on-chain covenant only requires both signatures plus
// a well-formed, request-bound proof.
// ──────────────────────────────────────────────────────────────────────────────

// inferenceBountyMagic ("mdlb") prefixes every inference bounty covenant script
// so the node can recognise the output type from the pkScript alone.
var inferenceBountyMagic = []byte{'m', 'd', 'l', 'b'}

// inferenceBountyVersion is the covenant script serialisation version.
const inferenceBountyVersion = uint8(1)

// inferenceBountyDataSize is the covenant parameter blob length:
//
//	magic(4) + version(1) + requester_xonly(32) + coordinator(32) +
//	prompt_hash(32) + nonce(8) + model(2) = 111 bytes.
const inferenceBountyDataSize = 4 + 1 + 32 + 32 + 32 + 8 + 2

// inferenceBountyScriptSize is the full pkScript length: the blob is wrapped in
// an OP_PUSHDATA1 push (0x4c <len>) so it is VALID Script — wallet tooling can
// decode it and it is a spendable output in the UTXO set — while the node still
// recognises it from the "mdlb" magic and validates spends via the covenant
// rule (the script engine is skipped for covenant inputs).
const inferenceBountyScriptSize = 2 + inferenceBountyDataSize // 113 bytes

// refundSigHashTag domain-separates the refund signature so a requester's
// refund authorisation can never be replayed as any other kind of signature.
var refundSigHashTag = []byte("MDL-INFERENCE-REFUND")

// InferenceBountyParams are the covenant parameters carried by the bounty
// output's pkScript.
type InferenceBountyParams struct {
	// RequesterPubKey is the x-only BIP-340 key that may REFUND the bounty
	// after the proof window passes.
	RequesterPubKey [32]byte

	// CoordinatorPubKey is the x-only BIP-340 key whose attestation a CLAIM
	// must carry (it must equal the inference_tx's CoordinatorPubKey).
	CoordinatorPubKey [32]byte

	// PromptHash is the BLAKE3 prompt hash from the InferencePayload.  Binds a
	// claim's commitment to this exact request.
	PromptHash [32]byte

	// Nonce is the InferencePayload nonce (also the vLLM seed).
	Nonce uint64

	// ModelVersion is the requested model tier.
	ModelVersion uint16
}

// BuildInferenceBountyScript serialises the covenant parameters into the
// self-contained pkScript used for an inference_tx's TxOut[0].
func BuildInferenceBountyScript(p *InferenceBountyParams) []byte {
	buf := make([]byte, 0, inferenceBountyScriptSize)
	buf = append(buf, 0x4c, byte(inferenceBountyDataSize)) // OP_PUSHDATA1 <111>
	buf = append(buf, inferenceBountyMagic...)
	buf = append(buf, inferenceBountyVersion)
	buf = append(buf, p.RequesterPubKey[:]...)
	buf = append(buf, p.CoordinatorPubKey[:]...)
	buf = append(buf, p.PromptHash[:]...)
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], p.Nonce)
	buf = append(buf, n[:]...)
	var m [2]byte
	binary.LittleEndian.PutUint16(m[:], p.ModelVersion)
	buf = append(buf, m[:]...)
	return buf
}

// ParseInferenceBountyScript decodes an inference bounty covenant pkScript.
// Returns the params and true on success; (nil, false) for any non-covenant or
// malformed script.
func ParseInferenceBountyScript(pkScript []byte) (*InferenceBountyParams, bool) {
	if len(pkScript) != inferenceBountyScriptSize {
		return nil, false
	}
	// OP_PUSHDATA1 framing: 0x4c <dataLen> then the blob.
	if pkScript[0] != 0x4c || pkScript[1] != byte(inferenceBountyDataSize) {
		return nil, false
	}
	d := pkScript[2:]
	if !bytes.Equal(d[0:4], inferenceBountyMagic) {
		return nil, false
	}
	if d[4] != inferenceBountyVersion {
		return nil, false
	}
	p := &InferenceBountyParams{}
	o := 5
	copy(p.RequesterPubKey[:], d[o:o+32])
	o += 32
	copy(p.CoordinatorPubKey[:], d[o:o+32])
	o += 32
	copy(p.PromptHash[:], d[o:o+32])
	o += 32
	p.Nonce = binary.LittleEndian.Uint64(d[o : o+8])
	o += 8
	p.ModelVersion = binary.LittleEndian.Uint16(d[o : o+2])
	return p, true
}

// IsInferenceBountyScript reports whether pkScript is an inference bounty
// covenant output.
func IsInferenceBountyScript(pkScript []byte) bool {
	_, ok := ParseInferenceBountyScript(pkScript)
	return ok
}

// recomputeInferenceCommitment recomputes the proof commitment the way the
// worker built it:
//
//	SHA3-256(prompt_hash || result_hash || worker_address || block_hash ||
//	         nonce_le8 || model_version_le2)
//
// Binding the commitment to the covenant's prompt_hash/nonce/model is what stops
// a worker from claiming this bounty with a proof produced for a different
// request or model tier.
func recomputeInferenceCommitment(
	promptHash [32]byte,
	resultHash [32]byte,
	workerAddress [34]byte,
	blockHash chainhash.Hash,
	nonce uint64,
	modelVersion uint16,
) [32]byte {
	h := sha3.New256()
	h.Write(promptHash[:])
	h.Write(resultHash[:])
	h.Write(workerAddress[:])
	h.Write(blockHash[:])
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], nonce)
	h.Write(n[:])
	var m [2]byte
	binary.LittleEndian.PutUint16(m[:], modelVersion)
	h.Write(m[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// ValidateInferenceBountyClaim verifies that proof is a valid CLAIM against the
// bounty covenant params, confirmed by the block confirmBlockHash.  Returns nil
// iff the claim may release the bounty.
//
// Checks (all required):
//  1. commitment binds to (params.PromptHash, proof.ResultHash, proof.WorkerAddress,
//     proof.BlockHash, params.Nonce, params.ModelVersion);
//  2. proof.BlockHash == the block that confirmed the inference_tx;
//  3. the worker BIP-340 signature verifies over the commitment under the
//     x-only key embedded in WorkerAddress (P2TR: OP_1 0x20 <x-only>);
//  4. the coordinator BIP-340 attestation verifies over the commitment under
//     params.CoordinatorPubKey.
func ValidateInferenceBountyClaim(
	proof *wire.InferenceResultProof,
	params *InferenceBountyParams,
	confirmBlockHash *chainhash.Hash,
) error {
	// (2) the proof must reference the block that actually confirmed the request.
	if !bytes.Equal(proof.BlockHash[:], confirmBlockHash[:]) {
		return ruleError(ErrBadTxOutValue,
			"inference claim: proof BlockHash does not match the confirming block")
	}

	// (1) recompute + bind the commitment to THIS request.
	want := recomputeInferenceCommitment(
		params.PromptHash, proof.ResultHash, proof.WorkerAddress,
		proof.BlockHash, params.Nonce, params.ModelVersion,
	)
	if !bytes.Equal(want[:], proof.CommitmentHash[:]) {
		return ruleError(ErrBadTxOutValue,
			"inference claim: commitment does not bind to this request (prompt/nonce/model)")
	}

	// (3) worker signature.  WorkerAddress must be P2TR (OP_1 0x20 <x-only>).
	if proof.WorkerAddress[0] != 0x51 || proof.WorkerAddress[1] != 0x20 {
		return ruleError(ErrBadTxOutValue,
			"inference claim: WorkerAddress is not a P2TR scriptPubKey")
	}
	workerKey, err := schnorr.ParsePubKey(proof.WorkerAddress[2:34])
	if err != nil {
		return ruleError(ErrBadTxOutValue, "inference claim: invalid worker x-only pubkey")
	}
	workerSig, err := schnorr.ParseSignature(proof.Signature[:])
	if err != nil {
		return ruleError(ErrBadTxOutValue, "inference claim: cannot parse worker signature")
	}
	if !workerSig.Verify(proof.CommitmentHash[:], workerKey) {
		return ruleError(ErrBadTxOutValue, "inference claim: worker signature does not verify")
	}

	// (4) coordinator attestation under the requester-named CoordinatorPubKey.
	coordKey, err := schnorr.ParsePubKey(params.CoordinatorPubKey[:])
	if err != nil {
		return ruleError(ErrBadTxOutValue, "inference claim: invalid coordinator x-only pubkey")
	}
	coordSig, err := schnorr.ParseSignature(proof.CoordinatorSignature[:])
	if err != nil {
		return ruleError(ErrBadTxOutValue, "inference claim: cannot parse coordinator attestation")
	}
	if !coordSig.Verify(proof.CommitmentHash[:], coordKey) {
		return ruleError(ErrBadTxOutValue,
			"inference claim: coordinator attestation does not verify against the named coordinator")
	}
	return nil
}

// InferenceRefundSigHash returns the message a requester signs to REFUND a
// specific bounty after the proof window.  Domain-separated and bound to the
// exact bounty outpoint so a refund authorisation cannot be replayed.
func InferenceRefundSigHash(bounty *wire.OutPoint) [32]byte {
	h := sha3.New256()
	h.Write(refundSigHashTag)
	h.Write(bounty.Hash[:])
	var idx [4]byte
	binary.LittleEndian.PutUint32(idx[:], bounty.Index)
	h.Write(idx[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// ValidateInferenceBountyRefund verifies a requester REFUND of the bounty after
// the window: a BIP-340 signature by params.RequesterPubKey over the refund
// sighash bound to the bounty outpoint.
func ValidateInferenceBountyRefund(
	refundSig [64]byte,
	params *InferenceBountyParams,
	bounty *wire.OutPoint,
) error {
	reqKey, err := schnorr.ParsePubKey(params.RequesterPubKey[:])
	if err != nil {
		return ruleError(ErrBadTxOutValue, "inference refund: invalid requester x-only pubkey")
	}
	sig, err := schnorr.ParseSignature(refundSig[:])
	if err != nil {
		return ruleError(ErrBadTxOutValue, "inference refund: cannot parse requester signature")
	}
	msg := InferenceRefundSigHash(bounty)
	if !sig.Verify(msg[:], reqKey) {
		return ruleError(ErrBadTxOutValue, "inference refund: requester signature does not verify")
	}
	return nil
}

// inferenceBountyWithinWindow reports whether a bounty confirmed at confirmHeight
// is still within the claim window at spendHeight.
func inferenceBountyWithinWindow(confirmHeight, spendHeight int32) bool {
	return spendHeight-confirmHeight <= wire.InferenceProofWindowBlocks
}

// checkInferenceBountySpends enforces the bounty covenant for every input of tx
// that spends an inference bounty covenant output.  Called from checkConnectBlock
// BEFORE the inputs are marked spent, so the bounty entry (covenant params +
// confirm height) is still resolvable from the view.
//
// Covenant inputs are NOT validated by the script engine (their script is not
// executable); checkBlockScripts/ValidateTransactionScripts skip them and this
// rule validates them instead.
func (b *BlockChain) checkInferenceBountySpends(tx *btcutil.Tx, node *blockNode, view *UtxoViewpoint) error {
	for _, txIn := range tx.MsgTx().TxIn {
		entry := view.LookupEntry(txIn.PreviousOutPoint)
		if entry == nil {
			continue
		}
		params, ok := ParseInferenceBountyScript(entry.PkScript())
		if !ok {
			continue
		}

		confirmHeight := entry.BlockHeight()
		ancestor := node.Ancestor(confirmHeight)
		if ancestor == nil {
			return ruleError(ErrBadTxOutValue,
				"inference bounty: cannot resolve the confirming block for the spend")
		}
		confirmHash := ancestor.Hash()

		// The refund path supplies the requester's BIP-340 signature as the
		// first witness element of the bounty-spending input (64 bytes); claim
		// inputs carry no witness (the proof is in the OP_RETURN).
		var refundSig [64]byte
		if w := txIn.Witness; len(w) >= 1 && len(w[0]) == 64 {
			copy(refundSig[:], w[0])
		}

		op := txIn.PreviousOutPoint
		if err := CheckInferenceBountySpend(
			tx.MsgTx(), params, &op, confirmHeight, node.height, &confirmHash, refundSig,
		); err != nil {
			return err
		}
	}
	return nil
}

// CheckInferenceBountySpendsAgainstTip validates any inference-bounty-spending
// inputs of tx against the CURRENT best-chain tip using the supplied view. It
// returns an error if a bound claim or refund is no longer valid at the tip —
// most importantly when a reorg has changed the block that confirmed the bounty
// so the claim's proof.BlockHash no longer matches the confirming block. Txs
// that don't spend an inference bounty return nil.
//
// This is a lightweight pre-filter (spend height = tip+1, mirroring the block
// that would include the tx) used by the mempool — to reject/evict stale claims,
// including on reorg re-add — and by the block-template generator — to SKIP them.
// It ensures a single reorg-stale inference claim can never fail the whole
// getblocktemplate and halt mining during reorgs.
func (b *BlockChain) CheckInferenceBountySpendsAgainstTip(tx *btcutil.Tx, view *UtxoViewpoint) error {
	// Resolve the tip lazily — only a tx that actually spends a bounty pays the
	// cost of reading it. This keeps the check cheap enough to run on EVERY
	// candidate tx regardless of version, which is required because a REFUND is
	// a plain v1 tx (not v3/v4) yet can go reorg-stale on a height-reducing
	// reorg; a version gate here would let a stale refund abort getblocktemplate.
	var tip *blockNode
	var spendHeight int32
	for _, txIn := range tx.MsgTx().TxIn {
		entry := view.LookupEntry(txIn.PreviousOutPoint)
		if entry == nil {
			continue
		}
		params, ok := ParseInferenceBountyScript(entry.PkScript())
		if !ok {
			continue
		}
		if tip == nil {
			tip = b.bestChain.Tip()
			if tip == nil {
				return nil
			}
			spendHeight = tip.height + 1
		}
		confirmHeight := entry.BlockHeight()
		ancestor := tip.Ancestor(confirmHeight)
		if ancestor == nil {
			return ruleError(ErrBadTxOutValue,
				"inference bounty: cannot resolve the confirming block at tip")
		}
		confirmHash := ancestor.Hash()

		var refundSig [64]byte
		if w := txIn.Witness; len(w) >= 1 && len(w[0]) == 64 {
			copy(refundSig[:], w[0])
		}

		op := txIn.PreviousOutPoint
		if err := CheckInferenceBountySpend(
			tx.MsgTx(), params, &op, confirmHeight, spendHeight, &confirmHash, refundSig,
		); err != nil {
			return err
		}
	}
	return nil
}

// CheckInferenceBountySpend is the full covenant rule applied when spendingTx
// spends the inference bounty output at bountyOutPoint (params), confirmed at
// confirmHeight in block confirmBlockHash, and the spend is being connected at
// spendHeight.
//
// Within the window it requires a valid CLAIM carried by spendingTx (a v4
// inference_proof_tx OP_RETURN); after the window it requires a requester
// REFUND signature (refundSig), which the spender supplies via the witness of
// the input that spends the bounty.  refundSig is ignored within the window,
// and the claim proof is ignored after it.
func CheckInferenceBountySpend(
	spendingTx *wire.MsgTx,
	params *InferenceBountyParams,
	bountyOutPoint *wire.OutPoint,
	confirmHeight int32,
	spendHeight int32,
	confirmBlockHash *chainhash.Hash,
	refundSig [64]byte,
) error {
	if inferenceBountyWithinWindow(confirmHeight, spendHeight) {
		proof, err := wire.ExtractInferenceResultProof(spendingTx)
		if err != nil {
			return ruleError(ErrBadTxOutValue,
				"inference bounty spent within window without a valid InferenceResultProof "+
					"(requester refund is not permitted until the proof window passes)")
		}
		return ValidateInferenceBountyClaim(proof, params, confirmBlockHash)
	}
	// After the window: only the requester may refund.
	return ValidateInferenceBountyRefund(refundSig, params, bountyOutPoint)
}
