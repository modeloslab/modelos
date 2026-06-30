# Changelog

All notable changes to modelOS (node, wallet, SPV/light client, and the ZK
proof-of-useful-work layer) are documented here.

## [1.0.7] — 2026-06-30

Reliability + merged-mining (AuxPoW) hardening release. No consensus change to
how existing blocks validate; existing V1 aux pools and miners are unaffected.

### Node (modelosd)

- **AuxPoW header decode — fresh-node / SPV sync fix.** The shared `HEADERS`
  wire decoder rejected any batch that mixed certified (native) and uncertified
  (AuxPoW, null-cert) headers. On an AuxPoW chain those are interleaved per
  block, so every real batch spanning both kinds was rejected — wedging any
  from-scratch syncer at the first AuxPoW↔native boundary. The decoder now
  enforces only the single combination consensus rejects (a non-AuxPoW header
  with no certificate, `ErrCertificateMissing`) — never stricter, so it cannot
  reject a header the chain would accept, and never looser. Block/proof
  validation is unchanged; only the wire decode is corrected.

- **AuxPoW commitment — V1 is the canonical contract.** The embedded Pearl
  parent header's proof commitment is validated strictly as
  `SHA256d(LE32(1) || PublicData)`. On a post-MoE (V2) Pearl chain, aux pools
  must normalize the embedded header's commitment to this V1 form before
  `submitauxblock` (the committed `PublicData` is byte-identical to V1, so this
  rewrites only the 32-byte commitment field and is sound — the real Pearl block
  sent to `pearld` keeps its native V2 commitment). Accepting both V1 and V2 was
  evaluated and rejected: it would let one proof mint two distinct same-height
  blocks (malleability). Native-V2 acceptance is deferred to a future
  coordinated hard fork.

### ZK proof-of-useful-work (verifier)

- **Cache robustness — no more poison cascade.** The Go-FFI circuit cache was a
  `lazy_static` that `.expect()`-panicked on a bad embedded cache, poisoning the
  init `Once` so every subsequent verification re-panicked with the cryptic
  "Once instance has previously been poisoned" — permanently wedging the verifier
  and masking the real cause. It is now a non-poisoning `OnceLock` with a
  fallible accessor that returns a clear, actionable error ("regenerate the
  cache") on every call, and reports a system-error code (2) distinct from a
  genuinely rejected proof (1). Both reject the block (fail-closed).

- **Cache build fix — embed what we build.** Every build path generated the
  cache to a dead `cache.bin` while the binary embeds `v2_cache.bin` /
  `v1_cache.bin` via `include_bytes!` — so the regeneration was a silent no-op
  and the embedded cache could drift from the circuit ("Invalid cache magic
  bytes or version mismatch"). All build paths (`build_cache` in the Dockerfile,
  both Taskfiles, and CI) now write the exact embedded filenames, so a release
  binary's embedded cache always matches the circuit it was compiled with.

- **Cache runtime override (opt-in).** `MODELOS_ZK_CACHE_DIR`, when set, loads
  `v2_cache.bin` / `v1_cache.bin` from that directory before falling back to the
  embedded cache — an operator escape hatch to drop in a published cache without
  rebuilding. Off by default; never read from a default path.

### SPV / light client (desktop wallet backend)

- **Sync stall handler.** The light client had no stall detection, so a sync
  peer stuck on a non-canonical fork (or advertising a height it could not
  serve) was never rotated away and sync stalled. It now samples header-tip
  progress and, after no progress while still behind, cools the peer down for 10
  minutes and rotates to another peer. Chain selection (most-work + checkpoints)
  is unchanged — only which peer drives sync.

- **Mainnet checkpoints.** Added verified checkpoints (heights 2000–17000, taken
  from the live mainnet best chain). A light client cannot be fed a forged chain
  below the latest checkpoint, and the node rejects any reorg that rewrites a
  checkpointed block.

- **Assume-valid proof verification.** The light client skips ZK-proof
  verification at/below the latest checkpoint (ancestry already pinned) and fully
  verifies every proof above it — fast initial sync with trustless recent blocks.

### Wallet (oyster)

- **Single-address model.** Change outputs and the receive address are locked to
  the account's primary address (external index 0, `m/86'/coin'/account'/0/0`)
  instead of rotating to a fresh index / internal change branch. The desktop
  wallet and the single-address compute wallet now present the same address for a
  phrase and agree on balance. (Address reuse is an accepted trade-off for a
  consistent, low-confusion UX.)

### Desktop app

- Version bumped to 1.0.7.

### Tests / build

- Fixed rebrand-stale test fixtures (mainnet network magic `MDLM`, the
  `/modeloswire/` user-agent size, and `mdl1…` Taproot addresses) that predated
  the Pearl→modelOS rename. These update expectations to the correct current
  values; no behavior change.
