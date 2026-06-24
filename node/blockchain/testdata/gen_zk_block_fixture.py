#!/usr/bin/env python3
"""Generate a ZK-block cross-check fixture from the PRODUCTION pool's coinbase code.

This produces a complete pool-assembled modelOS ZK block (coinbase-only, height 1) so
the Go test `zk_block_crosscheck_test.go` can feed it through the node's REAL block
validation (`checkBlockSanity` + `CheckSerializedHeight` + `ValidateWitnessCommitment`).
It proves the pool-specific bytes the node actually checks line up:

  * the coinbase the pool builds (`modelos_pool.coinbase.build_pool_coinbase`)
  * the merkle root the pool commits into the mined header
  * the BIP34 coinbase height encoding (OP_1 for height 1)
  * the segwit witness commitment (when present)
  * the cert | header | varint | txns wire framing

The plonky2 proof is NOT exercised here (the Go test runs with BFNoPoWCheck): proof
generation/verification is the SAME `generate_proof` the gateway uses to land blocks
today, and `miner/modelos-pool/tests/test_zk_block.py` already pins jackpot@offset 152.
So the certificate carries a structurally-valid but synthetic proof payload — only its
version (1) and size are consensus-checked under BFNoPoWCheck, and both are real.

Run locally (no pearl_mining / no GPU needed):
    /usr/bin/python3 node/blockchain/testdata/gen_zk_block_fixture.py
(needs `bitcoin-utils`: /usr/bin/python3 -m pip install --user 'bitcoin-utils>=0.7.0')
"""
from __future__ import annotations

import hashlib
import json
import os
import struct
import sys

# Import the PRODUCTION pool coinbase builder (pure bitcoinutils, no pearl_mining/torch).
_HERE = os.path.dirname(os.path.abspath(__file__))
_POOL_SRC = os.path.normpath(os.path.join(_HERE, "..", "..", "..", "miner", "modelos-pool", "src"))
sys.path.insert(0, _POOL_SRC)
from modelos_pool.coinbase import build_pool_coinbase  # noqa: E402

# Must match wire/certificate_zk.go PublicDataSize and CertificateVersionZK.
PUBLICDATA_SIZE = 164
ZK_CERT_VERSION = 1
# MainNet PowLimitBits (chaincfg/params.go) — easiest target, so checkProofOfWork's
# target-range check passes; the real PoW (jackpot) is BFNoPoWCheck-skipped.
NBITS = 0x1B00FFFF
HEIGHT = 1
# A real P2TR (bech32m) modelOS address (the pool fee address from .env.example).
POOL_ADDR = "mdl1pq4qnp8x2vg93ra5hfutfc3qn259hc93d8qstsg9ngwseczuxehfqa85t39"
# Fixed past timestamp — context-free sanity only checks it isn't in the future.
TIMESTAMP = 1749427200  # 2025-06-09T00:00:00Z


def _d256(b: bytes) -> bytes:
    return hashlib.sha256(hashlib.sha256(b).digest()).digest()


def _build_block(segwit: bool) -> dict:
    # default_witness_commitment for a COINBASE-ONLY block: the witness merkle root is
    # the coinbase wtxid = zeroHash, and the witness nonce is 32 zero bytes, so the node
    # computes commitment = SHA256d(zeroHash(32) || zeroNonce(32)) = SHA256d(zeros(64)).
    dwc = _d256(b"\x00" * 64).hex() if segwit else None

    template = {
        "height": HEIGHT,
        "coinbasevalue": 50_00000000,   # any in-range amount; value-vs-subsidy is a contextual check
        "coinbaseaux": {},
        "transactions": [],
    }
    if dwc is not None:
        template["default_witness_commitment"] = dwc

    # Use a non-trivial 4-byte extranonce (the per-session extranonce1) so the fixture
    # also exercises the swarm work-splitting path through the node's BIP34 + merkle checks.
    coinbase_bytes, merkle_root_be = build_pool_coinbase(template, POOL_ADDR, b"\x2a\x13\x00\x00")
    merkle_root_le = merkle_root_be[::-1]   # header wire order

    # 76-byte IncompleteBlockHeader: version | prev(32) | merkle(32) | time | nbits
    prev = b"\x00" * 32   # context-free sanity does not check prev; genesis-link is contextual
    incomplete = (
        struct.pack("<I", 1)            # block version
        + prev
        + merkle_root_le
        + struct.pack("<I", TIMESTAMP)
        + struct.pack("<I", NBITS)
    )
    assert len(incomplete) == 76, len(incomplete)

    public_data = b"\x00" * PUBLICDATA_SIZE      # synthetic; only its SIZE is consensus-checked here
    proof_commitment = _d256(struct.pack("<I", ZK_CERT_VERSION) + public_data)
    header108 = incomplete + proof_commitment
    assert len(header108) == 108, len(header108)

    header_hash = _d256(header108)
    proof_data = b"\x00" * 256                   # synthetic plonky2 proof (BFNoPoWCheck skips verify)
    cert = (
        struct.pack("<I", ZK_CERT_VERSION)
        + header_hash
        + public_data
        + struct.pack("<I", len(proof_data))
        + proof_data
    )

    # Block = ZK_CERT | HEADER(108) | varint(tx_count) | TRANSACTIONS  (coinbase first)
    block = cert + header108 + b"\x01" + coinbase_bytes
    return {
        "label": "segwit" if segwit else "legacy",
        "block_hex": block.hex(),
        "height": HEIGHT,
        "bits_hex": f"{NBITS:08x}",
        "segwit": segwit,
        "coinbase_hex": coinbase_bytes.hex(),
        "merkle_root_le_hex": merkle_root_le.hex(),
    }


def main() -> None:
    fixtures = [_build_block(segwit=True), _build_block(segwit=False)]
    out = {"fixtures": fixtures}
    path = os.path.join(_HERE, "zk_block_fixture.json")
    with open(path, "w") as f:
        json.dump(out, f, indent=2)
    for fx in fixtures:
        print(f"{fx['label']:>6}: block {len(fx['block_hex'])//2} bytes, "
              f"merkle_le={fx['merkle_root_le_hex'][:16]}…")
    print("wrote", path)


if __name__ == "__main__":
    main()
