// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
)

// ModelVersion identifies the on-chain LLM model version for an inference_tx.
type ModelVersion uint16

// NOTE: the version NUMBERS are consensus (encoded on-chain); the model behind each tier was
// refreshed (2026) to the current best-in-class reasoners. The identifiers keep their historical
// names; the comment on each gives the model actually served. Miners map version→model via
// vllm_miner MODEL_NAMES / compute INFERENCE_MODELS (keep all three in sync).
const (
	// ModelVersionDeepSeekR1_70B (v1) — DeepSeek-R1-Distill-Llama-70B.
	// ~140GB VRAM (2 GPUs, tensor-parallel).  Highest quality.
	ModelVersionDeepSeekR1_70B ModelVersion = 1

	// ModelVersionDeepSeekR1_32B (v2) — Qwen3-32B (thinking).
	// ~64GB VRAM (A100 80GB).  Very high quality.
	ModelVersionDeepSeekR1_32B ModelVersion = 2

	// ModelVersionDeepSeekR1_14B (v3) — Qwen3-14B (thinking).
	// ~28GB VRAM (A100 / RTX 3090).
	ModelVersionDeepSeekR1_14B ModelVersion = 3

	// ModelVersionDeepSeekR1_7B (v4) — DeepSeek-R1-0528-Qwen3-8B.
	// ~16GB VRAM (RTX 4080 / 3090+).  SOTA among 8B open reasoners.
	ModelVersionDeepSeekR1_7B ModelVersion = 4
)

// InferenceProofWindowBlocks is the number of blocks after the inference_tx
// is confirmed within which the bounty covenant is claimable by a worker +
// coordinator.  After this window passes, ONLY the requester can spend
// TxOut[0] (refund).
//
// Marketplace v1: bumped 3 → 5 for fast-block wall-time margin.  At 194s/block
// target this is ~970 seconds (≈16 minutes) — comfortable for a 4096-token 70B
// run.  CAVEAT: during fast bootstrap mining 5 blocks can be ~150s, which is
// tight vs a slow 70B run; watch during hashrate spikes.
const InferenceProofWindowBlocks = int32(5)

// InferenceResultStatus represents the fulfilment state of an inference_tx.
type InferenceResultStatus uint8

const (
	InferenceResultPending   InferenceResultStatus = 0
	InferenceResultFulfilled InferenceResultStatus = 1
	InferenceResultRefunded  InferenceResultStatus = 2
)

// inferencePayloadVersion is the serialisation version of InferencePayload.
//
// Marketplace v1 (HARD FORK): bumped 1 → 2.  Version 2 adds the 32-byte
// CoordinatorPubKey (the requester-named coordination layer that verifies the
// result and attests so the worker can claim).  The payload also grows
// 109 → 141 bytes; the OP_RETURN push length distinguishes the two formats,
// and the version byte makes the change explicit.
const inferencePayloadVersion = uint8(2)

// MaxInferenceTokens is the consensus-enforced cap on max_tokens in an
// InferencePayload.  It is deliberately set to the FORMAT ceiling — the maximum
// value the uint16 max_tokens field can hold — so consensus only enforces what
// the wire format physically allows, and NOT a per-model operational limit.
//
// Rationale (HARD FORK, bumped 4096 → 65535): as new/larger models are added,
// each has a different practical output ceiling (bounded by its context window,
// GPU speed, and the 5-block proof window). Encoding any specific number here
// would force a fresh consensus change every time the model set grows. Instead
// the OPERATIONAL caps live in the application layer — the compute UI's per-model
// `maxTokens`, and each worker's `max_model_len` (vLLM clamps generation to
// model_len − prompt) — which can change per model with zero consensus impact.
//
// Note: because this equals the uint16 maximum, the consensus check
// `payload.MaxTokens > MaxInferenceTokens` can never fail for a well-formed
// payload; that is intentional. A requester who commits more tokens than the
// assigned model can deliver simply overpays (per-token pricing) and receives a
// model_len-bounded response — a UI-guidance concern, not a safety one.
const MaxInferenceTokens = uint16(65535)

