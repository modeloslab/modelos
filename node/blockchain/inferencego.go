// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"

	"github.com/modelos/modelos/node/btcec/schnorr"
	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/txscript"
	"github.com/modelos/modelos/node/wire"
	"golang.org/x/crypto/sha3"
)

// ──────────────────────────────────────────────────────────────────────────────
// inferencego_tx validation rules (transaction version 5).
//
// A version-5 transaction is accepted when:
//
//   - the fork is active at this height,
//   - it carries exactly one well-formed InferenceGoPayload OP_RETURN,
//   - the payload's BIP-340 signature verifies against the authorised x-only key
//     configured below for this network,
//   - the payload's destination is paid at least the payload amount, and
//   - it spends at least one input.
//
// The rules are stateless: the authorised key is a consensus constant, so there
// is no chain state to persist and nothing to unwind on a reorg — validity
// depends only on the transaction and the consensus constants, as with every
// other rule in this package.
//
// The input requirement is what prevents replay: the signature commits to the
// transaction's first input outpoint, so a signed payload lifted out of one
// transaction cannot be re-broadcast inside another.
// ──────────────────────────────────────────────────────────────────────────────

// inferenceGoSigHashTag domain-separates the issuance signature so an
// authorisation can never be replayed as any other kind of signature.
var inferenceGoSigHashTag = []byte("MDL-INFERENCEGO-ISSUE")

// inferenceGoForkHeight is the height at which inferencego_tx becomes valid.
// The fork is DISABLED while this is 0, matching Pearl's convention for fork
// gates (IsSaltedSeedForkActive et al) and auxPowActivationHeight in auxpow.go.
//
// MUST be set before this feature ships.
// Declared as a var, not a const, purely so tests can exercise the gate without
// mining 36k blocks. Treat it as a consensus constant.
var inferenceGoForkHeight = int32(36438)

// inferenceGoForkActive reports whether issuance is valid at the given height.
func inferenceGoForkActive(height int32) bool {
	return inferenceGoForkHeight != 0 && height >= inferenceGoForkHeight
}

// The authorised x-only key per network. These are PUBLIC keys; the private
// halves are held offline and must never be reused.
//
// Per-network keys exist so regtest and testnet can sign in tests without the
// mainnet private key leaving cold storage.
//
// A zero key fails schnorr.ParsePubKey, so nothing validates until one is set —
// the fork height gate above is the primary switch, this is the backstop.
var (
	// Mainnet authority. There is no rotation path in this design, so the private
	// half must be held offline and never reused.
	inferenceGoMainnetAuthority = [32]byte{
		0x85, 0x67, 0xc2, 0xf6, 0x52, 0xb4, 0xe2, 0xa4, 0xb3, 0xcd, 0xff, 0xf6, 0xc4, 0xde, 0x5b, 0x84,
		0x48, 0xcb, 0x0a, 0xb2, 0x67, 0xc6, 0x94, 0x97, 0x80, 0x77, 0x34, 0x40, 0x90, 0x65, 0xfb, 0xd7,
	}

	// Test-network keys. Private halves are held locally, outside this repository.
	inferenceGoTestnetAuthority = [32]byte{
		0x63, 0x35, 0xcf, 0x71, 0x90, 0x87, 0x68, 0x2b, 0x0d, 0x32, 0x81, 0x41, 0x64, 0x41, 0x33, 0x5c,
		0x18, 0x12, 0x38, 0x07, 0xa6, 0x16, 0x6f, 0x5d, 0xa0, 0x74, 0x88, 0x8e, 0x59, 0x10, 0x46, 0x65,
	}
	inferenceGoRegtestAuthority = [32]byte{
		0x8e, 0x9a, 0x4e, 0x18, 0xf9, 0x23, 0xf4, 0xc4, 0xec, 0x41, 0xb5, 0x46, 0xf4, 0x6f, 0x60, 0x5b,
		0x76, 0x5d, 0xf3, 0x6a, 0xfa, 0x86, 0x00, 0x79, 0x3c, 0xa8, 0xbf, 0xbe, 0x7d, 0x5a, 0x0e, 0x4b,
	}
)

// inferenceGoAuthority returns the x-only key authorised to issue on this
// network.
func inferenceGoAuthority(params *chaincfg.Params) [32]byte {
	switch params.Net {
	case wire.MainNet:
		return inferenceGoMainnetAuthority
	case wire.TestNet, wire.TestNet2:
		return inferenceGoTestnetAuthority
	default:
		return inferenceGoRegtestAuthority
	}
}

