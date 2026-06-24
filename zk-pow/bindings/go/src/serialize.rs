//! PlainProof serialization FFI — pack already-found merkle-proof components into the canonical
//! bincode `PlainProof` witness bytes.
//!
//! `prove_plain_proof` (prove.rs) and the pool's `mining.submit_share` (`plain_proof_b64`) both
//! consume a *serialized* `PlainProof` (see `PlainProof::deserialize_compat`). The akoya-derived
//! GPU worker, however, holds the proof as flat components — the same ones it would otherwise pack
//! into a protobuf `ShareSubmission`: per-matrix merkle leaves/indices/siblings, the matrix roots
//! (carried as `hash_a` / `hash_b`), and the winning `row_indices` (already in hand — see
//! `pearl_capi_merkle_root_and_proof`, which the worker feeds the very same `row_indices`).
//!
//! This export is the missing bridge: components in → canonical witness bytes out. The bytes are
//! byte-identical to what zk-pow itself produces (`bincode::serialize(&PlainProof)`, as exercised by
//! plain_proof.rs's round-trip tests and read back by `deserialize_compat`), because we construct the
//! real `PlainProof` type here rather than mirroring its layout.
//!
//! Dense (non-MoE) only: akoya's `ShareSubmission` carries no MoE fields, so `moe` is always `None`.
//! An MoE variant can be added when/if the akoya path grows expert routing.
//!
//! C ABI (cbindgen → zk_pow_ffi.h), so it is callable from Go (cgo) AND C#/.NET (P/Invoke).

use std::os::raw::c_char;
use std::ptr;
use std::slice;

use zk_pow::ffi::plain_proof::{MatrixMerkleProof, PlainProof};

use crate::common::{catch_panic, set_error_msg};

/// BLAKE3 chunk length — each merkle leaf is exactly this many bytes. Asserted against the real
/// `MatrixMerkleProof::new` array type at the call site (a mismatch is a compile error).
const CHUNK_LEN: usize = 1024;
/// BLAKE3 digest length — root and each sibling are exactly this many bytes.
const DIGEST_LEN: usize = 32;

/// Flat, C-compatible view of one matrix's merkle proof, as the GPU worker already holds it.
///
/// Layout mirrors the *outputs* of `pearl_capi_merkle_root_and_proof`: the worker computes these
/// once per matrix and hands them straight back here. All buffers are borrowed for the duration of
/// the call; this struct owns nothing.
#[repr(C)]
pub struct CMatrixProofIn {
    /// 32-byte merkle root (= `ShareSubmission.hash_a` / `hash_b`).
    pub root: *const u8,
    /// Total number of 1024-byte leaf chunks in the (padded) committed matrix.
    pub total_leaves: u32,
    /// Opened leaf chunks, `leaf_count * 1024` contiguous bytes.
    pub leaf_data: *const u8,
    pub leaf_count: usize,
    /// Chunk index of each opened leaf, one `u32` per `leaf_count`. Strictly increasing.
    pub leaf_indices: *const u32,
    pub leaf_indices_len: usize,
    /// Sibling digests, `sibling_count * 32` contiguous bytes. May be empty.
    pub siblings: *const u8,
    pub sibling_count: usize,
    /// Winning matrix row indices (A rows / B^T rows). Distinct from `leaf_indices`, which are
    /// chunk indices; the worker already possesses these (it chose the tile).
    pub row_indices: *const u32,
    pub row_indices_len: usize,
}