// InferencePayload is the structured data encoded in the OP_RETURN output of
// an inference_tx (transaction version 3).
//
// Marketplace v1 wire layout (fixed 141 bytes):
//
//	[0]        payload_version    uint8        — always 2
//	[1–32]     prompt_hash        [32]byte     — BLAKE3 hash of the plaintext prompt
//	[33–96]    result_address     [64]byte     — COORDINATOR delivery endpoint (IP:port / .onion)
//	[97–98]    model_version      uint16 LE    — model identifier (1 = DeepSeek R1 70B)
//	[99–100]   max_tokens         uint16 LE    — maximum response tokens (≤ MaxInferenceTokens)
//	[101–108]  nonce              uint64 LE    — replay protection / vLLM seed
//	[109–140]  coordinator_pubkey [32]byte     — x-only BIP-340 key of the named coordinator
//
// The requester PICKS the coordinator (it may be the pool, a third party, or
// the requester itself).  Within InferenceProofWindowBlocks the bounty is
// spendable ONLY by a tx carrying a valid worker proof AND a Schnorr
// attestation from THIS coordinator pubkey.  result_address now points at the
// coordinator (prompt fetch + result delivery + verification all route there).
type InferencePayload struct {
	// PromptHash is the BLAKE3 hash of the off-chain plaintext prompt.
	// The coordinator buffer matches incoming prompts by this hash.
	PromptHash [32]byte

	// ResultAddress is the IP:port or .onion address of the COORDINATOR that
	// serves the prompt, receives + verifies the result, and attests it.
	ResultAddress [64]byte

	// ModelVersion identifies the requested model.
	ModelVersion ModelVersion

	// MaxTokens is the maximum number of tokens the miner should generate.
	MaxTokens uint16

	// Nonce provides replay protection across identical prompt hashes and is
	// the vLLM sampling seed (binds the result to this exact run).
	Nonce uint64

	// CoordinatorPubKey is the x-only (BIP-340) public key of the coordination
	// layer the requester named.  The covenant requires this key's Schnorr
	// attestation (alongside the worker proof) to release the bounty.
	CoordinatorPubKey [32]byte
}

// SerialiseSize returns the fixed serialised size of InferencePayload.
func (p *InferencePayload) SerialiseSize() int {
	return 1 + 32 + 64 + 2 + 2 + 8 + 32 // 141 bytes
}

// Serialise writes the InferencePayload to w in the canonical wire format.
func (p *InferencePayload) Serialise(w io.Writer) error {
	var buf [141]byte
	buf[0] = inferencePayloadVersion
	copy(buf[1:33], p.PromptHash[:])
	copy(buf[33:97], p.ResultAddress[:])
	binary.LittleEndian.PutUint16(buf[97:99], uint16(p.ModelVersion))
	binary.LittleEndian.PutUint16(buf[99:101], p.MaxTokens)
	binary.LittleEndian.PutUint64(buf[101:109], p.Nonce)
	copy(buf[109:141], p.CoordinatorPubKey[:])
	_, err := w.Write(buf[:])
	return err
}

// Deserialise reads an InferencePayload from r.
func (p *InferencePayload) Deserialise(r io.Reader) error {
	var buf [141]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return err
	}
	if buf[0] != inferencePayloadVersion {
		return fmt.Errorf("unsupported inference payload version %d", buf[0])
	}
	copy(p.PromptHash[:], buf[1:33])
	copy(p.ResultAddress[:], buf[33:97])
	p.ModelVersion = ModelVersion(binary.LittleEndian.Uint16(buf[97:99]))
	p.MaxTokens = binary.LittleEndian.Uint16(buf[99:101])
	p.Nonce = binary.LittleEndian.Uint64(buf[101:109])
	copy(p.CoordinatorPubKey[:], buf[109:141])
	return nil
}

