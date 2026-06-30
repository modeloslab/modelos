//! ZK Proof Verification FFI (Security Critical)
//!
//! This module contains ZK proof verification FFI functions.
//! Extra care must be taken when modifying this code as it's critical for security.

use std::os::raw::c_char;
use std::slice;

use crate::common::MAX_ZK_PROOF_SIZE;
use zk_pow::api::proof::{IncompleteBlockHeader, PublicProofParams, ZKProof};
use zk_pow::api::verify;
use zk_pow::ffi::plain_proof::PlainProof;

use crate::common::{acquire_cache, catch_panic, set_error_msg, CZKProof};

// ============================================================================
// ZK Proof Verification FFI
// ============================================================================

/// Shared implementation for ZK proof verification.
///
/// # Safety
/// - All pointers must be valid
/// - `zk_proof.proof_blob` must be a valid pointer to `proof_blob_len` bytes
/// - `error_msg_out` must be null or a valid pointer to a caller-allocated buffer of `ERROR_MSG_MAX_SIZE` bytes
unsafe fn verify_zk_proof_inner(
    block_header: *const IncompleteBlockHeader,
    zk_proof: *const CZKProof,
    nbits_override: Option<u32>,
    error_msg_out: *mut c_char,
) -> i32 {
    // Wrap in catch_unwind to prevent panics from crossing FFI boundary
    let result = catch_panic(|| {
        // Validate input pointers
        if block_header.is_null() || zk_proof.is_null() {
            set_error_msg(error_msg_out, "Null pointer");
            return 2;
        }

        let zk_proof_ref = &*zk_proof;

        if zk_proof_ref.proof_blob.is_null() || zk_proof_ref.proof_blob_len == 0 {
            set_error_msg(error_msg_out, "Null or empty proof blob");
            return 1;
        }
        if zk_proof_ref.proof_blob_len > MAX_ZK_PROOF_SIZE {
            set_error_msg(error_msg_out, "ZK Proof too large");
            return 1;
        }

        if !PublicProofParams::is_valid_wire_size(zk_proof_ref.public_data_len) {
            set_error_msg(
                error_msg_out,
                &format!("invalid public_data_len {}", zk_proof_ref.public_data_len),
            );
            return 1;
        }

        let plonky2_proof = slice::from_raw_parts(zk_proof_ref.proof_blob, zk_proof_ref.proof_blob_len);
        let public_data = &zk_proof_ref.public_data[..zk_proof_ref.public_data_len];
        let (params, zk_proof) = match ZKProof::deserialize(*block_header, public_data, plonky2_proof) {
            Ok(r) => r,
            Err(e) => {
                set_error_msg(error_msg_out, &format!("{}", e));
                return 1;
            }
        };

        // Acquire circuit cache (immutable - verifier doesn't modify cache). A
        // missing/corrupt cache returns a clean, actionable error instead of
        // panicking and poisoning the verifier for all subsequent blocks. Return
        // code 2 (system error) — not 1 (proof rejected) — so operators can tell an
        // infra/cache fault from a genuinely invalid proof. Both reject the block.
        let cache = match acquire_cache() {
            Ok(c) => c,
            Err(e) => {
                set_error_msg(error_msg_out, &format!("{}", e));
                return 2;
            }
        };

        // Verify using cached circuits only (no compilation)
        match verify::verify_block_cached_circuits_only(&params, &zk_proof, &cache, nbits_override) {
            Ok(_) => {
                set_error_msg(error_msg_out, "Proof verified successfully");
                0
            }
            Err(e) => {
                set_error_msg(error_msg_out, &format!("{}", e));
                1
            }
        }
    });

    match result {
        Ok(code) => code,
        Err(panic_msg) => {
            set_error_msg(error_msg_out, &format!("Internal panic: {}", panic_msg));
            2
        }
    }
}

/// Verify a ZK proof against public parameters.
///
/// # Security Considerations
/// - This is the primary entry point for verifying ZK proofs
/// - Validates all input parameters before verification
/// - Uses panic handling to prevent crashes from poisoning global state
/// - All validation errors return specific codes and messages
///
/// # Returns
/// - 0: Proof verified and accepted
/// - 1: Proof verified but rejected (proof is invalid)
/// - 2: System error (could not run verification)
///
/// # Safety
/// - All pointers must be valid
/// - `zk_proof.proof_blob` must be a valid pointer to `proof_blob_len` bytes
/// - `error_msg_out` must be null or a valid pointer to a caller-allocated buffer of `ERROR_MSG_MAX_SIZE` bytes
///
/// Verify a ZK proof. Panics are caught to prevent undefined behavior at FFI boundary.
/// Returns: 0 = success, 1 = proof rejected, 2 = system error.
#[no_mangle]
pub unsafe extern "C" fn verify_zk_proof_v2(
    block_header: *const IncompleteBlockHeader,
    zk_proof: *const CZKProof,
    error_msg_out: *mut c_char,
) -> i32 {
    verify_zk_proof_inner(block_header, zk_proof, None, error_msg_out)
}

/// Verify a ZK proof against public parameters, overriding the difficulty with the given nbits.
///
/// Identical to `verify_zk_proof_v2` except the difficulty target is derived from `nbits_override`
/// instead of the block header's nbits field.
///
/// # Returns
/// - 0: Proof verified and accepted
/// - 1: Proof verified but rejected (proof is invalid)
/// - 2: System error (could not run verification)
///
/// # Safety
/// - All pointers must be valid
/// - `zk_proof.proof_blob` must be a valid pointer to `proof_blob_len` bytes
/// - `error_msg_out` must be null or a valid pointer to a caller-allocated buffer of `ERROR_MSG_MAX_SIZE` bytes
#[no_mangle]
pub unsafe extern "C" fn verify_zk_proof_v2_with_nbits(
    block_header: *const IncompleteBlockHeader,
    zk_proof: *const CZKProof,
    nbits_override: u32,
    error_msg_out: *mut c_char,
) -> i32 {
    verify_zk_proof_inner(block_header, zk_proof, Some(nbits_override), error_msg_out)
}

