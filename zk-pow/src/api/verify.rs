use anyhow::{Result, bail, ensure};
use plonky2_field::extension::FieldExtension;
use plonky2_field::goldilocks_field::GoldilocksField;

use crate::{
    api::{
        proof::{IncompleteBlockHeader, PublicProofParams, SeedDerivation, ZKProof},
        proof_utils::{CompiledPublicParams, compute_jackpot_hash, hash_to_u32_field_array},
        sanity_checks::check_jackpot_against_nbits,
    },
    circuit::{
        chip::{compute_jackpot, compute_jackpot_dims},
        circuit_utils::CircuitCache,
        pearl_circuit::{PearlCircuitParams, PearlRecursion, PearlVerifierPIs, RecursionCircuit},
        pearl_noise::{compute_noise, compute_noise_for_indices},
        pearl_stark::PearlStark,
    },
    ffi::plain_proof::PlainProof,
};

/// Verifies a block proof, compiling circuits into cache if needed.
pub fn verify_block(public_params: &PublicProofParams, proof: &ZKProof, cache: &mut CircuitCache) -> Result<()> {
    verify_block_with_nbits(public_params, proof, cache, None)
}

/// Build-on-miss verify with an optional nbits override (the override counterpart to
/// `verify_block`). Compiles the recursive verifier circuit on a cold cache instead of
/// bailing "verifier circuit not found in cache" — so the merged-mining pool can verify a
/// Pearl proof against the easier modelOS target WITHOUT a pre-warmed cache. NON-consensus
/// only (the pool's submit_block_proof safety gate); the node's Go FFI stays cached-only so
/// block acceptance never triggers an unbounded build.
pub fn verify_block_with_nbits(
    public_params: &PublicProofParams,
    proof: &ZKProof,
    cache: &mut CircuitCache,
    nbits_override: Option<u32>,
) -> Result<()> {
    let (params, pis) = prepare_verification(public_params, proof, nbits_override)?;
    PearlRecursion::compile_circuits(params, cache, false)?;
    verify_with_cache(params, cache, &pis, proof)
}

/// Verifies a block proof using pre-compiled circuits. Fails proof if verifying circuit not in cache.
/// nbits_override: if provided, overrides the nbits of the block header.
pub fn verify_block_cached_circuits_only(
    public_params: &PublicProofParams,
    proof: &ZKProof,
    cache: &CircuitCache,
    nbits_override: Option<u32>,
) -> Result<()> {
    let (params, pis) = prepare_verification(public_params, proof, nbits_override)?;
    verify_with_cache(params, cache, &pis, proof)
}

fn verify_with_cache(params: PearlCircuitParams, cache: &CircuitCache, pis: &PearlVerifierPIs, proof: &ZKProof) -> Result<()> {
    if PearlRecursion::verify(params, cache, pis.clone(), &proof.plonky2_proof)? {
        Ok(())
    } else {
        bail!("Proof Invalid")
    }
}

fn prepare_verification(
    public_params: &PublicProofParams,
    proof: &ZKProof,
    nbits_override: Option<u32>,
) -> Result<(PearlCircuitParams, PearlVerifierPIs)> {
    public_params.sanity_check()?;
    check_jackpot_against_nbits(public_params, nbits_override)?;

    let compiled_params = CompiledPublicParams::from(public_params);
    let circuit_params = PearlCircuitParams {
        stark_degree_bits: compiled_params.degree_bits(),
        pow_bits: proof.pow_bits.map(|b| b as usize),
        rate_bits: proof.rate_bits.map(|b| b as usize),
    };
    circuit_params.sanity_check(&compiled_params)?;

    let pis = build_verifier_pis(public_params, &compiled_params, &circuit_params, proof)?;
    Ok((circuit_params, pis))
}

