//! Unified Python module for Pearl mining.
//!
//! Registers types from pearl-blake3 and zk-pow into a single Python module.
//! No wrapper types -- all #[pyclass] types are defined in their respective core crates.

#[cfg(unix)]
#[global_allocator]
static GLOBAL: tikv_jemallocator::Jemalloc = tikv_jemallocator::Jemalloc;

use lazy_static::lazy_static;
use pyo3::prelude::*;
use std::sync::Mutex;

use blake3::CHUNK_LEN;
use pearl_blake3::{pad_to_chunk_boundary, MerkleProof, MerkleTree};
use zk_pow::api::proof::{
    IncompleteBlockHeader, MMAType, MiningConfiguration, MoEConfig, PeriodicPattern,
    PublicProofParams, ZKProof,
};
use zk_pow::api::{prove, verify};
use zk_pow::circuit::pearl_circuit::{PearlRecursion, RecursionCircuit};
use zk_pow::ffi::mine::{mine as ffi_mine, mine_moe as ffi_mine_moe};
use zk_pow::ffi::plain_proof::{
    check_cert_version_eligible as core_check_cert_version_eligible, CertificateVersion,
    MatrixMerkleProof, MoEProofParams, PlainProof,
};

use zk_pow::v1::api::proof as v1_proof;
use zk_pow::v1::api::{prove as v1_prove, verify as v1_verify};

fn py_err(msg: &str, e: impl std::fmt::Display) -> PyErr {
    PyErr::new::<pyo3::exceptions::PyRuntimeError, _>(format!("{}: {}", msg, e))
}

// ============================================================================
// ZK Proof (only type defined in the binding crate)
// ============================================================================

#[pyclass(name = "ZKProof", get_all)]
#[derive(Clone)]
struct PyProof {
    public_data: Vec<u8>,
    proof_data: Vec<u8>,
}

#[pymethods]
impl PyProof {
    #[new]
    fn new(public_data: Vec<u8>, proof_data: Vec<u8>) -> PyResult<Self> {
        if !PublicProofParams::is_valid_wire_size(public_data.len()) {
            return Err(pyo3::exceptions::PyValueError::new_err(format!(
                "public_data length must be {} bytes (non-MoE) or {}..={} (MoE)",
                PublicProofParams::WIRE_SIZE,
                PublicProofParams::MIN_MOE_WIRE_SIZE,
                PublicProofParams::MAX_WIRE_SIZE,
            )));
        }
        Ok(Self {
            public_data,
            proof_data,
        })
    }
}

// ============================================================================
// ZK Functions
// ============================================================================

type CircuitCache = <PearlRecursion as RecursionCircuit>::CircuitCache;

lazy_static! {
    static ref CIRCUIT_CACHE: Mutex<CircuitCache> = Mutex::new(CircuitCache::default());
}

fn acquire_cache() -> PyResult<std::sync::MutexGuard<'static, CircuitCache>> {
    CIRCUIT_CACHE
        .lock()
        .map_err(|_| py_err("Cache poisoned by prior panic", "restart required"))
}

#[pyfunction]
fn generate_proof_v2(
    py: Python<'_>,
    block_header: IncompleteBlockHeader,
    plain_proof: PlainProof,
) -> PyResult<PyProof> {
    // RELEASE THE GIL for the duration of the ~18s plonky2 prove. Without this, even though the
    // miner runs this on a worker thread, holding the GIL freezes the WHOLE asyncio event loop for
    // the prove's duration — so witness shares awaiting the pool's network response stalled ~18s
    // (observed: "witness-share ACCEPTED in 17779 ms" exactly matching a 17.6s prove). allow_threads
    // lets the mine loop + share submits + status run concurrently with proving. Inputs are owned
    // values (Send); the circuit-cache mutex is acquired inside, so concurrent proves still serialize
    // on the cache (PROVE_WORKERS>1) but no longer serialize the entire event loop.
    let (public_data, proof_data) = py.allow_threads(|| -> PyResult<(Vec<u8>, Vec<u8>)> {
        let mut cache = acquire_cache()?;
        let result = prove::zk_prove_plain_proof(block_header, &plain_proof, &mut cache, true)
            .map_err(|e| py_err("Prove failed", e))?;
        Ok((result.public_data, result.proof_data))
    })?;

    Ok(PyProof {
        public_data,
        proof_data,
    })
}