/// Node-exact "does this witness clear `nbits_override`?" check — NO ZK proof, NO circuit cache.
///
/// This is the standalone-v2 block-detection gate, mirroring v1's `verify_plain_proof_v2`: it
/// recomputes the jackpot from the witness through the SAME verifier path the node/pool use
/// (`verify::verify_plain_proof` → `check_jackpot_against_nbits` → `extract_difficulty_bound`), so a
/// "clears" verdict is byte-identical to the consensus decision. The miner uses it to decide whether
/// a found share also clears the block target and must therefore be ZK-proved + submit_block_proof'd.
/// Replaces the host-side target arithmetic (which historically diverged by the difficulty factor).
///
/// # Returns
/// - 0: the witness's jackpot clears `nbits_override` (it is a block — prove it)
/// - 1: does not clear (ordinary share) OR the witness is structurally invalid
/// - 2: system error (null pointer / un-deserializable witness / panic)
///
/// # Safety
/// - `block_header` must point to a valid `IncompleteBlockHeader`.
/// - `witness_bytes` must point to `witness_len` bytes (a serialized `PlainProof`, current or
///   legacy V1 layout — see `PlainProof::deserialize_compat`).
/// - `error_msg_out` must be null or a caller-allocated `ERROR_MSG_MAX_SIZE` buffer.
#[no_mangle]
pub unsafe extern "C" fn verify_plain_proof(
    block_header: *const IncompleteBlockHeader,
    witness_bytes: *const u8,
    witness_len: usize,
    nbits_override: u32,
    error_msg_out: *mut c_char,
) -> i32 {
    let result = catch_panic(|| {
        if block_header.is_null() || witness_bytes.is_null() {
            set_error_msg(error_msg_out, "Null pointer");
            return 2;
        }
        let header = *block_header;
        let witness = slice::from_raw_parts(witness_bytes, witness_len);
        let plain_proof = match PlainProof::deserialize_compat(witness) {
            Ok(p) => p,
            Err(e) => {
                set_error_msg(error_msg_out, &format!("Invalid witness: {}", e));
                return 2;
            }
        };
        match verify::verify_plain_proof(&header, &plain_proof, Some(nbits_override)) {
            Ok(()) => {
                set_error_msg(error_msg_out, "clears target");
                0
            }
            Err(e) => {
                set_error_msg(error_msg_out, &format!("{}", e));
                1
            }
        }
    });

    match result {
        Ok(code) => code,
        Err(panic_msg) => {
            set_error_msg(error_msg_out, &format!("Internal panic: {}", panic_msg));
            2
        }
    }
}

/// Verify a V1 (version 1, master-format) ZK proof.
/// Uses the V1 circuit cache which contains master-compatible verifier circuits.
///
/// # Returns
/// - 0: Proof verified and accepted
/// - 1: Proof verified but rejected
/// - 2: System error
///
/// # Safety
/// Same requirements as `verify_zk_proof_v2`.
#[no_mangle]
pub unsafe extern "C" fn verify_zk_proof_v1(
    block_header: *const IncompleteBlockHeader,
    zk_proof: *const CZKProof,
    error_msg_out: *mut c_char,
) -> i32 {
    let result = catch_panic(|| {
        if block_header.is_null() || zk_proof.is_null() {
            set_error_msg(error_msg_out, "Null pointer");
            return 2;
        }

        let zk_proof_ref = &*zk_proof;

        if zk_proof_ref.proof_blob.is_null() || zk_proof_ref.proof_blob_len == 0 {
            set_error_msg(error_msg_out, "Null or empty proof blob");
            return 1;
        }
        if zk_proof_ref.proof_blob_len > MAX_ZK_PROOF_SIZE {
            set_error_msg(error_msg_out, "ZK Proof too large");
            return 1;
        }

        let expected_len = zk_pow::v1::api::proof::PublicProofParams::PUBLICDATA_SIZE;
        if zk_proof_ref.public_data_len != expected_len {
            set_error_msg(
                error_msg_out,
                &format!(
                    "v1 proof requires {} byte public_data, got {}",
                    expected_len, zk_proof_ref.public_data_len
                ),
            );
            return 1;
        }

        let block_header_ref = &*block_header;
        let block_header_bytes = block_header_ref.to_bytes();
        let public_data = &zk_proof_ref.public_data[..zk_proof_ref.public_data_len];
        let proof_data = slice::from_raw_parts(zk_proof_ref.proof_blob, zk_proof_ref.proof_blob_len);

        let cache = match crate::common::acquire_v1_cache() {
            Ok(c) => c,
            Err(e) => {
                // Code 2 (system error), not 1 (proof rejected): a cache fault is
                // infra, not a bad proof. Both reject the block.
                set_error_msg(error_msg_out, &format!("{}", e));
                return 2;
            }
        };

        match zk_pow::v1::verify_v1(&block_header_bytes, public_data, proof_data, &cache, None) {
            Ok(_) => {
                set_error_msg(error_msg_out, "V1 proof verified successfully");
                0
            }
            Err(e) => {
                set_error_msg(error_msg_out, &format!("{}", e));
                1
            }
        }
    });

    match result {
        Ok(code) => code,
        Err(panic_msg) => {
            set_error_msg(error_msg_out, &format!("Internal panic: {}", panic_msg));
            2
        }
    }
}
