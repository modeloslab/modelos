// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"fmt"

	"github.com/modelos/modelos/node/btcec"
	"github.com/modelos/modelos/node/btcec/schnorr"
	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/modelos/modelos/node/wire"
)

// signatureVerifier is an abstract interface that allows the op code execution
// to abstract over the _type_ of signature validation being executed.
// Pearl supports taproot key-spend and tapscript signature verification.
type signatureVerifier interface {
	// Verify returns whether or not the signature is valid for the
	// given context.
	Verify() verifyResult
}

type verifyResult struct {
	sigValid bool
}

// taprootSigVerifier verifies signatures according to the segwit v1 rules,
// which are described in BIP 341.
type taprootSigVerifier struct {
	pubKey  *btcec.PublicKey
	pkBytes []byte

	fullSigBytes []byte
	sig          *schnorr.Signature

	hashType SigHashType

	sigCache  *SigCache
	hashCache *TxSigHashes

	tx *wire.MsgTx

	inputIndex int

	annex []byte

	prevOuts PrevOutputFetcher
}

// parseTaprootSigAndPubKey attempts to parse the public key and signature for
// a taproot spend that may be a keyspend or script path spend. This function
// returns an error if the pubkey is invalid, or the sig is.
func parseTaprootSigAndPubKey(pkBytes, rawSig []byte,
) (*btcec.PublicKey, *schnorr.Signature, SigHashType, error) {

	pubKey, err := schnorr.ParsePubKey(pkBytes)
	if err != nil {
		return nil, nil, 0, err
	}

	// Parse the signature, which may or may not be appended with the
	// desired sighash flag.
	var (
		sig         *schnorr.Signature
		sigHashType SigHashType
	)
	switch {
	// If the signature is exactly 64 bytes, then we're using the
	// implicit SIGHASH_DEFAULT sighash type.
	case len(rawSig) == schnorr.SignatureSize:
		sig, err = schnorr.ParseSignature(rawSig)
		if err != nil {
			return nil, nil, 0, err
		}
		sigHashType = SigHashDefault

	// If the signature is 65 bytes with a non-zero trailing byte,
	// extract the explicit sighash type from the last byte.
	case len(rawSig) == schnorr.SignatureSize+1 && rawSig[64] != 0:
		sigHashType = SigHashType(rawSig[schnorr.SignatureSize])

		rawSig = rawSig[:schnorr.SignatureSize]
		sig, err = schnorr.ParseSignature(rawSig)
		if err != nil {
			return nil, nil, 0, err
		}

	default:
		str := fmt.Sprintf("invalid sig len: %v", len(rawSig))
		return nil, nil, 0, scriptError(ErrInvalidTaprootSigLen, str)
	}

	return pubKey, sig, sigHashType, nil
}

// newTaprootSigVerifier returns a new instance of a taproot sig verifier given
// the necessary contextual information.
func newTaprootSigVerifier(pkBytes []byte, fullSigBytes []byte,
	tx *wire.MsgTx, inputIndex int, prevOuts PrevOutputFetcher,
	sigCache *SigCache, hashCache *TxSigHashes,
	annex []byte) (*taprootSigVerifier, error) {

	pubKey, sig, sigHashType, err := parseTaprootSigAndPubKey(
		pkBytes, fullSigBytes,
	)
	if err != nil {
		return nil, err
	}

	return &taprootSigVerifier{
		pubKey:       pubKey,
		pkBytes:      pkBytes,
		sig:          sig,
		fullSigBytes: fullSigBytes,
		hashType:     sigHashType,
		tx:           tx,
		inputIndex:   inputIndex,
		prevOuts:     prevOuts,
		sigCache:     sigCache,
		hashCache:    hashCache,
		annex:        annex,
	}, nil
}

// verifySig attempts to verify a BIP 340 signature using the internal public
// key and signature, and the passed sigHash as the message digest.
func (t *taprootSigVerifier) verifySig(sigHash []byte) bool {
	// Check if this signature is already in the cache and valid.
	cacheKey, _ := chainhash.NewHash(sigHash)
	if t.sigCache != nil {
		if t.sigCache.Exists(*cacheKey, t.fullSigBytes, t.pkBytes) {
			return true
		}
	}

	// Perform full verification, adding the entry to the cache if valid.
	sigValid := t.sig.Verify(sigHash, t.pubKey)
	if sigValid {
		if t.sigCache != nil {
			t.sigCache.Add(*cacheKey, t.fullSigBytes, t.pkBytes)
		}

		return true
	}

	return false
}

// Verify computes the taproot sighash for this input and verifies the
// BIP 340 Schnorr signature against it.
func (t *taprootSigVerifier) Verify() verifyResult {
	var opts []TaprootSigHashOption
	if t.annex != nil {
		opts = append(opts, WithAnnex(t.annex))
	}

	// Compute the sighash based on the input and tx information.
	sigHash, err := calcTaprootSignatureHashRaw(
		t.hashCache, t.hashType, t.tx, t.inputIndex, t.prevOuts,
		opts...,
	)
	if err != nil {
		return verifyResult{}
	}

	return verifyResult{
		sigValid: t.verifySig(sigHash),
	}
}

var _ signatureVerifier = (*taprootSigVerifier)(nil)

// baseTapscriptSigVerifier verifies a signature for an input spending a
// tapscript leaf from the previous output.
type baseTapscriptSigVerifier struct {
	*taprootSigVerifier

	vm *Engine
}

// newBaseTapscriptSigVerifier returns a new sig verifier for tapscript input
// spends. If the public key or signature aren't correctly formatted, an error
// is returned.
func newBaseTapscriptSigVerifier(pkBytes, rawSig []byte,
	vm *Engine) (*baseTapscriptSigVerifier, error) {

	switch len(pkBytes) {
	// Empty public keys cause immediate failure.
	case 0:
		return nil, scriptError(ErrTaprootPubkeyIsEmpty, "")

	// 32-byte x-only public key as expected by BIP 340 Schnorr signatures.
	case 32:
		baseTaprootVerifier, err := newTaprootSigVerifier(
			pkBytes, rawSig, &vm.tx, vm.txIdx, vm.prevOutFetcher,
			vm.sigCache, vm.hashCache, vm.tapscriptCtx.annex,
		)
		if err != nil {
			return nil, err
		}

		return &baseTapscriptSigVerifier{
			taprootSigVerifier: baseTaprootVerifier,
			vm:                 vm,
		}, nil

	// Unknown public key lengths are rejected. Pearl does not use
	// Bitcoin's pass-through where unknown key types silently succeed.
	default:
		str := fmt.Sprintf("unsupported public key length: %d", len(pkBytes))
		return nil, scriptError(ErrDiscourageUpgradeablePubKeyType, str)
	}
}

// Verify returns whether or not the signature verifier context deems the
// signature to be valid for the given context.
func (b *baseTapscriptSigVerifier) Verify() verifyResult {
	sigHash, err := b.vm.CalcTapscriptSigHash(
		b.hashCache, b.hashType, b.tx, b.inputIndex, b.prevOuts,
	)
	if err != nil {
		// TODO(roasbeef): propagate the error here?
		return verifyResult{}
	}

	return verifyResult{
		sigValid: b.verifySig(sigHash),
	}
}

// A compile-time assertion to ensure baseTapscriptSigVerifier implements the
// signatureVerifier interface.
var _ signatureVerifier = (*baseTapscriptSigVerifier)(nil)