#[pyfunction]
fn verify_proof_v2(
    py: Python<'_>,
    block_header: IncompleteBlockHeader,
    proof: &PyProof,
) -> PyResult<(bool, String)> {
    if !PublicProofParams::is_valid_wire_size(proof.public_data.len()) {
        return Err(pyo3::exceptions::PyValueError::new_err(format!(
            "public_data length must be {} bytes (non-MoE) or {}..={} (MoE)",
            PublicProofParams::WIRE_SIZE,
            PublicProofParams::MIN_MOE_WIRE_SIZE,
            PublicProofParams::MAX_WIRE_SIZE,
        )));
    }

    let (params, zk_proof) =
        ZKProof::deserialize(block_header, &proof.public_data, &proof.proof_data)
            .map_err(|e| py_err("Deserialize failed", e))?;

    // RELEASE THE GIL for the heavy block-proof verify (builds the recursive circuit). The POOL
    // runs this on every submit_block_proof; holding the GIL froze its whole asyncio loop for the
    // verify's duration, so all in-flight witness shares stalled (observed: "witness-share ACCEPTED
    // in 9467 ms" next to a block prove/verify). allow_threads lets share grading + submits proceed
    // concurrently with block verification. params/zk_proof are owned → moved in.
    py.allow_threads(move || {
        let mut cache = acquire_cache()?;
        match verify::verify_block(&params, &zk_proof, &mut cache) {
            Ok(_) => Ok((true, "Verified".into())),
            Err(e) => Ok((false, format!("Rejected: {}", e))),
        }
    })
}

/// Verify a v2 ZK proof, but grade its jackpot against `nbits_override` instead of the
/// header's own nbits. Used by the merged-mining pool: miners mine the Pearl header (Pearl
/// bits) but prove on the easier MDL (child) target, so the pool verifies the proof at the
/// MDL target — exactly what the node's VerifyAuxPow does. Returns (accepted, message).
#[pyfunction]
fn verify_proof_v2_with_nbits(
    py: Python<'_>,
    block_header: IncompleteBlockHeader,
    proof: &PyProof,
    nbits_override: u32,
) -> PyResult<(bool, String)> {
    if !PublicProofParams::is_valid_wire_size(proof.public_data.len()) {
        return Err(pyo3::exceptions::PyValueError::new_err(format!(
            "public_data length must be {} bytes (non-MoE) or {}..={} (MoE)",
            PublicProofParams::WIRE_SIZE,
            PublicProofParams::MIN_MOE_WIRE_SIZE,
            PublicProofParams::MAX_WIRE_SIZE,
        )));
    }

    let (params, zk_proof) =
        ZKProof::deserialize(block_header, &proof.public_data, &proof.proof_data)
            .map_err(|e| py_err("Deserialize failed", e))?;

    // Build-on-miss (like verify_proof_v2), NOT cached-only: the pool's submit_block_proof
    // verify runs in a thread whose process-global cache may be cold (the in-process warmup
    // is racy — block proofs can arrive before any witness warms it). Building on miss makes
    // AuxPoW verify reliable exactly as the non-AuxPoW path is. Non-consensus (pool safety
    // gate); the node's Go FFI stays cached-only.
    // RELEASE THE GIL for the heavy verify (same reason as verify_proof_v2 — this is the AuxPoW
    // block-proof verify the pool runs; freezing the event loop here stalled witness shares).
    py.allow_threads(move || {
        let mut cache = acquire_cache()?;
        match verify::verify_block_with_nbits(&params, &zk_proof, &mut cache, Some(nbits_override)) {
            Ok(_) => Ok((true, "Verified".into())),
            Err(e) => Ok((false, format!("Rejected: {}", e))),
        }
    })
}

#[pyfunction]
#[pyo3(name = "pad_to_chunk_boundary")]
fn py_pad_to_chunk_boundary(data: &[u8]) -> Vec<u8> {
    pad_to_chunk_boundary(data)
}

