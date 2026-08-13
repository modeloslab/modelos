//! Standalone ZK prove FFI — generate a plonky2 proof from an already-found witness.
//!
//! `mine` (mine.rs) does search + prove in one step (the solo model). For pool / akoya-parity
//! mining the GPU search produces a cheap PlainProof *witness* and ONLY a block-clearing find is
//! ZK-proved. This export covers that second step: take the witness bytes the miner already has and
//! produce the serialized ZK proof to submit via `mining.submit_block_proof`. The Rust function it
//! wraps (`prove::zk_prove_plain_proof`) is the same one py-pearl-mining's `generate_proof_v2` uses.
//!
//! C ABI (cbindgen → zk_pow_ffi.h), so it is callable from Go (cgo) AND C#/.NET (P/Invoke).

use std::os::raw::c_char;
use std::slice;
use std::sync::OnceLock;

use zk_pow::api::prove;
use zk_pow::api::proof::{SeedDerivation, IncompleteBlockHeader, PublicProofParams};
use zk_pow::ffi::plain_proof::PlainProof;

use crate::common::{
    acquire_cache, catch_panic, set_error_msg, CZKProof, MAX_ZK_PROOF_SIZE,
};

/// Dedicated Rayon pool for block proving, isolated from the process-global Rayon pool that
/// pearl-blake3 / the akoya witness build use. Two properties make proving non-blocking to the
/// miner's hot path (mining + per-find witness build + share submit) WITHOUT any RAYON_NUM_THREADS
/// env (which would also throttle the akoya core):
///   1. Its own pool — plonky2's `par_iter`/`join` inside `pool.install(...)` use THESE threads, so
///      capping/nicing them never touches the global pool the share path runs on.
///   2. Niced to the bottom (Linux) — the OS always schedules the normal-priority mining/witness/IO
///      threads ahead of the prover, so a ~40s proof consumes only otherwise-idle CPU and yields
///      instantly when a share needs the core. (Proving is already serialized by the circuit-cache
///      Mutex, so one such pool serves all finds.)
static PROVE_POOL: OnceLock<rayon::ThreadPool> = OnceLock::new();

fn prove_pool() -> &'static rayon::ThreadPool {
    PROVE_POOL.get_or_init(|| {
        // Size the dedicated prove pool to a FRACTION of cores (default 35%) so block proving can
        // never occupy the whole CPU and starve the akoya hot path (GPU-feed + per-find witness
        // Merkle on the GLOBAL rayon pool + share submission). Bounded-dedicated-pool isolation at
        // the correct layer: plonky2's par_iter/join run inside `prove_pool().install(...)`, so
        // capping THESE threads never touches the global pool the share path uses. Tunable via
        // MODELOS_PROVE_CPU_PCT (5..=100; 100 = all cores).
        let cores = std::thread::available_parallelism().map(|p| p.get()).unwrap_or(4);
        let pct = std::env::var("MODELOS_PROVE_CPU_PCT")
            .ok()
            .and_then(|s| s.parse::<usize>().ok())
            .map(|p| p.clamp(5, 100))
            .unwrap_or(35);
        let threads = std::cmp::max(1, cores * pct / 100);
        eprintln!("zk-prove: dedicated pool {threads}/{cores} cores (~{pct}% CPU cap), niced");
        rayon::ThreadPoolBuilder::new()
            .num_threads(threads)
            .thread_name(|i| format!("zkprove-{i}"))
            .start_handler(|_| lower_prover_priority())
            .build()
            .expect("failed to build dedicated prove thread pool")
    })
}

/// Nice the calling (prover) thread to the bottom so mining/share threads always preempt it.
#[cfg(target_os = "linux")]
fn lower_prover_priority() {
    // PRIO_PROCESS with who=0 targets the current thread on Linux (per-thread nice values).
    unsafe { libc::setpriority(libc::PRIO_PROCESS, 0, 19); }
}
#[cfg(not(target_os = "linux"))]
fn lower_prover_priority() {}

