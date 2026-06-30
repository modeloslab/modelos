//go:build zkpow

// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package zkpow provides ZK proof verification via Rust FFI.
package zkpow

/*
#cgo linux LDFLAGS: ${SRCDIR}/../../zk-pow/bindings/go/target/release/libzk_pow_ffi.a -ldl -lpthread -lm -lgcc_s
#cgo darwin LDFLAGS: ${SRCDIR}/../../zk-pow/bindings/go/target/release/libzk_pow_ffi.a -framework Security -lpthread -lm
#cgo windows LDFLAGS: ${SRCDIR}/../../zk-pow/bindings/go/target/x86_64-pc-windows-gnu/release/libzk_pow_ffi.a -lws2_32 -luserenv -lbcrypt -lntdll
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

// ================================================================================
// CERTIFICATE VERIFICATION
// ================================================================================

// VerifyCertificate performs sanity checks followed by cryptographic proof verification.
// It returns an error if the certificate is invalid or does not match the header.
func VerifyCertificate(header *wire.BlockHeader, cert wire.BlockCertificate) error {
	switch c := cert.(type) {
	case *wire.ZKCertificate:
		return verifyZKCertificate(header, c)
	default:
		return fmt.Errorf("unknown certificate type: %T", cert)
	}
}

// ================================================================================
// ZK CERTIFICATE VERIFICATION
// ================================================================================

func verifyZKCertificate(header *wire.BlockHeader, c *wire.ZKCertificate) error {
	return verifyZKCertificateInner(header, c, nil)
}

// VerifyZKCertificateWithNbits verifies a ZK certificate using nbitsOverride
// as the difficulty target instead of the block header's nbits field.
//
// WARNING: This bypasses the header's embedded difficulty, Do not use it in block acceptance or relay paths.
func VerifyZKCertificateWithNbits(header *wire.BlockHeader, c *wire.ZKCertificate, nbitsOverride uint32) error {
	return verifyZKCertificateInner(header, c, &nbitsOverride)
}

func verifyZKCertificateInner(header *wire.BlockHeader, c *wire.ZKCertificate, nbitsOverride *uint32) error {
	blockHash := header.BlockHash()
	if !c.Hash.IsEqual(&blockHash) {
		return fmt.Errorf("block hash mismatch: certificate has %s, header has %s",
			c.Hash, blockHash)
	}

	// Commitment binds the header to the certificate's PublicData via
	// SHA256d(version_LE || PublicData). The certificate version is V1 here for
	// both native modelOS blocks and the embedded Pearl parent on the AuxPoW path.
	//
	// AuxPoW NOTE: a post-MoE (V2) Pearl chain commits with a version-2 prefix.
	// modelOS deliberately accepts ONLY the V1 commitment — aux pools MUST normalise
	// the embedded Pearl header's commitment to the V1 form before submitauxblock
	// (the committed PublicData is byte-identical to V1, so this rewrites only the
	// 32-byte commitment field and is sound — the real Pearl block sent to pearld
	// keeps its native V2 commitment). Accepting both versions was rejected because
	// it would let one proof mint two distinct same-height blocks (malleability);
	// native-V2 acceptance is deferred to a future coordinated hard fork.
	certCommitment := c.ProofCommitment()
	if header.ProofCommitment != certCommitment {
		return fmt.Errorf("proof commitment mismatch: header has %s, certificate has %s",
			header.ProofCommitment, certCommitment)
	}

	if len(c.ProofData) == 0 { // avoid this case because of c.ProofData[0] access below
		return fmt.Errorf("empty proof data")
	}

	cBlockHeader := blockHeaderToC(header)

	var cZKProof C.CZKProof
	// v2 CZKProof carries a variable-length public_data buffer (PUBLICDATA_MAX_SIZE,
	// sized for MoE proofs) plus an explicit public_data_len. Base (non-MoE) proofs —
	// which is what modelOS mines — use PublicDataSize (164), matching wire.ZKCertificate.
	cZKProof.public_data_len = C.uintptr_t(wire.PublicDataSize)
	C.memcpy(unsafe.Pointer(&cZKProof.public_data[0]), unsafe.Pointer(&c.PublicData[0]), C.size_t(wire.PublicDataSize))

	// Pin the ProofData memory to prevent GC from moving it during the C call
	var pinner runtime.Pinner
	pinner.Pin(&c.ProofData[0])
	defer pinner.Unpin()

	proofBlobPtr := (*C.uint8_t)(unsafe.Pointer(&c.ProofData[0]))
	cZKProof.proof_blob_len = C.uintptr_t(len(c.ProofData))
	cZKProof.proof_blob = proofBlobPtr

	// Call Rust FFI
	var errorBuf [C.ERROR_MSG_MAX_SIZE]C.char
	var result C.int32_t
	if nbitsOverride != nil {
		result = C.verify_zk_proof_v2_with_nbits(&cBlockHeader, &cZKProof, C.uint32_t(*nbitsOverride), &errorBuf[0])
	} else {
		result = C.verify_zk_proof_v2(&cBlockHeader, &cZKProof, &errorBuf[0])
	}
	msg := C.GoString(&errorBuf[0])

	switch result {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("proof rejected: %s", msg)
	case 2:
		return fmt.Errorf("verification system error: %s", msg)
	default:
		return fmt.Errorf("unknown verification result %d: %s", result, msg)
	}
}

// ================================================================================
// FFI CONVERSION HELPERS
// ================================================================================

// blockHeaderToC converts a Go BlockHeader to C.IncompleteBlockHeader.
// Note: PrevBlock and MerkleRoot are reversed from wire order to display order
func blockHeaderToC(header *wire.BlockHeader) C.IncompleteBlockHeader {
	cHeader := C.IncompleteBlockHeader{
		version:   C.uint32_t(header.Version),
		timestamp: C.uint32_t(header.Timestamp.Unix()),
		nbits:     C.uint32_t(header.Bits),
	}
	// Reverse hashes from wire order (internal) to display order
	hashLen := len(header.PrevBlock)
	for i := range cHeader.prev_block {
		cHeader.prev_block[i] = C.uint8_t(header.PrevBlock[hashLen-1-i])
		cHeader.merkle_root[i] = C.uint8_t(header.MerkleRoot[hashLen-1-i])
	}
	return cHeader
}