#[pyfunction]
fn clear_circuit_cache_v2() -> PyResult<()> {
    acquire_cache()?.clear();
    Ok(())
}

#[pyfunction]
fn warmup_prove_v2(mining_config: MiningConfiguration) -> PyResult<()> {
    let mut cache = acquire_cache()?;
    prove::warmup_prove(mining_config, &mut cache).map_err(|e| py_err("Warmup prove failed", e))
}

#[pyfunction]
#[pyo3(signature = (block_header, plain_proof, nbits_override=None))]
fn verify_plain_proof_v2(
    py: Python<'_>,
    block_header: IncompleteBlockHeader,
    plain_proof: PlainProof,
    nbits_override: Option<u32>,
) -> PyResult<(bool, String)> {
    // RELEASE THE GIL: this is the miner's per-share self-check, run for EVERY witness in the
    // witness pool (up to 3×/share). Holding the GIL serialized all witness-build threads on the
    // verify portion, so MODELOS_WITNESS_WORKERS could not actually parallelize. allow_threads lets
    // the N build threads recompute concurrently (the work is pure CPU over owned inputs).
    py.allow_threads(
        || match verify::verify_plain_proof(&block_header, &plain_proof, nbits_override) {
            Ok(()) => Ok((true, "Mining solution verified successfully".into())),
            Err(e) => Ok((false, e.to_string())),
        },
    )
}

/// Recompute the 32-byte jackpot from a witness — no ZK proof, no difficulty check.
/// Returns the jackpot bytes; raises ValueError on an invalid witness (bad merkle
/// openings, out-of-range strips, sanity failure). Used by the pool's cheap
/// witness-grading path to grade one witness against share/block/pearl targets.
#[pyfunction]
fn verify_plain_proof_jackpot_v2(
    py: Python<'_>,
    block_header: IncompleteBlockHeader,
    plain_proof: PlainProof,
) -> PyResult<Vec<u8>> {
    // RELEASE THE GIL: per-share jackpot recompute, also run in the witness pool for grading. Same
    // rationale as verify_plain_proof_v2 — lets the build threads run genuinely in parallel.
    py.allow_threads(|| {
        verify::verify_plain_proof_jackpot(&block_header, &plain_proof)
            .map(|h| h.to_vec())
            .map_err(value_err)
    })
}

/// CHEAP host-side jackpot recompute from RAW opened strips (miner pre-filter only — NOT consensus).
/// Returns the byte-identical 32-byte jackpot hash that `verify_plain_proof_jackpot_v2` produces, but
/// takes the opened A-rows / B-cols + commitment seeds directly, so it SKIPS the Merkle-tree build in
/// `create_proof`. The miner uses it to drop the GPU's over-signals before the expensive witness build.
/// Does NO membership verification, so it must never grade an untrusted submission. See
/// `zk_pow::api::verify::plain_jackpot_from_strips` (validated byte-exact in unit tests).
#[pyfunction]
#[allow(clippy::too_many_arguments)]
fn plain_jackpot_from_strips(
    h: usize,
    w: usize,
    k: usize,
    r: usize,
    a_noise_seed: [u8; 32],
    b_noise_seed: [u8; 32],
    a_rows_indices: Vec<usize>,
    b_cols_indices: Vec<usize>,
    s_a: Vec<Vec<i8>>,
    s_b: Vec<Vec<i8>>,
) -> PyResult<Vec<u8>> {
    verify::plain_jackpot_from_strips(
        h, w, k, r, a_noise_seed, b_noise_seed, &a_rows_indices, &b_cols_indices, &s_a, &s_b,
    )
    .map(|hash| hash.to_vec())
    .map_err(value_err)
}

#[pyfunction]
#[pyo3(signature = (m, n, k, block_header, mining_config, signal_range=None, wrong_jackpot_hash=false))]
fn mine(
    m: usize,
    n: usize,
    k: usize,
    block_header: IncompleteBlockHeader,
    mining_config: MiningConfiguration,
    signal_range: Option<(i8, i8)>,
    wrong_jackpot_hash: bool,
) -> PyResult<PlainProof> {
    ffi_mine(
        m,
        n,
        k,
        block_header,
        mining_config,
        signal_range,
        wrong_jackpot_hash,
    )
    .map_err(|e| py_err("Mining failed", e))
}

