"""
No-pod test harness for verify_plain_proof_jackpot — the cheap share-verification
primitive used by the pool to grade shares WITHOUT generating a plonky2 proof.

Everything here runs on CPU via mine() — no GPU, no pod, zero pod spend.

The gold-standard test (test_recompute_equals_zk_committed_jackpot) proves the cheap
recompute returns byte-for-byte the SAME jackpot the full ZK proof commits to, which
is exactly what the pool relies on: grade the recompute, knowing a real proof would
yield the identical value.
"""

import pearl_mining
import pytest

# Within PublicData: config(0..52) | hash_a(52..84) | hash_b(84..116) | hash_jackpot(116..148) | ...
_JACKPOT_OFFSET = 116
_JACKPOT_LEN = 32

DEFAULT_NBITS = 0x1D2FFFFF
DEFAULT_K = 1024
DEFAULT_RANK = 32
ROWS_PATTERN_LIST = [0, 8, 64, 72]
COLS_PATTERN_LIST = [0, 1, 8, 9, 32, 33, 40, 41]
OUT_OF_RANGE_SIGNAL_RANGE = (-64, 65)  # correct is [-64, 63]


def _header(nbits: int = DEFAULT_NBITS) -> pearl_mining.IncompleteBlockHeader:
    return pearl_mining.IncompleteBlockHeader(
        version=0,
        prev_block=b"\x00" * 32,
        merkle_root=b"0123456789abcdef" * 2,
        timestamp=0x66666666,
        nbits=nbits,
    )


def _config(k: int, rank: int = DEFAULT_RANK) -> pearl_mining.MiningConfiguration:
    return pearl_mining.MiningConfiguration(
        common_dim=k,
        rank=rank,
        mma_type=pearl_mining.MMAType.Int7xInt7ToInt32,
        rows_pattern=pearl_mining.PeriodicPattern.from_list(ROWS_PATTERN_LIST),
        cols_pattern=pearl_mining.PeriodicPattern.from_list(COLS_PATTERN_LIST),
        reserved=pearl_mining.MiningConfiguration.RESERVED,
    )


def _mine(m, n, k, header, rank=DEFAULT_RANK, signal_range=None, wrong_jackpot_hash=False):
    return pearl_mining.mine(
        m, n, k, header, _config(k, rank=rank),
        signal_range=signal_range, wrong_jackpot_hash=wrong_jackpot_hash,
    )


class TestVerifyPlainProofJackpot:
    def test_valid_returns_32_byte_jackpot(self):
        """A valid mined witness yields a 32-byte recomputed jackpot."""
        header = _header()
        pp = _mine(256, 128, DEFAULT_K, header)

        jackpot = pearl_mining.verify_plain_proof_jackpot(header, pp)
        assert isinstance(jackpot, (bytes, bytearray))
        assert len(jackpot) == _JACKPOT_LEN

        # Consistency: the canonical bool verifier accepts it at the mined difficulty,
        # so the recomputed jackpot must clear that target.
        ok, msg = pearl_mining.verify_plain_proof(header, pp)
        assert ok, f"canonical verify_plain_proof rejected a mined witness: {msg}"

    def test_recompute_equals_zk_committed_jackpot(self):
        """GOLD STANDARD: cheap recompute == the jackpot the full ZK proof commits to.

        This is what lets the pool grade a share by recompute and trust it equals
        what a real block proof would produce.
        """
        header = _header()
        pp = _mine(256, 128, DEFAULT_K, header)

        recomputed = bytes(pearl_mining.verify_plain_proof_jackpot(header, pp))

        proof = pearl_mining.generate_proof(header, pp)
        public_data = bytes(proof.public_data)
        committed = public_data[_JACKPOT_OFFSET:_JACKPOT_OFFSET + _JACKPOT_LEN]

        assert recomputed == committed, (
            f"recompute != ZK-committed jackpot\n"
            f"  recomputed={recomputed.hex()}\n"
            f"  committed ={committed.hex()}"
        )

    def test_forged_out_of_range_witness_raises(self):
        """Soundness: a witness with strips outside [-64, 64] must be rejected (raises)."""
        header = _header()
        pp = _mine(256, 128, DEFAULT_K, header, signal_range=OUT_OF_RANGE_SIGNAL_RANGE)

        with pytest.raises(Exception):
            pearl_mining.verify_plain_proof_jackpot(header, pp)

    def test_jackpot_is_deterministic(self):
        """Same witness -> same jackpot (pure recompute, no randomness)."""
        header = _header()
        pp = _mine(256, 128, DEFAULT_K, header)
        j1 = bytes(pearl_mining.verify_plain_proof_jackpot(header, pp))
        j2 = bytes(pearl_mining.verify_plain_proof_jackpot(header, pp))
        assert j1 == j2
