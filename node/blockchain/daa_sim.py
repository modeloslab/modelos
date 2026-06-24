#!/usr/bin/env python3
"""Difficulty-algorithm stability simulation for the modelOS chain.

Compares, over a few thousand blocks under a 10x hashrate step + a 12h stall +
a 10x step down:

  * WTEMA          — deployed Phase-1 algo (1-week half-life)
  * COLOSSUS(old)  — the median+ASERT hybrid that USED to be Phase-2 (float)
  * ASERT          — the integer aserti3-2d now wired in (blockchain/aserti.go)

The ASERT here is a byte-faithful port of calcASERT() in aserti.go (same integer
cubic 2^x, same truncating division), so this script doubles as a spec/regression
reference for the deployed Go DAA.

Run:  python3 node/blockchain/daa_sim.py
"""

import statistics

T        = 194                 # colossusTargetBlockTime  (s/block)
HL_WTEMA = 7 * 24 * 3600       # WTEMAHalfLife = 1 week
HL_ASERT = 144 * 194           # colossusASERTHalflife ≈ 8 h
POW      = 1 << 208            # mainPowLimit ≈ diff-1 target; difficulty = POW//target
ANCHOR_D = 100                 # anchor difficulty (chain settled at H=100)
NB       = 3200
STALL_AT, STALL_GAP = 1700, 12 * 3600   # one 12h dead gap


def hashrate(n):               # settle@100 -> 10x up -> stall -> 10x down
    if n < 700:  return 100.0
    if n < 2400: return 1000.0
    return 100.0


def solve(D, H):               # deterministic mean solve time (Poisson mean)
    return T * D / H


def _trunc_div(a, b):          # match Go integer division (truncate toward zero)
    q = abs(a) // b
    return -q if a < 0 else q


def calc_asert(anchor_target, spacing, time_diff, height_diff, pow_limit, half_life):
    """Byte-faithful port of calcASERT() in aserti.go — pure integer aserti3-2d."""
    exponent = _trunc_div((time_diff - spacing * (height_diff + 1)) * 65536, half_life)
    shifts = exponent >> 16            # arithmetic (floor) shift, == Go for int64
    frac = exponent & 0xFFFF           # two's-complement low 16 bits ∈ [0,65536)
    factor = 65536 + ((195766423245049 * frac
                       + 971821376 * frac * frac
                       + 5127 * frac * frac * frac
                       + (1 << 47)) >> 48)
    nxt = anchor_target * factor
    shifts -= 16
    nxt = nxt >> (-shifts) if shifts < 0 else nxt << shifts
    if nxt <= 0:
        return 1
    return min(nxt, pow_limit)


# ── algorithms: return next DIFFICULTY from history [{ts,diff}] ────────────────
def wtema(h):
    if len(h) < 2:
        return h[-1]['diff']
    t = h[-1]['ts'] - h[-2]['ts']
    return max(1.0, h[-1]['diff'] / (1 + (t - T) / HL_WTEMA))


def colossus_old(h):           # the unstable hybrid (float), for comparison
    import math
    n = len(h); actual = h[-1]['ts'] - h[0]['ts']; ideal = n * T
    e = max(-10, min(10, (actual - ideal) / HL_ASERT)); af = 2.0 ** e
    win = min(144, len(h) - 1)
    if win < 20:
        return max(1.0, h[-1]['diff'] / af)
    sts = [max(1, min(7 * 24 * 3600, h[i]['ts'] - h[i - 1]['ts']))
           for i in range(len(h) - 1, len(h) - 1 - win, -1)]
    medratio = statistics.median(sts) / T
    combined = max(0.01, (medratio + af) / 2.0)
    return max(1.0, h[-1]['diff'] / combined)   # newTarget = prevTarget*combined (telescopes)


def asert(h):                  # integer aserti3-2d == aserti.go (anchor = block 0)
    anchor_target = POW // ANCHOR_D
    anchor_ref_ts = -T                          # "parent of the anchor" timestamp
    time_diff   = h[-1]['ts'] - anchor_ref_ts
    height_diff = (len(h) - 1) - 0
    tgt = calc_asert(anchor_target, T, time_diff, height_diff, POW, HL_ASERT)
    return max(1.0, POW / tgt)


def run(fn):
    h = [{'ts': k * T, 'diff': float(ANCHOR_D)} for k in range(250)]   # on-schedule warmup
    traj = []
    for n in range(250, NB):
        D = fn(h); H = hashrate(n)
        st = STALL_GAP if n == STALL_AT else int(round(solve(D, H)))   # integer Unix seconds
        h.append({'ts': h[-1]['ts'] + st, 'diff': D})
        traj.append((n, D, H))
    return traj


def main():
    res = {name: run(fn) for name, fn in
           [("WTEMA", wtema), ("COLOSSUS", colossus_old), ("ASERT", asert)]}
    at = lambda tr, n: next(d for (b, d, H) in tr if b == n)
    win = lambda tr, a, b: [d for (n, d, H) in tr if a <= n < b]

    print(f"{'block':>6} {'H':>6} | {'WTEMA':>10} {'COLOSSUS':>12} {'ASERT':>10}")
    for n in [250, 700, 760, 900, 1200, 1699, 1701, 1800, 2399, 2420, 2700, 3190]:
        print(f"{n:>6} {hashrate(n):>6.0f} | {at(res['WTEMA'],n):>10.2f} "
              f"{at(res['COLOSSUS'],n):>12.2f} {at(res['ASERT'],n):>10.2f}")

    print("\n10x STEP UP (target difficulty = 1000):")
    for name in res:
        w = win(res[name], 700, 1700); peak = max(w)
        settle = statistics.mean(win(res[name], 1500, 1700))
        print(f"  {name:>9}: peak={peak:>10.1f} ({peak/1000*100:>6.0f}% of target)  "
              f"settled≈{settle:>8.1f}  overshoot={max(0,(peak-1000)/1000*100):>6.0f}%")

    print("\n10x STEP DOWN (target difficulty = 100):")
    for name in res:
        w = win(res[name], 2400, 3200); trough = min(w)
        settle = statistics.mean(win(res[name], 3000, 3200))
        print(f"  {name:>9}: trough={trough:>8.2f}  settled≈{settle:>8.1f}  "
              f"undershoot={max(0,(100-trough)/100*100):>5.0f}%")


if __name__ == "__main__":
    main()