#[pyfunction]
#[pyo3(signature = (m, n, k, block_header, mining_config, signal_range=None, wrong_jackpot_hash=false))]
fn mine_moe(
    m: usize,
    n: usize,
    k: usize,
    block_header: IncompleteBlockHeader,
    mining_config: MiningConfiguration,
    signal_range: Option<(i8, i8)>,
    wrong_jackpot_hash: bool,
) -> PyResult<PlainProof> {
    // Both `e` and `top_k` are committed in `mining_config` (via its `moe` field), so
    // the caller selects GROUPED_GEMM by passing a config with `moe` set.
    ffi_mine_moe(
        m,
        n,
        k,
        block_header,
        mining_config,
        signal_range,
        wrong_jackpot_hash,
    )
    .map_err(|e| py_err("MoE mining failed", e))
}

// ============================================================================
// V1 ZK Functions
// ============================================================================

type V1CircuitCache = zk_pow::v1::circuit::circuit_utils::CircuitCache;

lazy_static! {
    static ref V1_CIRCUIT_CACHE: Mutex<V1CircuitCache> = Mutex::new(V1CircuitCache::default());
}

fn acquire_v1_cache() -> PyResult<std::sync::MutexGuard<'static, V1CircuitCache>> {
    V1_CIRCUIT_CACHE
        .lock()
        .map_err(|_| py_err("V1 cache poisoned by prior panic", "restart required"))
}

fn v2_header_to_v1(h: &IncompleteBlockHeader) -> v1_proof::IncompleteBlockHeader {
    v1_proof::IncompleteBlockHeader {
        version: h.version,
        prev_block: h.prev_block,
        merkle_root: h.merkle_root,
        timestamp: h.timestamp,
        nbits: h.nbits,
    }
}

fn v2_config_to_v1(cfg: &MiningConfiguration) -> v1_proof::MiningConfiguration {
    v1_proof::MiningConfiguration {
        common_dim: cfg.common_dim,
        rank: cfg.rank,
        mma_type: v1_proof::MMAType::Int7xInt7ToInt32,
        rows_pattern: v1_proof::PeriodicPattern {
            shape: cfg.rows_pattern.shape,
        },
        cols_pattern: v1_proof::PeriodicPattern {
            shape: cfg.cols_pattern.shape,
        },
        reserved: v1_proof::MiningConfiguration::RESERVED_VALUE,
    }
}

#[pyfunction]
fn generate_proof_v1(
    block_header: IncompleteBlockHeader,
    plain_proof: PlainProof,
) -> PyResult<PyProof> {
    let v1_header = v2_header_to_v1(&block_header);
    let mut cache = acquire_v1_cache()?;
    let result = v1_prove::zk_prove_plain_proof(v1_header, &plain_proof, &mut cache, true)
        .map_err(|e| py_err("V1 prove failed", e))?;

    Ok(PyProof {
        public_data: result.public_data.to_vec(),
        proof_data: result.proof_data,
    })
}

#[pyfunction]
fn verify_proof_v1(
    block_header: IncompleteBlockHeader,
    proof: &PyProof,
) -> PyResult<(bool, String)> {
    if proof.public_data.len() != v1_proof::PublicProofParams::PUBLICDATA_SIZE {
        return Err(pyo3::exceptions::PyValueError::new_err(format!(
            "V1 public_data must be exactly {} bytes",
            v1_proof::PublicProofParams::PUBLICDATA_SIZE
        )));
    }

    let v1_header = v2_header_to_v1(&block_header);
    let public_data: &[u8; v1_proof::PublicProofParams::PUBLICDATA_SIZE] =
        proof.public_data.as_slice().try_into().unwrap();
    let (params, zk_proof) =
        v1_proof::ZKProof::deserialize(v1_header, public_data, &proof.proof_data)
            .map_err(|e| py_err("V1 deserialize failed", e))?;

    let mut cache = acquire_v1_cache()?;
    match v1_verify::verify_block(&params, &zk_proof, &mut cache) {
        Ok(_) => Ok((true, "Verified".into())),
        Err(e) => Ok((false, format!("Rejected: {}", e))),
    }
}

