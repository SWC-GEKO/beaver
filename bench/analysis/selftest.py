#!/usr/bin/env python3
"""Generates SYNTHETIC results in the real on-disk format and runs analyze.py on them.
Purpose: prove the parsing/metrics/plot pipeline works before spending cloud money.
    python3 analysis/selftest.py            # writes /tmp/bench-selftest/{results,figures}
"""
import gzip, json, random, subprocess, sys
from pathlib import Path

root = Path("/tmp/bench-selftest"); res = root / "results" / "selftest"
random.seed(1)
NS = 1_000_000_000
T0 = 1_790_000_000 * NS


def line(i, key, t0, created, recv, proc, cnt):
    return f"@@@ {i} {key} {t0} {created} {recv} {proc} {cnt} 50.00 10.00 @@@"


def write_run(path, meta, lines, extra=""):
    path.mkdir(parents=True, exist_ok=True)
    (path / "meta.json").write_text(json.dumps(meta))
    with gzip.open(path / "logs.txt.gz", "wt") as f:
        f.write(extra + "\n".join(lines) + "\n")


def exp12():
    rate, keys = 10, 32
    for system, startup in [("beaver", 0.35), ("faas-single", 0.4), ("faas-batch", None), ("dataflow", 95.0)]:
        for dur in (10, 30, 60):
            for rep in (1, 2, 3):
                t0 = T0 + rep * 1000 * NS
                n = dur * rate
                ev = [(i, t0 + int((i + 1) / rate * NS)) for i in range(n)]
                lines, boot = [], ""
                if system == "faas-batch":
                    recv = t0 + dur * NS + int(0.9 * NS)          # one request after the stream, incl. cold start
                    for k, (i, c) in enumerate(ev):
                        lines.append(line(i, i % keys, t0, c, recv, recv + int((k + 1) * 0.0004 * NS), 1))
                    boot = f"BENCH_BOOT {recv - int(0.8 * NS)}\n"
                elif system == "dataflow":
                    for i, c in ev:
                        recv = max(c, t0 + int(startup * NS)) + int(random.uniform(0.002, 0.02) * NS)
                        lines.append(line(i, i % keys, t0, c, recv, recv + int(0.001 * NS), 1))
                else:
                    for i, c in ev:
                        cold = startup * NS if i == 0 else 0
                        recv = c + int(cold) + int(random.uniform(0.0005, 0.003) * NS)
                        proc = recv + int(random.uniform(0.001, 0.008 if system == "faas-single" else 0.002) * NS)
                        lines.append(line(i, i % keys, t0, c, recv, proc, 1))
                    if system == "faas-single":
                        boot = f"BENCH_BOOT {t0 + int(0.1 * NS)}\n"
                random.shuffle(lines)
                meta = dict(system=system, stream_duration=dur, rep=rep, status="ok", t0_ns=t0, sent=n, errors=0,
                            non_2xx=0, mode="x")
                rd = res / "exp12" / system / f"d{dur}" / f"r{rep}"
                write_run(rd, meta, lines, boot)
                (rd / "clock.txt").write_text("lg 0.000020\nbeaver -0.000100\n")


def exp3():
    cap_per_shard = 900
    for shards in (1, 2, 4, 8):
        for n in (2, 4, 16, 64, 256, 1024):
            t0 = T0 + 5000 * NS
            warm, meas, cool = 10, 20, 5
            ms, me = t0 + warm * NS, t0 + (warm + meas) * NS
            offered = n * 10
            thr = min(offered, cap_per_shard * shards * random.uniform(0.95, 1.05))
            lines, mid = [], 0
            sent_meas = int(offered * meas)
            for sec in range(warm + meas + cool + 5):
                m = int(thr) if warm <= sec < warm + meas else int(min(offered, thr))
                for _ in range(m):
                    created = t0 + int((sec - 0.05) * NS)
                    lines.append(line(mid, mid % 4096, t0, created, created + 2_000_000, t0 + sec * NS + random.randint(0, NS - 1), 1)); mid += 1
            lines.append(line(0, 0, t0 - 30 * NS, t0 - 30 * NS, t0 - 30 * NS + 1_000_000, t0 - 30 * NS + 3_000_000, 1))  # priming event, other t0
            meta = dict(shards=shards, streams=n, rep=1, status="ok", t0_ns=t0, measure_start_ns=ms, measure_end_ns=me,
                        sent=int(offered * (warm + meas + cool)), sent_in_measure=sent_meas, errors=0, mode="nats-scale")
            d = res / "exp3" / f"s{shards}" / f"n{n}" / "r1"
            write_run(d, meta, lines)
            (d / "clock.txt").write_text("lg 0.000020\nbeaver -0.000100\n")
            hdr = "procs -----------memory---------- ---swap-- -----io---- -system-- ------cpu----- -----timestamp-----\n r  b   swpd   free   buff  cache   si   so    bi    bo   in   cs us sy id wa st                 UTC\n"
            (d / "beaver_vmstat.txt").write_text(hdr + " 1 0 0 1 1 1 0 0 0 0 1 1 5 5 90 0 0 2026-10-05 10:00:00\n" + " 1 0 0 1 1 1 0 0 0 0 1 1 40 10 50 0 0 2026-10-05 10:00:05\n")
            (d / "loadgen_vmstat.txt").write_text(hdr + " 1 0 0 1 1 1 0 0 0 0 1 1 5 5 90 0 0 2026-10-05 10:00:00\n" + " 1 0 0 1 1 1 0 0 0 0 1 1 10 5 85 0 0 2026-10-05 10:00:05\n")


if __name__ == "__main__":
    import shutil
    shutil.rmtree(root, ignore_errors=True)
    exp12(); exp3()
    here = Path(__file__).parent
    r = subprocess.run([sys.executable, str(here / "analyze.py"), "--results", str(res), "--out", str(root / "figures"), "--check"])
    sys.exit(r.returncode)
