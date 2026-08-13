//! Consensus parity for the V3 salted noise-seed derivation (upstream Pearl fc5ca65a1).
//!
//! These vectors are pinned in upstream's `zk-pow/src/api/seed.rs` and were independently
//! computed with the Python `blake3` package there. If any assert here fails, the seeds we
//! derive differ from Pearl's and every PRL block we mine is rejected (and every Pearl block
//! we verify is judged invalid). This lives in `tests/` rather than a `#[cfg(test)]` module
//! so it builds without the in-crate compat fixtures, which are absent from this fork.

use zk_pow::api::proof::{
    IncompleteBlockHeader, MMAType, MiningConfiguration, MoEParams, PeriodicPattern, PublicProofParams, SeedDerivation,
};

/// Mirrors upstream's `PublicProofParams::new_for_tests(192, 320, 256)`, which is `#[cfg(test)]`
/// and therefore not reachable from an integration test.
fn params_for_tests(m: u32, n: u32, k: u32, seed_derivation: SeedDerivation) -> PublicProofParams {
    let header = IncompleteBlockHeader {
        version: 0,
        prev_block: [0; 32],
        merkle_root: [0; 32],
        timestamp: 0,
        nbits: 0x207FFFFF,
    };
    let config = MiningConfiguration {
        common_dim: k,
        rank: 128,
        mma_type: MMAType::Int7xInt7ToInt32,
        rows_pattern: PeriodicPattern::from_list(&[0, 8, 64, 72]).unwrap(),
        cols_pattern: PeriodicPattern::from_list(&[0, 1, 8, 9, 32, 33, 40, 41, 64, 65, 72, 73, 96, 97, 104, 105]).unwrap(),
        moe: None,
    };
    let mut p = PublicProofParams::new_dummy(header, seed_derivation, config, m, n, 0, 0);
    p.hash_a = [0xAA; 32];
    p.hash_b = [0xBB; 32];
    p
}

#[test]
fn commitment_hash_matches_upstream_pinned_vectors() {
    let job_key = [0x11u8; 32];

    let (b, a) = params_for_tests(192, 320, 256, SeedDerivation::Legacy).commitment_hash(job_key);
    assert_eq!(
        b,
        [
            0xad, 0xd6, 0xf7, 0xea, 0x5f, 0xee, 0xbf, 0x89, 0xc8, 0xa7, 0x7e, 0x2e, 0xbf, 0xa0, 0xd8, 0x24, 0x42, 0xe7, 0xdb,
            0xb0, 0x04, 0x6d, 0xbd, 0x48, 0x97, 0x18, 0x61, 0xd1, 0x2f, 0xcb, 0x01, 0x77,
        ],
        "legacy b_noise_seed drifted — consensus break on pre-V3 blocks"
    );
    assert_eq!(
        a,
        [
            0x48, 0x3b, 0x07, 0xb6, 0xf7, 0x31, 0x05, 0x03, 0x0b, 0x94, 0x82, 0x25, 0x5f, 0x37, 0x72, 0x3f, 0x3f, 0xed, 0x69,
            0xae, 0x91, 0x67, 0x24, 0xee, 0x82, 0x91, 0x84, 0x8b, 0x8c, 0x28, 0x79, 0x4b,
        ],
        "legacy a_noise_seed drifted — consensus break on pre-V3 blocks"
    );

    let (b, a) = params_for_tests(192, 320, 256, SeedDerivation::Salted).commitment_hash(job_key);
    assert_eq!(
        b,
        [
            0x60, 0xed, 0x9b, 0x73, 0xc5, 0xa9, 0x59, 0x9b, 0x20, 0x0b, 0x6c, 0xd5, 0x63, 0xe7, 0xf0, 0xd5, 0xd9, 0xa6, 0x7d,
            0x24, 0x02, 0xd8, 0x5f, 0xd4, 0xef, 0x96, 0x6c, 0x58, 0x00, 0x80, 0xd0, 0xe5,
        ],
        "salted b_noise_seed drifted — PRL blocks we mine will be rejected"
    );
    assert_eq!(
        a,
        [
            0x30, 0x17, 0x84, 0x16, 0x80, 0x05, 0xec, 0x83, 0x3a, 0xb0, 0xaa, 0x60, 0x00, 0x6f, 0x7f, 0xe7, 0xfa, 0xaa, 0x95,
            0x30, 0x7d, 0x8c, 0x1f, 0xc6, 0x81, 0x9b, 0x2f, 0xfd, 0xd7, 0x17, 0xec, 0xcf,
        ],
        "salted a_noise_seed drifted — PRL blocks we mine will be rejected"
    );
}

#[test]
fn commitment_hash_matches_upstream_pinned_vector_salted_moe() {
    let mut p = params_for_tests(192, 320, 256, SeedDerivation::Salted);
    p.moe = Some(MoEParams {
        expert_idx: 0,
        routing_offsets: vec![3, 5, 9, 12],
        hash_routing: [0xCC; 32],
        outer_indices: vec![],
    });

    let (b, a) = p.commitment_hash([0x11u8; 32]);
    // B's seed does not involve the routing fold, so it equals the dense salted vector.
    assert_eq!(
        b,
        [
            0x60, 0xed, 0x9b, 0x73, 0xc5, 0xa9, 0x59, 0x9b, 0x20, 0x0b, 0x6c, 0xd5, 0x63, 0xe7, 0xf0, 0xd5, 0xd9, 0xa6, 0x7d,
            0x24, 0x02, 0xd8, 0x5f, 0xd4, 0xef, 0x96, 0x6c, 0x58, 0x00, 0x80, 0xd0, 0xe5,
        ],
        "salted MoE b_noise_seed drifted"
    );
    assert_eq!(
        a,
        [
            0x26, 0x8a, 0x2e, 0xb7, 0x8b, 0x3b, 0x2d, 0x2f, 0xea, 0xd2, 0x62, 0x22, 0x1b, 0x24, 0xc6, 0x34, 0x6d, 0x0a, 0x6d,
            0x20, 0x18, 0x90, 0xa4, 0x67, 0x75, 0x1f, 0x62, 0x3b, 0x7e, 0xac, 0xd5, 0xc2,
        ],
        "salted MoE a_noise_seed drifted — A must be salted BEFORE the routing fold"
    );
}

#[test]
fn salts_match_their_context_strings() {
    use zk_pow::api::seed::{SEED_SALT_A, SEED_SALT_B};
    assert_eq!(SEED_SALT_A, pearl_blake3::blake3_digest(b"pearl/cert-v3/noise-seed/A", None));
    assert_eq!(SEED_SALT_B, pearl_blake3::blake3_digest(b"pearl/cert-v3/noise-seed/B", None));
}
