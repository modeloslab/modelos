// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txsizes

import (
	"github.com/modelos/modelos/node/blockchain"
	"github.com/modelos/modelos/node/wire"
)

// Taproot-only transaction size constants.
const (

	// P2TRPkScriptSize is the size of a transaction output script that
	// pays to a taproot pubkey. It is calculated as:
	//
	//   - OP_1
	//   - OP_DATA_32
	//   - 32 bytes pubkey
	P2TRPkScriptSize = 1 + 1 + 32

	// P2TROutputSize is the serialize size of a transaction output with a
	// P2TR output script. It is calculated as:
	//
	//   - 8 bytes output value
	//   - 1 byte compact int encoding value 34
	//   - 34 bytes P2TR output script
	P2TROutputSize = 8 + 1 + P2TRPkScriptSize

	// RedeemP2TRScriptSize is the size of a transaction input script
	// that spends a pay-to-taproot hash (P2TR). The redeem
	// script for P2TR spends MUST be empty.
	RedeemP2TRScriptSize = 0

	// RedeemP2TRInputSize is the worst case size of a transaction
	// input redeeming a P2TR output. It is calculated as:
	//
	//   - 32 bytes previous tx
	//   - 4 bytes output index
	//   - 1 byte encoding empty redeem script
	//   - 0 bytes redeem script
	//   - 4 bytes sequence
	RedeemP2TRInputSize = 32 + 4 + 1 + RedeemP2TRScriptSize + 4

	// RedeemP2TRInputWitnessWeight is the worst case weight of
	// a witness for spending P2TR outputs. It is calculated as:
	//
	//   - 1 wu compact int encoding value 1 (number of items)
	//   - 1 wu compact int encoding value 65
	//   - 64 wu BIP-340 schnorr signature + 1 wu sighash
	RedeemP2TRInputWitnessWeight = 1 + 1 + 65
)

// P2MR (BIP 360) transaction size constants.
const (
	// P2MRPkScriptSize is the size of a P2MR output script:
	// OP_2 (1) + OP_DATA_32 (1) + 32-byte merkle root = 34 bytes.
	P2MRPkScriptSize = 1 + 1 + 32

	// P2MROutputSize is the size of a serialized P2MR output.
	P2MROutputSize = 8 + 1 + P2MRPkScriptSize
)

// SumOutputSerializeSizes sums up the serialized size of the supplied outputs.
func SumOutputSerializeSizes(outputs []*wire.TxOut) (serializeSize int) {
	for _, txOut := range outputs {
		serializeSize += txOut.SerializeSize()
	}
	return serializeSize
}

// EstimateSerializeSize returns a worst case serialize size estimate for a
// signed Taproot transaction that spends inputCount number of P2TR outputs
// and contains each transaction output from txOuts. The estimated size is
// incremented for an additional P2TR change output if addChangeOutput is true.
func EstimateSerializeSize(inputCount int, txOuts []*wire.TxOut, addChangeOutput bool) int {
	changeSize := 0
	outputCount := len(txOuts)
	if addChangeOutput {
		changeSize = P2TROutputSize
		outputCount++
	}

	// 8 additional bytes are for version and locktime
	return 8 + wire.VarIntSerializeSize(uint64(inputCount)) +
		wire.VarIntSerializeSize(uint64(outputCount)) +
		inputCount*RedeemP2TRInputSize +
		SumOutputSerializeSizes(txOuts) +
		changeSize
}

// EstimateVirtualSize returns a worst case virtual size estimate for a
// signed Taproot-only transaction. For backward compatibility, this function
// keeps the old signature but ignores non-Taproot parameters (numP2PKHIns,
// numP2WPKHIns, numNestedP2WPKHIns) and only uses numP2TRIns.
func EstimateVirtualSize(numP2PKHIns, numP2TRIns, numP2WPKHIns, numNestedP2WPKHIns int,
	txOuts []*wire.TxOut, changeScriptSize int) int {

	// For Taproot-only wallet, ignore legacy parameters and only use P2TR inputs
	totalInputs := numP2TRIns
	outputCount := len(txOuts)

	changeOutputSize := 0
	if changeScriptSize > 0 {
		changeOutputSize = 8 +
			wire.VarIntSerializeSize(uint64(changeScriptSize)) +
			changeScriptSize
		outputCount++
	}

	// Version 4 bytes + LockTime 4 bytes + Serialized var int size for the
	// number of transaction inputs and outputs + P2TR input sizes +
	// the size of the serialized outputs and change.
	baseSize := 8 +
		wire.VarIntSerializeSize(uint64(totalInputs)) +
		wire.VarIntSerializeSize(uint64(outputCount)) +
		totalInputs*RedeemP2TRInputSize +
		SumOutputSerializeSizes(txOuts) +
		changeOutputSize

	// Taproot transactions always have witness data when there are inputs.
	witnessWeight := 0
	if totalInputs > 0 {
		// Additional 2 weight units for segwit marker + flag.
		witnessWeight = 2 +
			wire.VarIntSerializeSize(uint64(totalInputs)) +
			totalInputs*RedeemP2TRInputWitnessWeight
	}

	// We add 3 to the witness weight to make sure the result is
	// always rounded up.
	return int(blockchain.CalcVsize(baseSize, witnessWeight))
}

// GetMinInputVirtualSize returns the minimum number of vbytes that this input
// adds to a transaction.
func GetMinInputVirtualSize(pkScript []byte) int {
	// Only P2TR scripts are supported in Taproot-only wallets
	baseSize := RedeemP2TRInputSize
	witnessWeight := RedeemP2TRInputWitnessWeight

	return int(blockchain.CalcVsize(baseSize, witnessWeight))
}