/// Generate a ZK proof for an already-found witness (the block-proving step for pool mining).
///
/// # Returns
/// - 0: Proof generation successful (`zk_proof_out` populated)
/// - 2: System error (bad input / deserialize / prove failure / panic — message in `error_msg_out`)
///
/// # Safety
/// - `block_header` must be a valid pointer to an `IncompleteBlockHeader`.
/// - `witness_bytes` must be a valid pointer to `witness_len` bytes (a serialized `PlainProof`,
///   current or legacy V1 format — see `PlainProof::deserialize_compat`).
/// - `zk_proof_out` must be non-null and its `proof_blob` a caller-allocated buffer of
///   `MAX_ZK_PROOF_SIZE` bytes.
/// - `error_msg_out` must be null or a valid pointer to a caller-allocated buffer of
///   `ERROR_MSG_MAX_SIZE` bytes.
unsafe fn prove_plain_proof_impl(
    block_header: *const IncompleteBlockHeader,
    witness_bytes: *const u8,
    witness_len: usize,
    zk_proof_out: *mut CZKProof,
    error_msg_out: *mut c_char,
    seed_derivation: SeedDerivation,
) -> i32 {
    if block_header.is_null() || witness_bytes.is_null() || zk_proof_out.is_null() {
        set_error_msg(error_msg_out, "Null pointer");
        return 2;
    }

    // The caller passes the 76 raw WIRE header bytes. Reinterpreting them straight as the #[repr(C)]
    // struct (`*block_header`) leaves prev_block/merkle_root in WIRE order — but job_key/to_bytes
    // expect INTERNAL order (they reverse those fields). `IncompleteBlockHeader::from_bytes` performs
    // that wire→internal reversal, exactly like the python prove path (generate_proof_v2), so the
    // recomputed job_key = BLAKE3(wire_header ‖ config) MATCHES the key the witness's A-merkle was
    // built with (the miner/pool hash the wire bytes as-is). Skipping it computed job_key over the
    // wrong byte order → the A-root the circuit commits to ≠ the witness root → "Hash A mismatch".
    let header_bytes = slice::from_raw_parts(block_header as *const u8, IncompleteBlockHeader::SERIALIZED_SIZE);
    let header = match IncompleteBlockHeader::from_bytes(header_bytes) {
        Ok(h) => h,
        Err(e) => {
            set_error_msg(error_msg_out, &format!("Invalid block header: {}", e));
            return 2;
        }
    };
    let out = &mut *zk_proof_out;
    if out.proof_blob.is_null() {
        set_error_msg(error_msg_out, "proof_blob buffer is null");
        return 2;
    }

    // Deserialize the witness the GPU search produced (accepts current + legacy V1 layouts).
    let witness = slice::from_raw_parts(witness_bytes, witness_len);
    let plain_proof = match PlainProof::deserialize_compat(witness) {
        Ok(p) => p,
        Err(e) => {
            set_error_msg(error_msg_out, &format!("Invalid witness: {}", e));
            return 2;
        }
    };

    // Generate the ZK proof. sanity_check=false matches the mine.rs prove path (the witness already
    // cleared the on-device check; the node re-verifies on submission anyway). acquire_cache +
    // catch_panic mirror the existing exports so a prover panic never unwinds across the FFI.
    // Run the proof on the dedicated low-priority pool. The cache Mutex is acquired INSIDE the
    // pool closure (it is !Send, so it must not cross the install boundary) — this also serializes
    // concurrent proofs naturally. plonky2's parallel work inside uses the niced prove pool, so it
    // never competes at normal priority with mining + share submission.
    let result = match catch_panic(move || {
        prove_pool().install(move || {
            // Propagate a missing/corrupt-cache error as a clean prove error
            // instead of panicking and poisoning the cache for all callers.
            let mut cache = acquire_cache()?;
            prove::zk_prove_plain_proof(header, &plain_proof, &mut cache, false, seed_derivation)
        })
    }) {
        Ok(Ok(r)) => r,
        Ok(Err(e)) => {
            set_error_msg(error_msg_out, &format!("Prove failed: {}", e));
            return 2;
        }
        Err(panic_msg) => {
            set_error_msg(error_msg_out, &format!("Prove panic: {}", panic_msg));
            return 2;
        }
    };

    // Copy the serialized proof into the caller's CZKProof (same layout mine_inner writes).
    let pd_len = result.public_data.len();
    if !PublicProofParams::is_valid_wire_size(pd_len) {
        set_error_msg(error_msg_out, &format!("public_data length {} is out of valid range", pd_len));
        return 2;
    }
    out.public_data_len = pd_len;
    out.public_data[..pd_len].copy_from_slice(&result.public_data);

    if result.proof_data.len() > MAX_ZK_PROOF_SIZE {
        set_error_msg(error_msg_out, "proof_data exceeds MAX_ZK_PROOF_SIZE");
        return 2;
    }
    let buffer = slice::from_raw_parts_mut(out.proof_blob, MAX_ZK_PROOF_SIZE);
    buffer[..result.proof_data.len()].copy_from_slice(&result.proof_data);
    out.proof_blob_len = result.proof_data.len();

    set_error_msg(error_msg_out, "Proof generation successful");
    0
}

/// Prove a witness under the pre-V3 (legacy) noise-seed derivation. Unchanged ABI.
///
/// # Safety
/// Same requirements as `prove_plain_proof_impl`.
#[no_mangle]
pub unsafe extern "C" fn prove_plain_proof(
    block_header: *const IncompleteBlockHeader,
    witness_bytes: *const u8,
    witness_len: usize,
    zk_proof_out: *mut CZKProof,
    error_msg_out: *mut c_char,
) -> i32 {
    prove_plain_proof_impl(block_header, witness_bytes, witness_len, zk_proof_out, error_msg_out, SeedDerivation::Legacy)
}

/// Prove a witness under the V3 salted noise-seed derivation (Pearl SaltedSeedForkHeight onward).
///
/// # Safety
/// Same requirements as `prove_plain_proof`.
#[no_mangle]
pub unsafe extern "C" fn prove_plain_proof_v3(
    block_header: *const IncompleteBlockHeader,
    witness_bytes: *const u8,
    witness_len: usize,
    zk_proof_out: *mut CZKProof,
    error_msg_out: *mut c_char,
) -> i32 {
    prove_plain_proof_impl(block_header, witness_bytes, witness_len, zk_proof_out, error_msg_out, SeedDerivation::Salted)
}