// InferenceResultProof is the cryptographic proof submitted within
// InferenceProofWindowBlocks blocks of the inference_tx being confirmed.
//
// OPEN INFERENCE MARKET, DELEGATED VERIFICATION: the node cannot run a 70B LLM
// to check result correctness, so a coordinator the requester PICKED does the
// quality check off-chain and ATTESTS.  The covenant releasing the bounty
// requires BOTH:
//   - the worker's BIP-340 signature over CommitmentHash (proves the worker ran
//     the exact model/prompt/seed), and
//   - the coordinator's BIP-340 attestation over CommitmentHash by the
//     CoordinatorPubKey named in the inference_tx (proves the result passed the
//     quality check).
//
// The 90/10 worker/coordinator split is enforced by co-signing: the worker
// won't sign without its 90% output and the coordinator won't attest without
// its 10% output, so the node only needs to require both signatures + a
// well-formed proof.
//
// Commitment: SHA3-256(prompt_hash || result_hash || worker_address || block_hash || nonce_le8 || model_version_le2)
// Nonce (8-byte LE, from InferencePayload) is the vLLM seed.
// ModelVersion (2-byte LE) cryptographically binds the proof to the model tier.
//
// Wire layout (fixed 258 bytes):
//
//	[0–31]    commitment_hash       [32]byte  — SHA3-256 commitment
//	[32–63]   result_hash           [32]byte  — SHA3-256 of stripped result text
//	[64–97]   worker_address        [34]byte  — P2TR scriptPubKey of fee recipient (worker 90%)
//	[98–129]  block_hash            [32]byte  — hash of block containing inference_tx
//	[130–193] signature             [64]byte  — worker BIP-340 Schnorr over commitment_hash
//	[194–257] coordinator_signature [64]byte  — coordinator BIP-340 Schnorr over commitment_hash
type InferenceResultProof struct {
	// CommitmentHash is SHA3-256(PromptHash || ResultHash || WorkerAddress || BlockHash || Nonce_LE8 || ModelVersion_LE2).
	CommitmentHash [32]byte

	// ResultHash is SHA3-256 of the inference result text after stripping
	// DeepSeek R1's <think>…</think> reasoning tokens.  SHA3-256 (not BLAKE3)
	// so the wallet/coordinator can verify it without a CGo dependency.
	ResultHash [32]byte

	// WorkerAddress is the P2TR scriptPubKey (34 bytes) of the inference worker
	// claiming its 90% of the fee.  Any address — not restricted to the block
	// winner's coinbase address.
	WorkerAddress [34]byte // P2TR scriptPubKey: OP_1 <32-byte x-only pubkey>

	// BlockHash is the hash of the block that confirmed the inference_tx.
	BlockHash chainhash.Hash

	// Signature is the worker's BIP-340 Schnorr signature over CommitmentHash.
	Signature [64]byte

	// CoordinatorSignature is the named coordinator's BIP-340 Schnorr signature
	// over CommitmentHash.  Its pubkey MUST equal the inference_tx's
	// CoordinatorPubKey.  This is the off-chain quality-check attestation the
	// covenant requires.
	CoordinatorSignature [64]byte
}

// SerialiseSize returns the fixed serialised size of InferenceResultProof.
func (p *InferenceResultProof) SerialiseSize() int {
	return 32 + 32 + 34 + chainhash.HashSize + 64 + 64 // 258 bytes
}

// Serialise writes the InferenceResultProof to w.
func (p *InferenceResultProof) Serialise(w io.Writer) error {
	var buf [258]byte
	copy(buf[0:32], p.CommitmentHash[:])
	copy(buf[32:64], p.ResultHash[:])
	copy(buf[64:98], p.WorkerAddress[:])
	copy(buf[98:130], p.BlockHash[:])
	copy(buf[130:194], p.Signature[:])
	copy(buf[194:258], p.CoordinatorSignature[:])
	_, err := w.Write(buf[:])
	return err
}

// Deserialise reads an InferenceResultProof from r.
func (p *InferenceResultProof) Deserialise(r io.Reader) error {
	var buf [258]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return err
	}
	copy(p.CommitmentHash[:], buf[0:32])
	copy(p.ResultHash[:], buf[32:64])
	copy(p.WorkerAddress[:], buf[64:98])
	copy(p.BlockHash[:], buf[98:130])
	copy(p.Signature[:], buf[130:194])
	copy(p.CoordinatorSignature[:], buf[194:258])
	return nil
}

// IsInferenceTx returns true if the transaction is an inference_tx (version 3)
// and has a well-formed OP_RETURN InferencePayload output.
func IsInferenceTx(tx *MsgTx) bool {
	return tx.Version == TxVersionInference &&
		len(tx.TxOut) >= 1 &&
		hasInferencePayloadOutput(tx)
}

// ExtractInferencePayload parses and returns the InferencePayload from an
// inference_tx's OP_RETURN output.  Returns an error if the transaction is
// not a valid inference_tx or the payload is malformed.
func ExtractInferencePayload(tx *MsgTx) (*InferencePayload, error) {
	if tx.Version != TxVersionInference {
		return nil, fmt.Errorf("not an inference_tx: version %d", tx.Version)
	}
	for _, out := range tx.TxOut {
		if payload, ok := parseInferenceOpReturn(out.PkScript); ok {
			return payload, nil
		}
	}
	return nil, fmt.Errorf("no valid InferencePayload OP_RETURN output found")
}

// hasInferencePayloadOutput checks whether any output carries a valid
// InferencePayload OP_RETURN script.
func hasInferencePayloadOutput(tx *MsgTx) bool {
	for _, out := range tx.TxOut {
		if _, ok := parseInferenceOpReturn(out.PkScript); ok {
			return true
		}
	}
	return false
}

// inferencePayloadSize is the fixed InferencePayload wire size (marketplace v1).
const inferencePayloadSize = 141

// inferenceOpReturnPrefix is OP_RETURN (0x6a) followed by a push of 141 bytes
// (OP_PUSHDATA1 0x4c 0x8d, where 0x8d = 141).
var inferenceOpReturnPrefix = []byte{0x6a, 0x4c, 0x8d}

