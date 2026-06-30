//! C-compatible structs and utilities for Go FFI.

use anyhow::{Context, Result};
use std::os::raw::c_char;
use std::panic::AssertUnwindSafe;
use std::slice;
use std::sync::{Mutex, OnceLock};

use zk_pow::api::proof::{MiningConfiguration, PublicProofParams};
use zk_pow::circuit::pearl_circuit::{PearlRecursion, RecursionCircuit};

/// Size of reserved field in MiningConfiguration (exported to C header).
pub const MINING_CONFIG_RESERVED_SIZE: usize = 32;

/// Size of serialized MiningConfiguration in bytes (exported to C header).
/// Note: IncompleteBlockHeader (76) + MiningConfiguration (52) = 128 bytes = 2 blake3 blocks.
pub const MINING_CONFIG_SERIALIZED_SIZE: usize = 52;

/// Maximum size of the error message buffer passed from Go (exported to C header).
pub const ERROR_MSG_MAX_SIZE: usize = 128;

/// Maximum size of a serialized ZK proof blob (excluding IncompleteBlockHeader and MiningConfiguration, including everything else).
pub const MAX_ZK_PROOF_SIZE: usize = 60000;

// Compile-time assertions to ensure constants stay in sync
const _: () = assert!(MINING_CONFIG_RESERVED_SIZE == MiningConfiguration::RESERVED_SIZE);
const _: () = assert!(MINING_CONFIG_SERIALIZED_SIZE == MiningConfiguration::SERIALIZED_SIZE);

type CircuitCache = <PearlRecursion as RecursionCircuit>::CircuitCache;
type V1CircuitCache = zk_pow::v1::circuit::circuit_utils::CircuitCache;

// Global circuit caches shared across Go FFI functions (verify and prove), each
// guarded by a Mutex for access from multiple Go goroutines.
//
// IMPORTANT: these are FALLIBLE, NON-POISONING OnceLocks rather than a
// `lazy_static` that `.expect()`s. A `lazy_static` whose initializer panics (e.g.
// the embedded cache has the wrong magic / a version mismatch) poisons the inner
// `Once`, after which EVERY subsequent access re-panics with the cryptic "Once
// instance has previously been poisoned" — permanently wedging the verifier and
// masking the real, actionable cause (regenerate the cache). With OnceLock we only
// store the cache on SUCCESS; a bad cache returns the same clear, actionable error
// on every call, and a later (fixed) build can succeed without a poisoned cascade.
static CIRCUIT_CACHE: OnceLock<Mutex<CircuitCache>> = OnceLock::new();
static V1_CIRCUIT_CACHE: OnceLock<Mutex<V1CircuitCache>> = OnceLock::new();

/// Env var naming a directory that may contain `v2_cache.bin` / `v1_cache.bin`
/// OPERATOR OVERRIDES for the embedded circuit caches.
const CACHE_OVERRIDE_DIR_ENV: &str = "MODELOS_ZK_CACHE_DIR";

