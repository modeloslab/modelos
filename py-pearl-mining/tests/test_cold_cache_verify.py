"""Regression guard: verify_proof_v2_with_nbits MUST build the recursive circuit on a
COLD cache (build-on-miss), exactly like verify_proof_v2.

This is the AuxPoW merged-mining failure that bit us repeatedly in production: the pool's
submit_block_proof verifies the miner's Pearl proof against the MDL target via
verify_proof_v2_with_nbits, in a thread whose process-global CIRCUIT_CACHE may be cold (a
block proof can arrive before any share witness warms it). When that binding was
cached-circuits-ONLY it bailed:

    Rejected: First verifier circuit not found in cache. Params: FirstCircuitKey { ... }

and EVERY AuxPoW MDL block was lost. The fix routes verify_proof_v2_with_nbits through
verify::verify_block_with_nbits, which compiles the circuit on miss. This test pins that
behavior so it can never silently regress to cached-only.

To GUARANTEE a cold cache regardless of test ordering, the verify runs in a FRESH
subprocess (a new interpreter => a fresh, empty CIRCUIT_CACHE, no warmup). If the binding
is cached-only, the subprocess fails with "not found in cache" and this test fails.

Uses the committed AuxPoW fixture (a real Pearl v2 proof) so no GPU / proving is needed.
"""

import json
import struct
import subprocess
import sys
from pathlib import Path

import pytest

_FIXTURE = (
    Path(__file__).resolve().parents[2]
    / "node" / "blockchain" / "testdata" / "auxpow_verify_fixture.json"
)

# Verify, in a brand-new interpreter (cold cache, NO warmup), that the AuxPoW nbits-override
# path builds-on-miss and returns Verified. Printed markers are asserted by the parent.
_CHILD = r"""
import json, struct, sys
import pearl_mining as pm
fx = json.load(open(sys.argv[1]))
aux = bytes.fromhex(fx["auxpow_hex"]); mdl_bits = int(fx["modelos_bits"], 16)
header = aux[:108]; public_data = aux[144:144+164]
plen = struct.unpack_from("<I", aux, 308)[0]; proof_data = aux[312:312+plen]
hdr = pm.IncompleteBlockHeader.from_bytes(header[:76])
zk  = pm.ZKProof(public_data, proof_data)
# COLD process-global cache, no generate_proof/warmup beforehand — the exact pool scenario.
ok, msg = pm.verify_proof_v2_with_nbits(hdr, zk, mdl_bits)
print("COLD_VERIFY_OK" if ok else "COLD_VERIFY_FAIL", repr(msg))
sys.exit(0 if ok else 1)
"""


@pytest.mark.skipif(not _FIXTURE.exists(), reason="auxpow fixture not generated")
def test_verify_proof_v2_with_nbits_builds_on_cold_cache():
    fx = json.loads(_FIXTURE.read_text())
    assert fx.get("auxpow_hex") and fx.get("modelos_bits"), "fixture missing fields"

    proc = subprocess.run(
        [sys.executable, "-c", _CHILD, str(_FIXTURE)],
        capture_output=True, text=True, timeout=600,
    )
    out = proc.stdout + proc.stderr
    # The exact regression we are guarding against:
    assert "not found in cache" not in out, (
        "verify_proof_v2_with_nbits regressed to cached-circuits-only — it must build the "
        f"circuit on a cold cache (build-on-miss). Child output:\n{out}"
    )
    assert proc.returncode == 0 and "COLD_VERIFY_OK" in out, (
        f"cold-cache AuxPoW verify failed (rc={proc.returncode}):\n{out}"
    )
