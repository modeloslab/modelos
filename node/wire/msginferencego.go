// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// ──────────────────────────────────────────────────────────────────────────────
// inferencego_tx (transaction version 5).
//
// Carries an InferenceGoPayload in an OP_RETURN output, signed by the authorised
// key configured in consensus (see blockchain/inferencego.go for the validation
// rules and the per-network keys).
//
// The design is stateless — the authorised key is a consensus constant — so
// there is no chain state to keep and nothing to unwind on a reorg. Validity
// depends only on the transaction itself and the consensus constants.
// ──────────────────────────────────────────────────────────────────────────────

// inferenceGoPayloadVersion is the payload serialisation version.
const inferenceGoPayloadVersion = byte(1)

// InferenceGoPayloadSize is the fixed InferenceGoPayload wire size.
//
//	version(1) + amount(8) + destination(32) + signature(64) = 105 bytes.
const InferenceGoPayloadSize = 1 + 8 + 32 + 64

// inferenceGoOpReturnPrefix is OP_RETURN (0x6a) followed by a push of 105 bytes
// (OP_PUSHDATA1 0x4c 0x69, where 0x69 = 105).
var inferenceGoOpReturnPrefix = []byte{0x6a, 0x4c, byte(InferenceGoPayloadSize)}

// InferenceGoPayload is the OP_RETURN payload of an inferencego_tx.
//
// Wire layout (little-endian):
//
//	[0]        version      byte      — payload serialisation version (1)
//	[1–8]      amount       uint64 LE — amount in grains
//	[9–40]     destination  [32]byte  — taproot OUTPUT key receiving the issue
//	                                    (the witness program of a bech32m address);
//	                                    all-zero means "pay the authorised key"
//	[41–104]   signature    [64]byte  — BIP-340 signature by the authorised key
type InferenceGoPayload struct {
	// Amount is the payload amount in grains. Consensus requires an output
	// paying at least this to Destination.
	Amount uint64

	// Destination is the 32-byte taproot OUTPUT key that receives the payment —
	// exactly the witness program a bech32m address decodes to, NOT the
	// untweaked internal key.
	//
	// This distinction matters: a wallet watches the BIP-341 tweaked output key,
	// so paying the untweaked internal key would land the funds on an output the
	// wallet never sees and no standard taproot signer will spend.
	//
	// All-zero means "pay the authorised key itself", in which case consensus
	// derives the authority's own tweaked output key.
	Destination [32]byte

	// Signature is the authorised key's BIP-340 signature over the payload
	// sighash (see blockchain.InferenceGoSigHash). It is excluded from the
	// signed message, and the message binds the transaction's first input so
	// the same signed payload cannot be replayed in another transaction.
	Signature [64]byte
}

// HasDestination reports whether the payload names an explicit destination.
// A zero Destination means "pay the authorised key".
func (p *InferenceGoPayload) HasDestination() bool {
	return p.Destination != [32]byte{}
}

// SerialiseSize returns the fixed serialised size of InferenceGoPayload.
func (p *InferenceGoPayload) SerialiseSize() int {
	return InferenceGoPayloadSize
}

// Serialise writes the InferenceGoPayload to w in the canonical wire format.
func (p *InferenceGoPayload) Serialise(w io.Writer) error {
	var buf [InferenceGoPayloadSize]byte
	buf[0] = inferenceGoPayloadVersion
	binary.LittleEndian.PutUint64(buf[1:9], p.Amount)
	copy(buf[9:41], p.Destination[:])
	copy(buf[41:105], p.Signature[:])
	_, err := w.Write(buf[:])
	return err
}

// Deserialise reads an InferenceGoPayload from r.
func (p *InferenceGoPayload) Deserialise(r io.Reader) error {
	var buf [InferenceGoPayloadSize]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return err
	}
	if buf[0] != inferenceGoPayloadVersion {
		return fmt.Errorf("unsupported InferenceGoPayload version %d", buf[0])
	}
	p.Amount = binary.LittleEndian.Uint64(buf[1:9])
	copy(p.Destination[:], buf[9:41])
	copy(p.Signature[:], buf[41:105])
	return nil
}