/// Serialize a dense `PlainProof` from its components into canonical witness bytes.
///
/// On success allocates `*out_witness` (Rust-owned) and sets `*out_witness_len`. The caller MUST
/// release it with [`zk_pow_free_buffer`] passing the same length. On any error nothing is
/// allocated, `*out_witness` is left NULL, and the message is written to `error_msg_out`.
///
/// # Returns
/// - 0: success (`*out_witness` / `*out_witness_len` populated)
/// - 2: error (bad input / panic — message in `error_msg_out`)
///
/// # Safety
/// - `a` and `bt` must point to valid `CMatrixProofIn` whose buffers are valid for the stated
///   lengths for the duration of the call.
/// - `out_witness` and `out_witness_len` must be non-null and writable.
/// - `error_msg_out` must be null or a valid pointer to a caller-allocated buffer of
///   `ERROR_MSG_MAX_SIZE` bytes.
#[no_mangle]
pub unsafe extern "C" fn serialize_plain_proof(
    m: u32,
    n: u32,
    k: u32,
    noise_rank: u32,
    a: *const CMatrixProofIn,
    bt: *const CMatrixProofIn,
    out_witness: *mut *mut u8,
    out_witness_len: *mut usize,
    error_msg_out: *mut c_char,
) -> i32 {
    // Zero outputs up front so partial-failure paths leave the caller consistent.
    if !out_witness.is_null() {
        *out_witness = ptr::null_mut();
    }
    if !out_witness_len.is_null() {
        *out_witness_len = 0;
    }

    if a.is_null() || bt.is_null() || out_witness.is_null() || out_witness_len.is_null() {
        set_error_msg(error_msg_out, "Null pointer");
        return 2;
    }

    let a = &*a;
    let bt = &*bt;

    let bytes = match catch_panic(|| serialize_inner(m, n, k, noise_rank, a, bt)) {
        Ok(Ok(b)) => b,
        Ok(Err(msg)) => {
            set_error_msg(error_msg_out, &msg);
            return 2;
        }
        Err(panic_msg) => {
            set_error_msg(error_msg_out, &format!("Serialize panic: {}", panic_msg));
            return 2;
        }
    };

    // Hand ownership to the caller. `into_boxed_slice` guarantees capacity == len, so
    // `zk_pow_free_buffer` can reconstitute the Vec with `(len, len)`.
    let boxed = bytes.into_boxed_slice();
    let len = boxed.len();
    *out_witness = Box::into_raw(boxed) as *mut u8;
    *out_witness_len = len;

    set_error_msg(error_msg_out, "Serialization successful");
    0
}

/// Free a buffer previously returned by [`serialize_plain_proof`].
///
/// # Safety
/// `ptr` must be a pointer returned by `serialize_plain_proof` and `len` the matching
/// `out_witness_len`. Must be called at most once per buffer. A NULL `ptr` is a no-op.
#[no_mangle]
pub unsafe extern "C" fn zk_pow_free_buffer(ptr: *mut u8, len: usize) {
    if ptr.is_null() || len == 0 {
        return;
    }
    drop(Vec::from_raw_parts(ptr, len, len));
}

/// Build a `PlainProof` from the two matrix component views and bincode-serialize it.
unsafe fn serialize_inner(
    m: u32,
    n: u32,
    k: u32,
    noise_rank: u32,
    a: &CMatrixProofIn,
    bt: &CMatrixProofIn,
) -> Result<Vec<u8>, String> {
    let a_proof = build_matrix("A", a)?;
    let bt_proof = build_matrix("B^T", bt)?;

    let proof = PlainProof {
        m: m as usize,
        n: n as usize,
        k: k as usize,
        noise_rank: noise_rank as usize,
        a: a_proof,
        bt: bt_proof,
        moe: None,
    };

    // `bincode::serialize` is fixint + little-endian, exactly what `PlainProof::deserialize_compat`
    // reads back (plain_proof.rs round-trips with this same call).
    bincode::serialize(&proof).map_err(|e| format!("bincode serialize failed: {}", e))
}

/// Convert a `CMatrixProofIn` into a `MatrixMerkleProof`, validating shapes.
unsafe fn build_matrix(label: &str, mp: &CMatrixProofIn) -> Result<MatrixMerkleProof, String> {
    if mp.root.is_null() {
        return Err(format!("{label}: root pointer is null"));
    }
    if mp.leaf_count > 0 && mp.leaf_data.is_null() {
        return Err(format!("{label}: leaf_data null with non-zero leaf_count"));
    }
    if mp.leaf_indices_len > 0 && mp.leaf_indices.is_null() {
        return Err(format!("{label}: leaf_indices null with non-zero len"));
    }
    if mp.sibling_count > 0 && mp.siblings.is_null() {
        return Err(format!("{label}: siblings null with non-zero count"));
    }
    if mp.row_indices_len > 0 && mp.row_indices.is_null() {
        return Err(format!("{label}: row_indices null with non-zero len"));
    }
    if mp.leaf_count != mp.leaf_indices_len {
        return Err(format!(
            "{label}: leaf_count ({}) must equal leaf_indices_len ({})",
            mp.leaf_count, mp.leaf_indices_len
        ));
    }

    let mut root = [0u8; DIGEST_LEN];
    ptr::copy_nonoverlapping(mp.root, root.as_mut_ptr(), DIGEST_LEN);

    let leaf_data: Vec<[u8; CHUNK_LEN]> = if mp.leaf_count == 0 {
        Vec::new()
    } else {
        let bytes = slice::from_raw_parts(mp.leaf_data, mp.leaf_count * CHUNK_LEN);
        bytes
            .chunks_exact(CHUNK_LEN)
            .map(|c| {
                let mut arr = [0u8; CHUNK_LEN];
                arr.copy_from_slice(c);
                arr
            })
            .collect()
    };

    let siblings: Vec<[u8; DIGEST_LEN]> = if mp.sibling_count == 0 {
        Vec::new()
    } else {
        let bytes = slice::from_raw_parts(mp.siblings, mp.sibling_count * DIGEST_LEN);
        bytes
            .chunks_exact(DIGEST_LEN)
            .map(|c| {
                let mut arr = [0u8; DIGEST_LEN];
                arr.copy_from_slice(c);
                arr
            })
            .collect()
    };

    let leaf_indices: Vec<usize> = if mp.leaf_indices_len == 0 {
        Vec::new()
    } else {
        slice::from_raw_parts(mp.leaf_indices, mp.leaf_indices_len)
            .iter()
            .map(|&x| x as usize)
            .collect()
    };

    let row_indices: Vec<usize> = if mp.row_indices_len == 0 {
        Vec::new()
    } else {
        slice::from_raw_parts(mp.row_indices, mp.row_indices_len)
            .iter()
            .map(|&x| x as usize)
            .collect()
    };

    Ok(MatrixMerkleProof::new(
        leaf_data,
        leaf_indices,
        row_indices,
        mp.total_leaves as usize,
        root,
        siblings,
    ))
}