// parseInferenceOpReturn attempts to decode an InferencePayload from a
// pkScript. Returns the payload and true on success.
func parseInferenceOpReturn(pkScript []byte) (*InferencePayload, bool) {
	// OP_RETURN (1) + OP_PUSHDATA1 (1) + length byte (1) + 141 data bytes = 144.
	if len(pkScript) != 3+inferencePayloadSize {
		return nil, false
	}
	if pkScript[0] != 0x6a { // OP_RETURN
		return nil, false
	}
	// Marketplace v1 always encodes the 141-byte payload with OP_PUSHDATA1.
	if pkScript[1] != 0x4c { // OP_PUSHDATA1
		return nil, false
	}
	if pkScript[2] != inferencePayloadSize {
		return nil, false
	}
	d := pkScript[3:]
	if len(d) != inferencePayloadSize {
		return nil, false
	}

	p := &InferencePayload{}
	if d[0] != inferencePayloadVersion {
		return nil, false
	}
	copy(p.PromptHash[:], d[1:33])
	copy(p.ResultAddress[:], d[33:97])
	p.ModelVersion = ModelVersion(binary.LittleEndian.Uint16(d[97:99]))
	p.MaxTokens = binary.LittleEndian.Uint16(d[99:101])
	p.Nonce = binary.LittleEndian.Uint64(d[101:109])
	copy(p.CoordinatorPubKey[:], d[109:141])
	return p, true
}

// ──────────────────────────────────────────────────────────────────────────────
// Inference proof transaction (version 4) — open inference market
// ──────────────────────────────────────────────────────────────────────────────

// inferenceProofSize is the fixed InferenceResultProof wire size (marketplace v1).
const inferenceProofSize = 258

// inferenceProofOpReturnPrefix is OP_RETURN (0x6a) followed by a push of 258
// bytes.  258 > 255, so OP_PUSHDATA1 cannot encode it — use OP_PUSHDATA2
// (0x4d) with a little-endian 2-byte length (258 = 0x0102 → 0x02 0x01).
var inferenceProofOpReturnPrefix = []byte{0x6a, 0x4d, 0x02, 0x01}

// IsInferenceProofTx returns true if tx is a version-4 inference proof transaction.
func IsInferenceProofTx(tx *MsgTx) bool {
	return tx.Version == TxVersionInferenceProof &&
		len(tx.TxOut) >= 1 &&
		hasInferenceProofOutput(tx)
}

// hasInferenceProofOutput reports whether any output carries a valid 258-byte
// InferenceResultProof OP_RETURN script.
func hasInferenceProofOutput(tx *MsgTx) bool {
	for _, out := range tx.TxOut {
		if _, ok := parseInferenceProofOpReturn(out.PkScript); ok {
			return true
		}
	}
	return false
}

// ExtractInferenceResultProof parses and returns the InferenceResultProof from
// a version-4 inference_proof_tx's OP_RETURN output.
func ExtractInferenceResultProof(tx *MsgTx) (*InferenceResultProof, error) {
	if tx.Version != TxVersionInferenceProof {
		return nil, fmt.Errorf("not an inference_proof_tx: version %d", tx.Version)
	}
	for _, out := range tx.TxOut {
		if proof, ok := parseInferenceProofOpReturn(out.PkScript); ok {
			return proof, nil
		}
	}
	return nil, fmt.Errorf("no valid InferenceResultProof OP_RETURN output found")
}

// parseInferenceProofOpReturn decodes an InferenceResultProof from a pkScript.
// Returns the proof and true on success.
func parseInferenceProofOpReturn(pkScript []byte) (*InferenceResultProof, bool) {
	// OP_RETURN (1) + OP_PUSHDATA2 (1) + 2-byte LE length (2) + 258 data = 262.
	if len(pkScript) != 4+inferenceProofSize {
		return nil, false
	}
	if pkScript[0] != 0x6a { // OP_RETURN
		return nil, false
	}
	if pkScript[1] != 0x4d { // OP_PUSHDATA2
		return nil, false
	}
	if int(binary.LittleEndian.Uint16(pkScript[2:4])) != inferenceProofSize {
		return nil, false
	}
	data := pkScript[4:]
	if len(data) != inferenceProofSize {
		return nil, false
	}

	p := &InferenceResultProof{}
	copy(p.CommitmentHash[:], data[0:32])
	copy(p.ResultHash[:], data[32:64])
	copy(p.WorkerAddress[:], data[64:98])
	copy(p.BlockHash[:], data[98:130])
	copy(p.Signature[:], data[130:194])
	copy(p.CoordinatorSignature[:], data[194:258])
	return p, true
}
