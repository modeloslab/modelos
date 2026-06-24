// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"testing"

	"github.com/modelos/modelos/node/btcec"
	"github.com/modelos/modelos/node/btcec/schnorr"
	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/modelos/modelos/node/wire"
)

func mustKey(t *testing.T) (*btcec.PrivateKey, [32]byte) {
	t.Helper()
	k, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("NewPrivateKey: %v", err)
	}
	var x [32]byte
	copy(x[:], schnorr.SerializePubKey(k.PubKey()))
	return k, x
}

func signMsg(t *testing.T, k *btcec.PrivateKey, msg []byte) [64]byte {
	t.Helper()
	sig, err := schnorr.Sign(k, msg)
	if err != nil {
		t.Fatalf("schnorr.Sign: %v", err)
	}
	var out [64]byte
	copy(out[:], sig.Serialize())
	return out
}

// claimFixture builds a valid claim: worker + coordinator keys, a request-bound
// commitment, both signatures.  Returns the proof, the covenant params, the
// confirming block hash, and the requester key (for refund tests).
func claimFixture(t *testing.T) (*wire.InferenceResultProof, *InferenceBountyParams, chainhash.Hash, *btcec.PrivateKey) {
	t.Helper()
	workerKey, workerX := mustKey(t)
	coordKey, coordX := mustKey(t)
	reqKey, reqX := mustKey(t)

	var confirmBlock chainhash.Hash
	for i := range confirmBlock {
		confirmBlock[i] = byte(i + 1)
	}

	params := &InferenceBountyParams{
		RequesterPubKey:   reqX,
		CoordinatorPubKey: coordX,
		Nonce:             0x1122334455667788,
		ModelVersion:      uint16(wire.ModelVersionDeepSeekR1_7B),
	}
	for i := range params.PromptHash {
		params.PromptHash[i] = byte(0x30 + i)
	}

	proof := &wire.InferenceResultProof{}
	for i := range proof.ResultHash {
		proof.ResultHash[i] = byte(0x50 + i)
	}
	proof.WorkerAddress[0] = 0x51 // OP_1
	proof.WorkerAddress[1] = 0x20 // push 32
	copy(proof.WorkerAddress[2:34], workerX[:])
	proof.BlockHash = confirmBlock

	commit := recomputeInferenceCommitment(
		params.PromptHash, proof.ResultHash, proof.WorkerAddress,
		proof.BlockHash, params.Nonce, params.ModelVersion,
	)
	proof.CommitmentHash = commit
	proof.Signature = signMsg(t, workerKey, commit[:])
	proof.CoordinatorSignature = signMsg(t, coordKey, commit[:])

	return proof, params, confirmBlock, reqKey
}

// proofTx wraps a proof in a v4 inference_proof_tx that spends bountyOut.
func proofTx(proof *wire.InferenceResultProof, bountyOut wire.OutPoint) *wire.MsgTx {
	var body bytes.Buffer
	_ = proof.Serialise(&body)
	script := append([]byte{0x6a, 0x4d, 0x02, 0x01}, body.Bytes()...) // OP_RETURN OP_PUSHDATA2 258
	tx := wire.NewMsgTx(wire.TxVersionInferenceProof)
	tx.AddTxIn(&wire.TxIn{PreviousOutPoint: bountyOut})
	tx.AddTxOut(&wire.TxOut{Value: 0, PkScript: script})
	return tx
}

func TestInferenceBountyScriptRoundTrip(t *testing.T) {
	_, params, _, _ := claimFixture(t)
	script := BuildInferenceBountyScript(params)
	if len(script) != inferenceBountyScriptSize {
		t.Fatalf("script len = %d, want %d", len(script), inferenceBountyScriptSize)
	}
	if !IsInferenceBountyScript(script) {
		t.Fatal("IsInferenceBountyScript = false for a valid script")
	}
	got, ok := ParseInferenceBountyScript(script)
	if !ok {
		t.Fatal("ParseInferenceBountyScript ok=false")
	}
	if *got != *params {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", *got, *params)
	}
	// A plain P2TR-ish script must not be recognised.
	if IsInferenceBountyScript([]byte{0x51, 0x20}) {
		t.Fatal("non-covenant script recognised as bounty")
	}
}

func TestInferenceBountyClaimValid(t *testing.T) {
	proof, params, confirmBlock, _ := claimFixture(t)
	if err := ValidateInferenceBountyClaim(proof, params, &confirmBlock); err != nil {
		t.Fatalf("valid claim rejected: %v", err)
	}
}

func TestInferenceBountyClaimWrongCoordinator(t *testing.T) {
	proof, params, confirmBlock, _ := claimFixture(t)
	// Re-sign the coordinator attestation with a DIFFERENT key → must fail.
	otherKey, _ := mustKey(t)
	proof.CoordinatorSignature = signMsg(t, otherKey, proof.CommitmentHash[:])
	if err := ValidateInferenceBountyClaim(proof, params, &confirmBlock); err == nil {
		t.Fatal("claim with attestation from the wrong coordinator was accepted")
	}
}

