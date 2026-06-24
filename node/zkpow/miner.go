//go:build zkpow

// Copyright (c) 2025-2026 The Pearl Research Labs developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package zkpow provides ZK-POW mining and proof generation functionality.
package zkpow

/*
#include "../../zk-pow/bindings/go/zk_pow_ffi.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/modelos/modelos/node/wire"
)

const (
	DefaultNBits     = 0x1E01FFFF
	DefaultM         = 256
	DefaultN         = 512
	DefaultNoiseRank = 32
	DefaultMMAType   = 0
)

// ================================================================================
// MINER (Rust FFI)
// ================================================================================

// miningConfigSize matches MINING_CONFIG_SERIALIZED_SIZE in the FFI header.
const miningConfigSize = 52

// defaultMiningConfig is the serialized MiningConfiguration passed to the Rust FFI.
// Corresponds to: common_dim=1024, rank=32, mma_type=Int7xInt7ToInt32,
// rows_pattern=[0,8,64,72], cols_pattern=[0,1,8,9,32,33,40,41]
var defaultMiningConfig = [miningConfigSize]byte{
	0x00, 0x04, 0x00, 0x00, // common_dim = 1024
	0x20, 0x00, // rank = 32
	0x00, 0x00, // mma_type = 0
	0x07, 0x01, 0x03, 0x01, 0x00, 0x00, // rows_pattern
	0x00, 0x01, 0x03, 0x01, 0x01, 0x01, // cols_pattern
	// reserved (32 bytes) are zero
}

// Mine mines a block using the Rust implementation and returns a ZKCertificate.
// This function modifies header.ProofCommitment to match the mined certificate.
func Mine(header *wire.BlockHeader) (*wire.ZKCertificate, error) {
	cHeader := blockHeaderToC(header)
	cMiningConfig := (*[miningConfigSize]C.uint8_t)(unsafe.Pointer(&defaultMiningConfig))

	proofData := make([]byte, wire.MaxZKProofSize)
	var pinner runtime.Pinner
	pinner.Pin(&proofData[0])
	defer pinner.Unpin()

	cZKProof := C.CZKProof{
		proof_blob_len: 0,
		proof_blob:     (*C.uint8_t)(unsafe.Pointer(&proofData[0])),
	}

	var errorBuf [C.ERROR_MSG_MAX_SIZE]C.char
	result := C.mine(
		C.uint32_t(DefaultM), C.uint32_t(DefaultN),
		&cHeader, cMiningConfig, &cZKProof, &errorBuf[0],
	)
	msg := C.GoString(&errorBuf[0])

	if result != 0 {
		return nil, fmt.Errorf("mining failed (code %d): %s", result, msg)
	}

	cert := &wire.ZKCertificate{
		ProofData: proofData[:int(cZKProof.proof_blob_len)],
	}
	C.memcpy(unsafe.Pointer(&cert.PublicData[0]), unsafe.Pointer(&cZKProof.public_data[0]), C.size_t(wire.PublicDataSize))

	header.ProofCommitment = cert.ProofCommitment()
	cert.Hash = header.BlockHash()

	return cert, nil
}
