// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"testing"
)

// sampleInferencePayload returns a fully-populated marketplace-v1 InferencePayload.
func sampleInferencePayload() *InferencePayload {
	p := &InferencePayload{
		ModelVersion: ModelVersionDeepSeekR1_7B,
		MaxTokens:    4096,
		Nonce:        0x0102030405060708,
	}
	for i := range p.PromptHash {
		p.PromptHash[i] = byte(i + 1)
	}
	for i := range p.ResultAddress {
		p.ResultAddress[i] = byte(0x40 + i)
	}
	for i := range p.CoordinatorPubKey {
		p.CoordinatorPubKey[i] = byte(0xC0 + i)
	}
	return p
}

// sampleInferenceProof returns a fully-populated marketplace-v1 InferenceResultProof.
func sampleInferenceProof() *InferenceResultProof {
	p := &InferenceResultProof{}
	for i := range p.CommitmentHash {
		p.CommitmentHash[i] = byte(i + 1)
	}
	for i := range p.ResultHash {
		p.ResultHash[i] = byte(0x20 + i)
	}
	// Valid P2TR scriptPubKey: OP_1 (0x51) 0x20 <32-byte x-only key>.
	p.WorkerAddress[0] = 0x51
	p.WorkerAddress[1] = 0x20
	for i := 2; i < 34; i++ {
		p.WorkerAddress[i] = byte(0x80 + i)
	}
	for i := range p.BlockHash {
		p.BlockHash[i] = byte(0x90 + i)
	}
	for i := range p.Signature {
		p.Signature[i] = byte(0xA0 + i)
	}
	for i := range p.CoordinatorSignature {
		p.CoordinatorSignature[i] = byte(0x10 + i)
	}
	return p
}

// TestInferencePayloadSizes pins the marketplace-v1 fixed sizes.
func TestInferencePayloadSizes(t *testing.T) {
	if got := sampleInferencePayload().SerialiseSize(); got != 141 {
		t.Fatalf("InferencePayload.SerialiseSize = %d, want 141", got)
	}
	if got := sampleInferenceProof().SerialiseSize(); got != 258 {
		t.Fatalf("InferenceResultProof.SerialiseSize = %d, want 258", got)
	}
	if inferencePayloadVersion != 2 {
		t.Fatalf("inferencePayloadVersion = %d, want 2", inferencePayloadVersion)
	}
	if InferenceProofWindowBlocks != 5 {
		t.Fatalf("InferenceProofWindowBlocks = %d, want 5", InferenceProofWindowBlocks)
	}
}

// TestInferencePayloadRoundTrip serialises then deserialises and compares.
func TestInferencePayloadRoundTrip(t *testing.T) {
	want := sampleInferencePayload()

	var buf bytes.Buffer
	if err := want.Serialise(&buf); err != nil {
		t.Fatalf("Serialise: %v", err)
	}
	if buf.Len() != 141 {
		t.Fatalf("serialised %d bytes, want 141", buf.Len())
	}

	var got InferencePayload
	if err := got.Deserialise(&buf); err != nil {
		t.Fatalf("Deserialise: %v", err)
	}
	if got != *want {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, *want)
	}
}

// TestInferencePayloadOpReturnRoundTrip builds the OP_RETURN pkScript the way a
// wallet would and confirms the node parses it back, including the coordinator key.
func TestInferencePayloadOpReturnRoundTrip(t *testing.T) {
	want := sampleInferencePayload()

	var body bytes.Buffer
	if err := want.Serialise(&body); err != nil {
		t.Fatalf("Serialise: %v", err)
	}
	pkScript := append(append([]byte{}, inferenceOpReturnPrefix...), body.Bytes()...)
	if len(pkScript) != 144 {
		t.Fatalf("pkScript = %d bytes, want 144", len(pkScript))
	}

	got, ok := parseInferenceOpReturn(pkScript)
	if !ok {
		t.Fatal("parseInferenceOpReturn returned ok=false")
	}
	if *got != *want {
		t.Fatalf("OP_RETURN round-trip mismatch:\n got %+v\nwant %+v", *got, *want)
	}

	// Tamper: a 109-byte (old-format) payload must NOT parse.
	if _, ok := parseInferenceOpReturn(pkScript[:112]); ok {
		t.Fatal("old-format 112-byte script parsed as v1 payload; want reject")
	}
}

// TestInferenceProofOpReturnRoundTrip builds the OP_PUSHDATA2 proof script and
// confirms the node parses it back, including the coordinator attestation.
func TestInferenceProofOpReturnRoundTrip(t *testing.T) {
	want := sampleInferenceProof()

	var body bytes.Buffer
	if err := want.Serialise(&body); err != nil {
		t.Fatalf("Serialise: %v", err)
	}
	pkScript := append(append([]byte{}, inferenceProofOpReturnPrefix...), body.Bytes()...)
	if len(pkScript) != 262 {
		t.Fatalf("pkScript = %d bytes, want 262", len(pkScript))
	}

	got, ok := parseInferenceProofOpReturn(pkScript)
	if !ok {
		t.Fatal("parseInferenceProofOpReturn returned ok=false")
	}
	if got.CommitmentHash != want.CommitmentHash ||
		got.ResultHash != want.ResultHash ||
		got.WorkerAddress != want.WorkerAddress ||
		got.BlockHash != want.BlockHash ||
		got.Signature != want.Signature ||
		got.CoordinatorSignature != want.CoordinatorSignature {
		t.Fatalf("proof OP_RETURN round-trip mismatch:\n got %+v\nwant %+v", *got, *want)
	}

	// Tamper: a 197-byte (old OP_PUSHDATA1 194) proof script must NOT parse.
	old := append([]byte{0x6a, 0x4c, 0xc2}, body.Bytes()[:194]...)
	if _, ok := parseInferenceProofOpReturn(old); ok {
		t.Fatal("old-format 197-byte proof parsed; want reject")
	}
}