fn build_verifier_pis(
    public_params: &PublicProofParams,
    compiled_params: &CompiledPublicParams,
    circuit_params: &PearlCircuitParams,
    proof: &ZKProof,
) -> Result<PearlVerifierPIs> {
    let (_, commitment_hash) = compiled_params.commitment_hash;
    let zeta = proof.zeta()?;
    ensure!(
        !FieldExtension::<2>::is_in_basefield(&zeta),
        "zeta must lie strictly in the extension field (b component is zero)"
    );
    Ok(PearlVerifierPIs {
        job_key: hash_to_u32_field_array(&compiled_params.job_key),
        commitment_hash: hash_to_u32_field_array(&commitment_hash),
        hash_a: hash_to_u32_field_array(&public_params.hash_a()),
        hash_b: hash_to_u32_field_array(&public_params.hash_b()),
        hash_routing: public_params
            .moe
            .as_ref()
            .map(|moe| hash_to_u32_field_array(&moe.hash_routing)),
        hash_jackpot: hash_to_u32_field_array(&public_params.hash_jackpot()),
        public_data_commitment: public_params.public_data_commitment(circuit_params)?,
        zeta: proof.zeta()?,
        preprocessed_columns: PearlStark::<GoldilocksField, 2>::preprocessed_columns(compiled_params)?,
    })
}

/// Verifies a plain proof (mining solution, whether MoE or not) without generating a ZK proof.
/// Returns `Ok(())` if valid, `Err(message)` if invalid.
///
/// `nbits_override`: when set (e.g. pool share difficulty from `mining.set_difficulty`),
/// jackpot difficulty is checked against this compact target instead of `block_header.nbits`.
pub fn verify_plain_proof(
    block_header: &IncompleteBlockHeader,
    plain_proof: &PlainProof,
    nbits_override: Option<u32>,
    seed_derivation: SeedDerivation,
) -> Result<()> {
    // Parse the plain proof to get private and public params
    let (private_params, mut public_params) = plain_proof.parse_proof(*block_header, seed_derivation)?;

    // Perform public params sanity check
    public_params.sanity_check()?;

    // Verify all strip values are in [-64, 64] (matching the ZK circuit's IRANGE7P1 range check)
    for strip in private_params.s_a.iter().chain(private_params.s_b.iter()) {
        for &val in strip {
            ensure!((-64..=64).contains(&val), "Matrix value {} out of range [-64, 64]", val);
        }
    }

    // Create CompiledPublicParams to compute noise
    let compiled = CompiledPublicParams::from(&public_params);

    // Compute noise matrices from commitment hash
    let noise = compute_noise(&compiled);

    // Compute the jackpot message (strips + noise -> msg)
    let jackpot = compute_jackpot(&compiled, &private_params.s_a, &private_params.s_b, &noise);

    // Compute the actual jackpot hash and check the difficulty condition
    public_params.hash_jackpot = compute_jackpot_hash(&jackpot, compiled.a_noise_seed());
    check_jackpot_against_nbits(&public_params, nbits_override)?;

    Ok(())
}

/// Recompute the 32-byte jackpot hash from a plain proof (witness) WITHOUT generating
/// a ZK proof and WITHOUT applying any difficulty check.
///
/// Errors on an invalid witness (bad merkle openings, out-of-range strips, sanity
/// failure) so a forged witness is rejected. The returned hash is byte-identical to
/// the jackpot a real ZK block proof commits to, so the pool can grade one witness
/// against several targets (share / block / pearl) cheaply, then prove only winners.
pub fn verify_plain_proof_jackpot(
    block_header: &IncompleteBlockHeader,
    plain_proof: &PlainProof,
    seed_derivation: SeedDerivation,
) -> Result<[u8; 32]> {
    let (private_params, mut public_params) = plain_proof.parse_proof(*block_header, seed_derivation)?;
    public_params.sanity_check()?;
    for strip in private_params.s_a.iter().chain(private_params.s_b.iter()) {
        for &val in strip {
            ensure!((-64..=64).contains(&val), "Matrix value {} out of range [-64, 64]", val);
        }
    }
    let compiled = CompiledPublicParams::from(&public_params);
    let noise = compute_noise(&compiled);
    let jackpot = compute_jackpot(&compiled, &private_params.s_a, &private_params.s_b, &noise);
    public_params.hash_jackpot = compute_jackpot_hash(&jackpot, compiled.a_noise_seed());
    Ok(public_params.hash_jackpot)
}