/// Load a circuit cache, preferring an OPERATOR-PROVIDED override file over the
/// embedded data. The override is an opt-in escape hatch: if a deployed binary's
/// embedded cache ever mismatches the circuit, an operator can drop a freshly
/// published, trusted `<file>` into `$MODELOS_ZK_CACHE_DIR` and restart — no rebuild
/// required — and this loads it instead of the stale embedded one.
///
/// Resolution order:
///   1. `$MODELOS_ZK_CACHE_DIR/<file>` if the env var is set and the file parses.
///   2. Otherwise the embedded cache.
/// An override that is missing or invalid is logged and IGNORED (we fall back to
/// embedded) — the override can only help, never break a working embedded cache.
///
/// SECURITY: the override is only as trustworthy as the file the operator places
/// there (a malicious cache could make the verifier accept invalid proofs). It is
/// therefore OPT-IN via the env var and never read from a default path; operators
/// must use a trusted, published cache. An attacker able to write this file already
/// has filesystem access to the node and could replace the binary outright, so this
/// does not meaningfully expand the trust boundary.
fn load_cache_with_override<C>(
    label: &str,
    file: &str,
    embedded: &'static [u8],
    parse: impl Fn(&[u8]) -> Result<C>,
) -> Result<C> {
    if let Ok(dir) = std::env::var(CACHE_OVERRIDE_DIR_ENV) {
        if !dir.trim().is_empty() {
            let path = std::path::Path::new(dir.trim()).join(file);
            if path.exists() {
                match std::fs::read(&path)
                    .map_err(anyhow::Error::from)
                    .and_then(|b| parse(&b))
                {
                    Ok(c) => {
                        eprintln!(
                            "zk-pow: using {} circuit-cache OVERRIDE from {}",
                            label,
                            path.display()
                        );
                        return Ok(c);
                    }
                    Err(e) => eprintln!(
                        "zk-pow: {} cache override at {} is invalid ({}); \
                         falling back to the embedded cache",
                        label,
                        path.display(),
                        e
                    ),
                }
            }
        }
    }
    parse(embedded).with_context(|| {
        format!(
            "{label} circuit cache is missing or corrupt; rebuild zk-pow with a freshly \
             generated {file}, or drop a valid {file} into ${CACHE_OVERRIDE_DIR_ENV}"
        )
    })
}

/// Acquires the V2 circuit cache, lazily building it on first use (operator
/// override → embedded). Returns a clean error (never panics/poisons) if no valid
/// cache is available. Recovers from a poisoned Mutex (the cache is read-only, so
/// its contents remain valid even if a prior holder panicked).
pub(crate) fn acquire_cache() -> Result<std::sync::MutexGuard<'static, CircuitCache>> {
    let m = match CIRCUIT_CACHE.get() {
        Some(m) => m,
        None => {
            use zk_pow::circuit::embedded_cache;
            let cache = load_cache_with_override(
                "V2",
                "v2_cache.bin",
                embedded_cache::CACHE_DATA,
                CircuitCache::from_bytes,
            )?;
            // First writer wins; a racing thread's value is dropped — both are valid.
            let _ = CIRCUIT_CACHE.set(Mutex::new(cache));
            CIRCUIT_CACHE.get().expect("cache was just set")
        }
    };
    Ok(m.lock().unwrap_or_else(|poisoned| poisoned.into_inner()))
}

/// Acquires the V1 circuit cache for version-1 proof verification. Same
/// fallible, non-poisoning, override-then-embedded semantics as [`acquire_cache`].
pub(crate) fn acquire_v1_cache() -> Result<std::sync::MutexGuard<'static, V1CircuitCache>> {
    let m = match V1_CIRCUIT_CACHE.get() {
        Some(m) => m,
        None => {
            use zk_pow::v1::embedded_cache;
            let cache = load_cache_with_override(
                "V1",
                "v1_cache.bin",
                embedded_cache::CACHE_DATA,
                V1CircuitCache::from_bytes,
            )?;
            let _ = V1_CIRCUIT_CACHE.set(Mutex::new(cache));
            V1_CIRCUIT_CACHE.get().expect("cache was just set")
        }
    };
    Ok(m.lock().unwrap_or_else(|poisoned| poisoned.into_inner()))
}

/// Catches panics from a closure and returns Ok(result) or Err(panic_message).
/// The closure is wrapped in AssertUnwindSafe internally.
pub(crate) fn catch_panic<F, R>(f: F) -> Result<R>
where
    F: FnOnce() -> R,
{
    std::panic::catch_unwind(AssertUnwindSafe(f)).map_err(|e| {
        let msg = e
            .downcast::<String>()
            .map(|s| *s)
            .or_else(|e| e.downcast::<&str>().map(|s| s.to_string()))
            .unwrap_or_else(|_| "Unknown panic".to_string());
        let first_line = msg.lines().next().unwrap_or(&msg).to_string();
        anyhow::anyhow!(first_line)
    })
}

/// Size of the committed public data in bytes for a standard (non-MoE) ZK proof (exported to C header).
pub const PUBLICDATA_SIZE: usize = 164;
const _: () = assert!(PUBLICDATA_SIZE == PublicProofParams::WIRE_SIZE);