#[cfg(test)]
mod tests {
    use super::*;

    /// A serialized PlainProof built via the FFI must deserialize back through the exact path the
    /// prover and pool use (`deserialize_compat`), with every field preserved. This is the
    /// byte-exactness gate for the akoya→PlainProof bridge.
    #[test]
    fn roundtrips_through_deserialize_compat() {
        let root_a = [0x11u8; DIGEST_LEN];
        let root_b = [0x22u8; DIGEST_LEN];
        let leaf_a = [7u8; CHUNK_LEN];
        let leaf_b = [9u8; CHUNK_LEN];
        let sib_a = [0xAAu8; DIGEST_LEN];
        let leaf_idx_a: [u32; 1] = [3];
        let leaf_idx_b: [u32; 1] = [5];
        let rows_a: [u32; 2] = [1, 2];
        let rows_b: [u32; 2] = [4, 6];

        let a = CMatrixProofIn {
            root: root_a.as_ptr(),
            total_leaves: 8,
            leaf_data: leaf_a.as_ptr(),
            leaf_count: 1,
            leaf_indices: leaf_idx_a.as_ptr(),
            leaf_indices_len: 1,
            siblings: sib_a.as_ptr(),
            sibling_count: 1,
            row_indices: rows_a.as_ptr(),
            row_indices_len: 2,
        };
        let bt = CMatrixProofIn {
            root: root_b.as_ptr(),
            total_leaves: 8,
            leaf_data: leaf_b.as_ptr(),
            leaf_count: 1,
            leaf_indices: leaf_idx_b.as_ptr(),
            leaf_indices_len: 1,
            siblings: ptr::null(),
            sibling_count: 0,
            row_indices: rows_b.as_ptr(),
            row_indices_len: 2,
        };

        let mut out_ptr: *mut u8 = ptr::null_mut();
        let mut out_len: usize = 0;
        let mut err = [0i8; crate::common::ERROR_MSG_MAX_SIZE];

        let rc = unsafe {
            serialize_plain_proof(2, 3, 16, 4, &a, &bt, &mut out_ptr, &mut out_len, err.as_mut_ptr())
        };
        assert_eq!(rc, 0, "serialize_plain_proof returned error");
        assert!(!out_ptr.is_null() && out_len > 0);

        let witness = unsafe { slice::from_raw_parts(out_ptr, out_len).to_vec() };
        let parsed = PlainProof::deserialize_compat(&witness).expect("deserialize_compat failed");

        assert_eq!(parsed.m, 2);
        assert_eq!(parsed.n, 3);
        assert_eq!(parsed.k, 16);
        assert_eq!(parsed.noise_rank, 4);
        assert!(parsed.moe.is_none());
        assert_eq!(parsed.a.proof.root, root_a);
        assert_eq!(parsed.bt.proof.root, root_b);
        assert_eq!(parsed.a.row_indices, vec![1, 2]);
        assert_eq!(parsed.bt.row_indices, vec![4, 6]);
        assert_eq!(parsed.a.proof.total_leaves, 8);
        assert_eq!(parsed.a.proof.leaf_indices, vec![3]);
        assert_eq!(parsed.bt.proof.leaf_indices, vec![5]);
        assert_eq!(parsed.a.proof.leaf_data, vec![leaf_a]);
        assert_eq!(parsed.a.proof.siblings, vec![sib_a]);
        assert!(parsed.bt.proof.siblings.is_empty());

        unsafe { zk_pow_free_buffer(out_ptr, out_len) };
    }
}