/// FAST jackpot for pool block-SCREENING: byte-identical to `verify_plain_proof_jackpot` for an HONEST
/// witness, but uses `parse_proof_unverified` so it SKIPS the BLAKE3 Merkle membership recompute
/// (`evaluate_blake` — the ~190ms cost). The compute path (noise → jackpot → hash) is IDENTICAL, so an
/// honest witness yields the exact same jackpot; a forged witness could supply arbitrary strips, so
/// this MUST NOT be a sole consensus check — the pool runs the FULL `verify_plain_proof_jackpot`/`_v2`
/// (membership) before proving/submitting a block, plus a random anti-cheat sample of regular shares.
pub fn verify_plain_proof_jackpot_fast(
    block_header: &IncompleteBlockHeader,
    plain_proof: &PlainProof,
    seed_derivation: SeedDerivation,
) -> Result<[u8; 32]> {
    let (private_params, mut public_params) = plain_proof.parse_proof_unverified(*block_header, seed_derivation)?;
    public_params.sanity_check()?;
    for strip in private_params.s_a.iter().chain(private_params.s_b.iter()) {
        for &val in strip {
            ensure!((-64..=64).contains(&val), "Matrix value {} out of range [-64, 64]", val);
        }
    }
    let compiled = CompiledPublicParams::from(&public_params);
    let noise = compute_noise(&compiled);
    let jackpot = compute_jackpot(&compiled, &private_params.s_a, &private_params.s_b, &noise);
    public_params.hash_jackpot = compute_jackpot_hash(&jackpot, compiled.a_noise_seed());
    Ok(public_params.hash_jackpot)
}

/// CHEAP jackpot recompute from RAW opened strips — for a host-side miner pre-filter, NOT a
/// consensus path. Recomputes the byte-identical 32-byte jackpot hash that `verify_plain_proof_jackpot`
/// produces, but takes the opened A-rows / B-cols and noise inputs DIRECTLY instead of parsing a
/// `PlainProof`, so it skips the multi-hundred-MB Merkle-tree build that `create_proof` does.
///
/// This lets the standalone miner drop the kernel's "over-signals" (tiles the GPU flags whose real
/// jackpot is far above the share target) BEFORE spending the Merkle build on them — the build storm
/// that saturates the witness pool at the vardiff cap. It performs NO Merkle/membership verification
/// (the caller supplies strips it reconstructed itself from its own A/W), so it must never be used to
/// grade an untrusted submission — only to decide whether the miner bothers building the full witness.
///
/// Inputs (all already held by the miner):
/// - `h`/`w`: jackpot tile dims = `s_a.len()` / `s_b.len()` (rows of A / cols of B^t opened).
/// - `k`: common dimension (matmul k); `r`: noise rank.
/// - `a_noise_seed`/`b_noise_seed`: the commitment hashes (`noise_seed_A`, `noise_seed_B`).
/// - `a_rows_indices`/`b_cols_indices`: the opened row/col positions (drive the per-row/col noise).
/// - `s_a`/`s_b`: the opened strip values (each length `k`).
///
/// Composes the SAME `compute_noise_for_indices` → `compute_jackpot_dims` → `compute_jackpot_hash`
/// the verifier runs, so the result is byte-identical (asserted by `plain_jackpot_from_strips_matches_full_proof`).
#[allow(clippy::too_many_arguments)]
pub fn plain_jackpot_from_strips(
    h: usize,
    w: usize,
    k: usize,
    r: usize,
    a_noise_seed: [u8; 32],
    b_noise_seed: [u8; 32],
    a_rows_indices: &[usize],
    b_cols_indices: &[usize],
    s_a: &[Vec<i8>],
    s_b: &[Vec<i8>],
) -> Result<[u8; 32]> {
    ensure!(s_a.len() == h, "s_a has {} rows, expected h={}", s_a.len(), h);
    ensure!(s_b.len() == w, "s_b has {} cols, expected w={}", s_b.len(), w);
    ensure!(
        a_rows_indices.len() == h && b_cols_indices.len() == w,
        "index counts must match (h, w)"
    );
    // Same range check the verifier applies to the strips (ZK circuit IRANGE7P1).
    for strip in s_a.iter().chain(s_b.iter()) {
        ensure!(strip.len() == k, "strip length {} != k={}", strip.len(), k);
        for &val in strip {
            ensure!((-64..=64).contains(&val), "Matrix value {} out of range [-64, 64]", val);
        }
    }
    // compute_noise expects commitment_hash = (b_noise_seed, a_noise_seed) — same order as
    // CompiledPublicParams::commitment_hash (b first), and compute_jackpot_hash keys on a_noise_seed.
    let noise = compute_noise_for_indices(k, r, (b_noise_seed, a_noise_seed), a_rows_indices, b_cols_indices);
    let jackpot = compute_jackpot_dims(h, w, k, r, s_a, s_b, &noise);
    Ok(compute_jackpot_hash(&jackpot, a_noise_seed))
}