#[pyfunction]
#[pyo3(signature = (block_header, plain_proof, nbits_override=None))]
fn verify_plain_proof_v1(
    block_header: IncompleteBlockHeader,
    plain_proof: PlainProof,
    nbits_override: Option<u32>,
) -> PyResult<(bool, String)> {
    let v1_header = v2_header_to_v1(&block_header);
    match v1_verify::verify_plain_proof(&v1_header, &plain_proof, nbits_override) {
        Ok(()) => Ok((true, "Mining solution verified successfully".into())),
        Err(e) => Ok((false, e.to_string())),
    }
}

#[pyfunction]
fn warmup_prove_v1(mining_config: MiningConfiguration) -> PyResult<()> {
    let v1_config = v2_config_to_v1(&mining_config);
    let mut cache = acquire_v1_cache()?;
    v1_prove::warmup_prove(v1_config, &mut cache).map_err(|e| py_err("V1 warmup prove failed", e))
}

#[pyfunction]
fn clear_v1_circuit_cache() -> PyResult<()> {
    acquire_v1_cache()?.clear();
    Ok(())
}

// ============================================================================
// Certificate-version dispatchers
//
// One entry point per operation, keyed on the certificate version the node
// reports in getblocktemplate (`requiredcertversion`), so callers cannot pick
// the wrong circuit around the MoE fork crossover.
// ============================================================================

fn value_err(e: impl std::fmt::Display) -> PyErr {
    pyo3::exceptions::PyValueError::new_err(e.to_string())
}

#[pyfunction]
#[pyo3(name = "check_cert_version_eligible")]
fn py_check_cert_version_eligible(cert_version: u32, plain_proof: PlainProof) -> PyResult<()> {
    core_check_cert_version_eligible(cert_version, &plain_proof).map_err(value_err)?;
    Ok(())
}

#[pyfunction]
fn generate_proof_for_cert_version(
    py: Python<'_>,
    cert_version: u32,
    block_header: IncompleteBlockHeader,
    plain_proof: PlainProof,
) -> PyResult<PyProof> {
    match core_check_cert_version_eligible(cert_version, &plain_proof).map_err(value_err)? {
        CertificateVersion::ZkDense => generate_proof_v1(block_header, plain_proof),
        // generate_proof_v2 now takes `py` to release the GIL during the ~18s prove.
        CertificateVersion::ZkMoe => generate_proof_v2(py, block_header, plain_proof),
    }
}

#[pyfunction]
fn verify_proof_for_cert_version(
    py: Python<'_>,
    cert_version: u32,
    block_header: IncompleteBlockHeader,
    proof: &PyProof,
) -> PyResult<(bool, String)> {
    // No PlainProof here, so only the version itself can be validated.
    match CertificateVersion::try_from(cert_version).map_err(value_err)? {
        CertificateVersion::ZkDense => verify_proof_v1(block_header, proof),
        // verify_proof_v2 now takes `py` to release the GIL during the heavy block verify.
        CertificateVersion::ZkMoe => verify_proof_v2(py, block_header, proof),
    }
}

#[pyfunction]
#[pyo3(signature = (cert_version, block_header, plain_proof, nbits_override=None))]
fn verify_plain_proof_for_cert_version(
    py: Python<'_>,
    cert_version: u32,
    block_header: IncompleteBlockHeader,
    plain_proof: PlainProof,
    nbits_override: Option<u32>,
) -> PyResult<(bool, String)> {
    match core_check_cert_version_eligible(cert_version, &plain_proof).map_err(value_err)? {
        CertificateVersion::ZkDense => {
            verify_plain_proof_v1(block_header, plain_proof, nbits_override)
        }
        // verify_plain_proof_v2 now takes `py` to release the GIL during the per-share verify.
        CertificateVersion::ZkMoe => {
            verify_plain_proof_v2(py, block_header, plain_proof, nbits_override)
        }
    }
}