/// Maximum `public_data` buffer length, sized for largest MoE proofs (exported to C header).
pub const PUBLICDATA_MAX_SIZE: usize = 4807;
const _: () = assert!(PUBLICDATA_MAX_SIZE == PublicProofParams::MAX_WIRE_SIZE);

/// Go-owned ZK proof structure. Buffer is sized for the largest MoE proof;
/// `public_data_len` indicates how many bytes are actually used.
#[repr(C)]
pub struct CZKProof {
    pub public_data_len: usize,
    pub public_data: [u8; PUBLICDATA_MAX_SIZE],
    pub proof_blob_len: usize,
    pub proof_blob: *mut u8,
}

/// Writes an error message into a caller-allocated buffer of ERROR_MSG_MAX_SIZE bytes.
/// The message is always null-terminated. Truncation respects UTF-8 char boundaries.
/// # Safety
/// `out` must be null or a valid pointer to a buffer of at least `ERROR_MSG_MAX_SIZE` bytes.
pub(crate) unsafe fn set_error_msg(out: *mut c_char, msg: &str) {
    if out.is_null() {
        return;
    }
    let buf = slice::from_raw_parts_mut(out as *mut u8, ERROR_MSG_MAX_SIZE);
    // Truncate at a UTF-8 char boundary that fits in ERROR_MSG_MAX_SIZE-1 bytes (reserve 1 for null)
    let max_len = ERROR_MSG_MAX_SIZE - 1;
    let mut end = msg.len().min(max_len);
    while end > 0 && !msg.is_char_boundary(end) {
        end -= 1;
    }
    buf[..end].copy_from_slice(&msg.as_bytes()[..end]);
    buf[end] = 0;
}

#[cfg(test)]
mod cache_override_tests {
    use super::{load_cache_with_override, CACHE_OVERRIDE_DIR_ENV};

    // A trivial parser: any non-empty data is "valid" and yields its first byte as
    // the cache identity; empty data is "invalid". Lets us test the resolution
    // order (override vs embedded) without real circuit caches.
    fn parse(b: &[u8]) -> anyhow::Result<u8> {
        if b.is_empty() {
            anyhow::bail!("empty/invalid cache");
        }
        Ok(b[0])
    }

    // NOTE: these touch a process-global env var, so they run in one test to avoid
    // cross-test interference under the default parallel runner.
    #[test]
    fn override_resolution_order() {
        let embedded: &'static [u8] = &[0xEE];

        // 1. No env var set → embedded.
        std::env::remove_var(CACHE_OVERRIDE_DIR_ENV);
        assert_eq!(
            load_cache_with_override("T", "t.bin", embedded, parse).unwrap(),
            0xEE
        );

        let dir = std::env::temp_dir().join(format!("zkcache_test_{}", std::process::id()));
        let _ = std::fs::create_dir_all(&dir);
        std::env::set_var(CACHE_OVERRIDE_DIR_ENV, &dir);

        // 2. Env set but file missing → embedded.
        let _ = std::fs::remove_file(dir.join("t.bin"));
        assert_eq!(
            load_cache_with_override("T", "t.bin", embedded, parse).unwrap(),
            0xEE
        );

        // 3. Env set, file present + valid → override wins.
        std::fs::write(dir.join("t.bin"), [0xAB]).unwrap();
        assert_eq!(
            load_cache_with_override("T", "t.bin", embedded, parse).unwrap(),
            0xAB
        );

        // 4. Env set, file present but INVALID → fall back to embedded (never breaks).
        std::fs::write(dir.join("t.bin"), Vec::<u8>::new()).unwrap();
        assert_eq!(
            load_cache_with_override("T", "t.bin", embedded, parse).unwrap(),
            0xEE
        );

        // 5. If BOTH override and embedded are invalid → clean error (no panic).
        std::fs::write(dir.join("t.bin"), Vec::<u8>::new()).unwrap();
        assert!(load_cache_with_override("T", "t.bin", &[], parse).is_err());

        std::env::remove_var(CACHE_OVERRIDE_DIR_ENV);
        let _ = std::fs::remove_dir_all(&dir);
    }
}