#[cfg(test)]
mod plain_jackpot_strips_tests {
    use super::*;
    use crate::api::proof::PublicProofParams;
    use crate::api::proof_utils::CompiledPublicParams;
    use crate::circuit::chip::compute_jackpot;
    use crate::circuit::pearl_noise::compute_noise;

    // Deterministic strip values in the verifier's legal range [-64, 64].
    fn make_strips(rows: usize, k: usize, salt: i32) -> Vec<Vec<i8>> {
        (0..rows)
            .map(|u| {
                (0..k)
                    .map(|l| (((u as i32 * 131 + l as i32 * 17 + salt) % 129) - 64) as i8)
                    .collect()
            })
            .collect()
    }

    /// The cheap strips-only pre-filter MUST return the byte-identical jackpot hash that the
    /// canonical (Merkle-parsed) `verify_plain_proof_jackpot` path produces. If this fails, the
    /// miner's pre-filter would drop genuine shares (or keep over-signals), so it gates the build.
    #[test]
    fn plain_jackpot_from_strips_matches_full_proof() {
        // k must be a multiple of rank (128); m/n above the pattern maxima (72 / 105).
        let params = PublicProofParams::new_for_tests(128, 128, 256);
        let compiled = CompiledPublicParams::from(&params);

        let h = params.h();
        let w = params.w();
        let k = params.common_dim();
        let r = params.rank();

        let s_a = make_strips(h, k, 0);
        let s_b = make_strips(w, k, 9_999);

        // Canonical path (what the pool/verifier runs).
        let noise = compute_noise(&compiled);
        let jackpot = compute_jackpot(&compiled, &s_a, &s_b, &noise);
        let expected = compute_jackpot_hash(&jackpot, compiled.a_noise_seed());

        // Cheap strips-only path (what the miner pre-filter runs).
        let (b_noise_seed, a_noise_seed) = compiled.commitment_hash;
        let actual = plain_jackpot_from_strips(
            h,
            w,
            k,
            r,
            a_noise_seed,
            b_noise_seed,
            &compiled.a_rows_indices,
            &compiled.b_cols_indices,
            &s_a,
            &s_b,
        )
        .expect("strips jackpot");

        assert_eq!(expected, actual, "strips-only jackpot must equal the canonical jackpot");
    }
}