// ============================================================================
// Module
// ============================================================================

const DEFAULT_RAYON_THREADS: usize = 6;

fn rayon_thread_count() -> usize {
    std::env::var("RAYON_NUM_THREADS")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(DEFAULT_RAYON_THREADS)
}

#[pymodule]
fn pearl_mining(m: &Bound<'_, pyo3::types::PyModule>) -> PyResult<()> {
    let _ = env_logger::try_init();
    m.add("__version__", env!("CARGO_PKG_VERSION"))?;
    rayon::ThreadPoolBuilder::new()
        .num_threads(rayon_thread_count())
        .build_global()
        .expect("Failed to initialize rayon global thread pool");
    m.add("MERKLE_LEAF_SIZE", CHUNK_LEN)?;
    m.add("PUBLICDATA_SIZE", PublicProofParams::WIRE_SIZE)?;
    m.add(
        "MIN_MOE_PUBLICDATA_SIZE",
        PublicProofParams::MIN_MOE_WIRE_SIZE,
    )?;
    m.add("PUBLICDATA_MAX_SIZE", PublicProofParams::MAX_WIRE_SIZE)?;
    m.add_class::<MerkleTree>()?;
    m.add_class::<MerkleProof>()?;
    m.add_class::<PeriodicPattern>()?;
    m.add_class::<IncompleteBlockHeader>()?;
    m.add_class::<MiningConfiguration>()?;
    m.add_class::<MoEConfig>()?;
    m.add_class::<MMAType>()?;
    m.add_class::<MatrixMerkleProof>()?;
    m.add_class::<PlainProof>()?;
    m.add_class::<MoEProofParams>()?;
    m.add_class::<PyProof>()?;
    m.add_function(wrap_pyfunction!(mine, m)?)?;
    m.add_function(wrap_pyfunction!(mine_moe, m)?)?;
    m.add_function(wrap_pyfunction!(py_pad_to_chunk_boundary, m)?)?;
    // V2 functions (current circuit; MoE and dense proofs)
    m.add_function(wrap_pyfunction!(generate_proof_v2, m)?)?;
    m.add_function(wrap_pyfunction!(verify_proof_v2, m)?)?;
    m.add_function(wrap_pyfunction!(verify_proof_v2_with_nbits, m)?)?;
    m.add_function(wrap_pyfunction!(verify_plain_proof_v2, m)?)?;
    m.add_function(wrap_pyfunction!(verify_plain_proof_jackpot_v2, m)?)?;
    m.add_function(wrap_pyfunction!(plain_jackpot_from_strips, m)?)?;
    m.add_function(wrap_pyfunction!(clear_circuit_cache_v2, m)?)?;
    m.add_function(wrap_pyfunction!(warmup_prove_v2, m)?)?;
    // V1 functions (legacy circuit; dense proofs only)
    m.add(
        "V1_PUBLICDATA_SIZE",
        v1_proof::PublicProofParams::PUBLICDATA_SIZE,
    )?;
    m.add_function(wrap_pyfunction!(generate_proof_v1, m)?)?;
    m.add_function(wrap_pyfunction!(verify_proof_v1, m)?)?;
    m.add_function(wrap_pyfunction!(verify_plain_proof_v1, m)?)?;
    m.add_function(wrap_pyfunction!(warmup_prove_v1, m)?)?;
    m.add_function(wrap_pyfunction!(clear_v1_circuit_cache, m)?)?;
    // Certificate-version dispatchers (recommended entry points)
    m.add("CERT_VERSION_ZK_DENSE", CertificateVersion::ZkDense as u32)?;
    m.add("CERT_VERSION_ZK_MOE", CertificateVersion::ZkMoe as u32)?;
    m.add_function(wrap_pyfunction!(py_check_cert_version_eligible, m)?)?;
    m.add_function(wrap_pyfunction!(generate_proof_for_cert_version, m)?)?;
    m.add_function(wrap_pyfunction!(verify_proof_for_cert_version, m)?)?;
    m.add_function(wrap_pyfunction!(verify_plain_proof_for_cert_version, m)?)?;
    Ok(())
}
