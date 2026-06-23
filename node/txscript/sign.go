// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"github.com/modelos/modelos/node/btcec"
	"github.com/modelos/modelos/node/btcec/schnorr"
	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/wire"
)

// RawTxInTaprootSignature returns a valid schnorr signature required to
// perform a taproot key-spend of the specified input. If SigHashDefault was
// specified, then the returned signature is 64-byte in length, as it omits the
// additional byte to denote the sighash type.
func RawTxInTaprootSignature(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, pkScript []byte, tapScriptRootHash []byte, hashType SigHashType,
	key *btcec.PrivateKey) ([]byte, error) {

	// First, we'll start by compute the top-level taproot sighash.
	sigHash, err := calcTaprootSignatureHashRaw(
		sigHashes, hashType, tx, idx,
		NewCannedPrevOutputFetcher(pkScript, amt),
	)
	if err != nil {
		return nil, err
	}

	// Before we sign the sighash, we'll need to apply the taptweak to the
	// private key based on the tapScriptRootHash.
	privKeyTweak := TweakTaprootPrivKey(*key, tapScriptRootHash)

	// With the sighash constructed, we can sign it with the specified
	// private key.
	signature, err := schnorr.Sign(privKeyTweak, sigHash)
	if err != nil {
		return nil, err
	}

	sig := signature.Serialize()

	// If this is sighash default, then we can just return the signature
	// directly.
	if hashType == SigHashDefault {
		return sig, nil
	}

	// Otherwise, append the sighash type to the final sig.
	return append(sig, byte(hashType)), nil
}

// TaprootWitnessSignature returns a valid witness stack that can be used to
// spend the key-spend path of a taproot input as specified in BIP 342 and BIP
// 86. This method assumes that the public key included in pkScript was
// generated using ComputeTaprootKeyNoScript that commits to a fake root
// tapscript hash. If not, then RawTxInTaprootSignature should be used with the
// actual committed contents.
//
// TODO(roasbeef): add support for annex even tho it's non-standard?
func TaprootWitnessSignature(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, pkScript []byte, hashType SigHashType,
	key *btcec.PrivateKey) (wire.TxWitness, error) {

	// As we're assuming this was a BIP 86 key, we use an empty root hash
	// which means output key commits to just the public key.
	fakeTapscriptRootHash := []byte{}

	sig, err := RawTxInTaprootSignature(
		tx, sigHashes, idx, amt, pkScript, fakeTapscriptRootHash,
		hashType, key,
	)
	if err != nil {
		return nil, err
	}

	// The witness script to spend a taproot input using the key-spend path
	// is just the signature itself, given the public key is
	// embedded in the previous output script.
	return wire.TxWitness{sig}, nil
}

// RawTxInTapscriptSignature computes a raw schnorr signature for a signature
// generated from a tapscript leaf. This differs from the
// RawTxInTaprootSignature which is used to generate signatures for top-level
// taproot key spends.
//
// TODO(roasbeef): actually add code-sep to interface? not really used
// anywhere....
func RawTxInTapscriptSignature(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, pkScript []byte, tapLeaf TapLeaf, hashType SigHashType,
	privKey *btcec.PrivateKey) ([]byte, error) {

	// First, we'll start by compute the top-level taproot sighash.
	tapLeafHash := tapLeaf.TapHash()
	sigHash, err := calcTaprootSignatureHashRaw(
		sigHashes, hashType, tx, idx,
		NewCannedPrevOutputFetcher(pkScript, amt),
		WithBaseTapscriptVersion(blankCodeSepValue, tapLeafHash[:]),
	)
	if err != nil {
		return nil, err
	}

	// With the sighash constructed, we can sign it with the specified
	// private key.
	signature, err := schnorr.Sign(privKey, sigHash)
	if err != nil {
		return nil, err
	}

	// Finally, append the sighash type to the final sig if it's not the
	// default sighash value (in which case appending it is disallowed).
	if hashType != SigHashDefault {
		return append(signature.Serialize(), byte(hashType)), nil
	}

	// The default sighash case where we'll return _just_ the signature.
	return signature.Serialize(), nil
}

// KeyDB is an interface type provided to SignTxOutput, it encapsulates
// any user state required to get the private keys for an address.
type KeyDB interface {
	GetKey(btcutil.Address) (*btcec.PrivateKey, bool, error)
}

// KeyClosure implements KeyDB with a closure.
type KeyClosure func(btcutil.Address) (*btcec.PrivateKey, bool, error)

// GetKey implements KeyDB by returning the result of calling the closure.
func (kc KeyClosure) GetKey(address btcutil.Address) (*btcec.PrivateKey, bool, error) {
	return kc(address)
}

// ScriptDB is an interface type provided to SignTxOutput, it encapsulates any
// user state required to get the scripts for an pay-to-script-hash address.
type ScriptDB interface {
	GetScript(btcutil.Address) ([]byte, error)
}

// ScriptClosure implements ScriptDB with a closure.
type ScriptClosure func(btcutil.Address) ([]byte, error)

// GetScript implements ScriptDB by returning the result of calling the closure.
func (sc ScriptClosure) GetScript(address btcutil.Address) ([]byte, error) {
	return sc(address)
}