func TestInferenceBountyClaimWrongCommitment(t *testing.T) {
	proof, params, confirmBlock, _ := claimFixture(t)
	// Tamper the request's prompt_hash so the commitment no longer binds.
	params.PromptHash[0] ^= 0xff
	if err := ValidateInferenceBountyClaim(proof, params, &confirmBlock); err == nil {
		t.Fatal("claim with a commitment that doesn't bind to the request was accepted")
	}
}

func TestInferenceBountyClaimWrongBlock(t *testing.T) {
	proof, params, _, _ := claimFixture(t)
	var otherBlock chainhash.Hash
	otherBlock[0] = 0xaa
	if err := ValidateInferenceBountyClaim(proof, params, &otherBlock); err == nil {
		t.Fatal("claim referencing the wrong confirming block was accepted")
	}
}

func TestInferenceBountyClaimWrongWorkerSig(t *testing.T) {
	proof, params, confirmBlock, _ := claimFixture(t)
	otherKey, _ := mustKey(t)
	proof.Signature = signMsg(t, otherKey, proof.CommitmentHash[:])
	if err := ValidateInferenceBountyClaim(proof, params, &confirmBlock); err == nil {
		t.Fatal("claim with a worker signature not matching WorkerAddress was accepted")
	}
}

func TestInferenceBountyRefundValid(t *testing.T) {
	_, params, _, reqKey := claimFixture(t)
	bounty := wire.OutPoint{Index: 0}
	for i := range bounty.Hash {
		bounty.Hash[i] = byte(i + 1)
	}
	msg := InferenceRefundSigHash(&bounty)
	sig := signMsg(t, reqKey, msg[:])
	if err := ValidateInferenceBountyRefund(sig, params, &bounty); err != nil {
		t.Fatalf("valid requester refund rejected: %v", err)
	}
	// A refund signed by a non-requester key must fail.
	otherKey, _ := mustKey(t)
	bad := signMsg(t, otherKey, msg[:])
	if err := ValidateInferenceBountyRefund(bad, params, &bounty); err == nil {
		t.Fatal("refund by a non-requester key was accepted")
	}
	// A refund bound to a DIFFERENT outpoint must fail (no replay).
	other := wire.OutPoint{Index: 7}
	if err := ValidateInferenceBountyRefund(sig, params, &other); err == nil {
		t.Fatal("refund replayed onto a different bounty outpoint was accepted")
	}
}

func TestCheckInferenceBountySpendWindow(t *testing.T) {
	proof, params, confirmBlock, reqKey := claimFixture(t)
	bounty := wire.OutPoint{Index: 0}
	for i := range bounty.Hash {
		bounty.Hash[i] = byte(0x11 + i)
	}
	var zeroSig [64]byte

	// Within window (spend at confirm+1): a valid CLAIM must pass.
	claimTx := proofTx(proof, bounty)
	if err := CheckInferenceBountySpend(claimTx, params, &bounty, 100, 101, &confirmBlock, zeroSig); err != nil {
		t.Fatalf("in-window valid claim rejected: %v", err)
	}

	// Within window: a REFUND (no proof) must be rejected — no front-running.
	refundTx := wire.NewMsgTx(1)
	refundTx.AddTxIn(&wire.TxIn{PreviousOutPoint: bounty})
	refundTx.AddTxOut(&wire.TxOut{Value: 1000, PkScript: []byte{0x51, 0x20}})
	if err := CheckInferenceBountySpend(refundTx, params, &bounty, 100, 101, &confirmBlock, zeroSig); err == nil {
		t.Fatal("in-window refund (no proof) was accepted — requester front-ran the window")
	}

	// At the window edge (spend at confirm+window): still a claim window.
	edge := 100 + wire.InferenceProofWindowBlocks
	if err := CheckInferenceBountySpend(claimTx, params, &bounty, 100, edge, &confirmBlock, zeroSig); err != nil {
		t.Fatalf("claim at window edge rejected: %v", err)
	}

	// After the window: a valid requester REFUND must pass...
	after := 100 + wire.InferenceProofWindowBlocks + 1
	msg := InferenceRefundSigHash(&bounty)
	refundSig := signMsg(t, reqKey, msg[:])
	if err := CheckInferenceBountySpend(refundTx, params, &bounty, 100, after, &confirmBlock, refundSig); err != nil {
		t.Fatalf("post-window requester refund rejected: %v", err)
	}
	// ...and a refund with a bad signature must fail.
	if err := CheckInferenceBountySpend(refundTx, params, &bounty, 100, after, &confirmBlock, zeroSig); err == nil {
		t.Fatal("post-window refund with an empty signature was accepted")
	}
}