// InferenceGoSigHash returns the message the authorised key signs to authorise a
// specific issuance.
//
// It commits to the payload fields (version, amount, destination) AND to the
// transaction's first input outpoint. Binding the outpoint is what makes the
// authorisation single-use: the same signed payload cannot be lifted into a
// different transaction, because that transaction would spend a different input
// and therefore hash to a different message.
func InferenceGoSigHash(p *wire.InferenceGoPayload, firstInput *wire.OutPoint) [32]byte {
	h := sha3.New256()
	h.Write(inferenceGoSigHashTag)
	h.Write(p.SignedBytes())
	h.Write(firstInput.Hash[:])
	var idx [4]byte
	idx[0] = byte(firstInput.Index)
	idx[1] = byte(firstInput.Index >> 8)
	idx[2] = byte(firstInput.Index >> 16)
	idx[3] = byte(firstInput.Index >> 24)
	h.Write(idx[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// taprootOutputKey returns the BIP-341 key-path-only output key for an x-only
// internal key — i.e. the 32-byte witness program a bech32m address encodes.
func taprootOutputKey(internal [32]byte) ([32]byte, error) {
	var out [32]byte
	pub, err := schnorr.ParsePubKey(internal[:])
	if err != nil {
		return out, err
	}
	tweaked := txscript.ComputeTaprootKeyNoScript(pub)
	copy(out[:], schnorr.SerializePubKey(tweaked))
	return out, nil
}

// ValidateInferenceGoTx checks an inferencego_tx against the authorised key and
// returns the number of grains it is permitted to create from nothing.
//
// `authority` is the network's authorised x-only key. The caller has already
// established that the fork is active.
func ValidateInferenceGoTx(msgTx *wire.MsgTx, authority [32]byte) (int64, error) {
	if wire.CountInferenceGoPayloadOutputs(msgTx) != 1 {
		return 0, ruleError(ErrBadTxOutValue,
			"inferencego: a transaction must carry exactly one issuance payload")
	}
	payload, err := wire.ExtractInferenceGoPayload(msgTx)
	if err != nil {
		return 0, ruleError(ErrBadTxOutValue, "inferencego: "+err.Error())
	}

	// An ordinary input is required: it pays the fee and, more importantly, it
	// is what the signature binds to, making the authorisation single-use.
	if len(msgTx.TxIn) == 0 {
		return 0, ruleError(ErrBadTxOutValue,
			"inferencego: an issuance must spend at least one input")
	}

	// Amount must fit a signed 64-bit grain total and stay within the money range.
	if payload.Amount > uint64(btcutil.MaxGrain) {
		return 0, ruleError(ErrBadTxOutValue,
			"inferencego: issue amount exceeds the maximum allowed value")
	}
	amount := int64(payload.Amount)
	if amount <= 0 {
		return 0, ruleError(ErrBadTxOutValue, "inferencego: issue amount must be positive")
	}

	// The authorised key must have signed THIS issuance bound to THIS input.
	authKey, err := schnorr.ParsePubKey(authority[:])
	if err != nil {
		return 0, ruleError(ErrBadTxOutValue, "inferencego: invalid authorised x-only pubkey")
	}
	sig, err := schnorr.ParseSignature(payload.Signature[:])
	if err != nil {
		return 0, ruleError(ErrBadTxOutValue, "inferencego: cannot parse authority signature")
	}
	firstInput := msgTx.TxIn[0].PreviousOutPoint
	msg := InferenceGoSigHash(payload, &firstInput)
	if !sig.Verify(msg[:], authKey) {
		return 0, ruleError(ErrBadTxOutValue,
			"inferencego: signature does not verify against the authorised key")
	}

	// Destination: the payload's output key, or the AUTHORITY'S OWN TAPROOT OUTPUT
	// KEY when none is named.
	//
	// The authority constant is the untweaked x-only signing key. A wallet watches
	// the BIP-341 tweaked output key, so paying `authority` directly would put the
	// funds on an output the wallet never sees and no standard taproot signer will
	// spend. Derive the tweak so the default destination is an ordinary,
	// wallet-visible address.
	dest := payload.Destination
	if !payload.HasDestination() {
		dest, err = taprootOutputKey(authority)
		if err != nil {
			return 0, ruleError(ErrBadTxOutValue, "inferencego: "+err.Error())
		}
	}
	want := wire.InferenceGoP2TRScript(dest)

	// AT LEAST `amount` must reach the destination, summed across outputs so an
	// issue may be split.
	//
	// "At least" rather than "exactly" for two reasons. First, the total value a
	// transaction may create is ALREADY capped at `amount` by the conservation
	// rule in CheckTransactionInputsWithIssuance, so paying the destination more
	// than `amount` cannot mint anything extra — the surplus must come from the
	// transaction's own inputs. Second, when no destination is named the payee IS
	// the authority key, and an exact rule would make ordinary change back to that
	// key indistinguishable from the issuance payment, so no issuance could ever
	// carry change.
	//
	// What this still guarantees is the property that matters: the issued amount
	// cannot be created and then quietly routed somewhere other than the
	// destination the authority signed for.
	var paid int64
	for _, txOut := range msgTx.TxOut {
		if bytes.Equal(txOut.PkScript, want) {
			paid += txOut.Value
		}
	}
	if paid < amount {
		return 0, ruleError(ErrBadTxOutValue,
			"inferencego: outputs to the destination must total at least the issued amount")
	}

	return amount, nil
}

// CheckInferenceGoIssuance validates a version-5 transaction at the given height
// and returns its permitted amount (0 for any other transaction).
//
// Exported because every admission path must agree — block connect, mempool and
// block-template assembly. A transaction accepted by one and rejected by another
// could never be relayed or mined by the normal path.
func CheckInferenceGoIssuance(tx *btcutil.Tx, height int32, params *chaincfg.Params) (int64, error) {
	msgTx := tx.MsgTx()
	if msgTx.Version != wire.TxVersionInferenceGo {
		return 0, nil
	}
	if !inferenceGoForkActive(height) {
		return 0, ruleError(ErrBadTxOutValue,
			"inferencego: issuance is not active at this height")
	}
	// The coinbase has its own rule and must not also carry a version-5 payload.
	if IsCoinBase(tx) {
		return 0, ruleError(ErrBadTxOutValue,
			"inferencego: the coinbase may not carry an issuance")
	}
	return ValidateInferenceGoTx(msgTx, inferenceGoAuthority(params))
}

// checkInferenceGoTx is the block-connect wrapper around CheckInferenceGoIssuance.
func (b *BlockChain) checkInferenceGoTx(tx *btcutil.Tx, node *blockNode) (int64, error) {
	return CheckInferenceGoIssuance(tx, node.height, b.chainParams)
}