// SignedBytes returns the payload fields covered by the authority signature —
// everything except the signature itself. Consensus hashes these together with
// the transaction's first input to produce the issuance sighash.
func (p *InferenceGoPayload) SignedBytes() []byte {
	buf := make([]byte, 0, 1+8+32)
	buf = append(buf, inferenceGoPayloadVersion)
	var a [8]byte
	binary.LittleEndian.PutUint64(a[:], p.Amount)
	buf = append(buf, a[:]...)
	buf = append(buf, p.Destination[:]...)
	return buf
}

// IsInferenceGoTx returns true if the transaction is an inferencego_tx
// (version 5) and has a well-formed OP_RETURN InferenceGoPayload output.
func IsInferenceGoTx(tx *MsgTx) bool {
	return tx.Version == TxVersionInferenceGo &&
		len(tx.TxOut) >= 1 &&
		hasInferenceGoPayloadOutput(tx)
}

// ExtractInferenceGoPayload parses and returns the InferenceGoPayload from an
// inferencego_tx's OP_RETURN output. Returns an error if the transaction is not
// a valid inferencego_tx or the payload is malformed.
func ExtractInferenceGoPayload(tx *MsgTx) (*InferenceGoPayload, error) {
	if tx.Version != TxVersionInferenceGo {
		return nil, fmt.Errorf("not an inferencego_tx: version %d", tx.Version)
	}
	for _, out := range tx.TxOut {
		if payload, ok := parseInferenceGoOpReturn(out.PkScript); ok {
			return payload, nil
		}
	}
	return nil, fmt.Errorf("no valid InferenceGoPayload OP_RETURN output found")
}

// hasInferenceGoPayloadOutput checks whether any output carries a valid
// InferenceGoPayload OP_RETURN script.
func hasInferenceGoPayloadOutput(tx *MsgTx) bool {
	for _, out := range tx.TxOut {
		if _, ok := parseInferenceGoOpReturn(out.PkScript); ok {
			return true
		}
	}
	return false
}

// CountInferenceGoPayloadOutputs returns how many outputs carry a well-formed
// payload. Consensus rejects a transaction with more than one: two payloads
// would make "which one authorises the issue" ambiguous.
func CountInferenceGoPayloadOutputs(tx *MsgTx) int {
	n := 0
	for _, out := range tx.TxOut {
		if _, ok := parseInferenceGoOpReturn(out.PkScript); ok {
			n++
		}
	}
	return n
}

// parseInferenceGoOpReturn attempts to decode an InferenceGoPayload from a
// pkScript. Returns (payload, true) only for the exact canonical framing.
func parseInferenceGoOpReturn(pkScript []byte) (*InferenceGoPayload, bool) {
	if len(pkScript) != len(inferenceGoOpReturnPrefix)+InferenceGoPayloadSize {
		return nil, false
	}
	if !bytes.Equal(pkScript[:len(inferenceGoOpReturnPrefix)], inferenceGoOpReturnPrefix) {
		return nil, false
	}
	p := &InferenceGoPayload{}
	if err := p.Deserialise(bytes.NewReader(pkScript[len(inferenceGoOpReturnPrefix):])); err != nil {
		return nil, false
	}
	return p, true
}

// BuildInferenceGoOpReturn serialises a payload into its canonical OP_RETURN
// pkScript.
func BuildInferenceGoOpReturn(p *InferenceGoPayload) ([]byte, error) {
	var body bytes.Buffer
	if err := p.Serialise(&body); err != nil {
		return nil, err
	}
	script := make([]byte, 0, len(inferenceGoOpReturnPrefix)+InferenceGoPayloadSize)
	script = append(script, inferenceGoOpReturnPrefix...)
	script = append(script, body.Bytes()...)
	return script, nil
}

// InferenceGoP2TRScript returns the P2TR scriptPubKey (OP_1 <32-byte output key>)
// for a taproot OUTPUT key. Pass the witness program of an address, not an
// untweaked internal key.
func InferenceGoP2TRScript(outputKey [32]byte) []byte {
	script := make([]byte, 0, 34)
	script = append(script, 0x51, 0x20) // OP_1, push 32
	script = append(script, outputKey[:]...)
	return script
}
