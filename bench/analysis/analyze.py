#!/usr/bin/env python3
"""Parse benchmark results, compute the metrics, draw the figures (PDF, for LaTeX).

Data source for every system: the `@@@ messageId key t0 tCreated tReceived tProcessed count avg std @@@`
lines the functions log (Beaver: docker logs, FaaS/Dataflow: Cloud Logging).

Exp. 1+2, per run (t0 = stream start on the load generator, t1 = min(tReceived), t2 = max(tProcessed)):
  theta    = (t1 - t0) / (t2 - t0)                      cold start penalty, in [0,1]
             FaaS-Batched: t0 := max(tCreated) (the batch is only sent after the stream ended; as in the notebook's var_analyze)
  overhead = (t2 - t0) - (max(tCreated) - min(tCreated))   default "notebook": total duration minus the stream's own span
             --overhead-def total    : t2 - t0 literally
             --overhead-def last     : t2 - max(tCreated)
             --overhead-def mean     : mean(tProcessed - tCreated)
Exp. 3, per run, inside the measurement window [ms, me):
  offered    = events sent in the window / window      (x axis, open loop)
  throughput = unique events with tProcessed in window / window
  latency    = tProcessed - tCreated of those events: p50, p99
  error rate = (publish errors + events sent but never processed) / (sent + publish errors)
"""
import argparse, gzip, json, re, sys
from pathlib import Path

import numpy as np
import pandas as pd

sys.path.insert(0, str(Path(__file__).parent))
import plotstyle as ps
import matplotlib.pyplot as plt

EVENT_RE = re.compile(r"@@@ (\d+) (\S+) (\d+) (\d+) (\d+) (\d+) (\d+) (\S+) (\S+) @@@")
CLOCK_WARN_MS = 1.0
BOOT_RE = re.compile(r"BENCH_BOOT (\d+)")



def read_text(p: Path) -> str:
    if not p.exists():
        return ""
    with gzip.open(p, "rt", errors="replace") as f:
        return f.read()


def parse_events(text: str, t0=None) -> pd.DataFrame:
    rows = [m.groups() for m in EVENT_RE.finditer(text)]
    if not rows:
        return pd.DataFrame(columns=["msg", "key", "t0", "created", "received", "processed", "count"])
    df = pd.DataFrame(rows, columns=["msg", "key", "t0", "created", "received", "processed", "count", "avg", "std"])
    for c in ["msg", "t0", "created", "received", "processed", "count"]:
        df[c] = df[c].astype("int64")
    df = df.drop(columns=["avg", "std"])
    # keep only events of THIS run (same t0): drops e.g. the priming event of Exp. 3, which has its own t0
    # and the same message id as the first real event
    if t0 is not None:
        # tolerance: meta.json went through jq, which stores 64-bit ints as doubles (error up to ~256 ns)
        df = df[(df["t0"] - int(t0)).abs() <= 10_000]
    # at-least-once systems may log an event twice: keep the first processing
    return df.sort_values("processed").drop_duplicates("msg", keep="first").sort_values("msg").reset_index(drop=True)


def load_clock(d: Path):
    """clock.txt: 'lg <seconds>' / 'beaver <seconds>' = chrony 'System time' offset (signed). Returns max |lg-beaver| in ms or None."""
    try:
        v = {}
        for ln in (d / "clock.txt").read_text().split("\n"):
            k, _, x = ln.partition(" ")
            if k in ("lg", "beaver"):
                v[k] = float(x)
        if len(v) == 2:
            return abs(v["lg"] - v["beaver"]) * 1e3
    except Exception:
        pass
    return None


def load_meta(path: Path):
    try:
        return json.loads(path.read_text())
    except Exception:
        return None


# --------------------------------------------------------------------------------------------
# Experiments 1 + 2
# --------------------------------------------------------------------------------------------

def exp12_runs(root: Path, overhead_def: str) -> pd.DataFrame:
    out = []
    for mp in sorted(root.glob("exp12/*/d*/r*/meta.json")):
        meta = load_meta(mp)
        if not meta:
            continue
        d = mp.parent
        text = read_text(d / "logs.txt.gz")
        ev = parse_events(text, t0=meta["t0_ns"])
        boots = [int(x) for x in BOOT_RE.findall(text)]
        system = meta["system"]
        t0 = meta["t0_ns"]
        expected = int(meta.get("sent", 0)) - int(meta.get("non_2xx", 0) or 0) - int(meta.get("errors", 0) or 0)
        row = dict(system=system, duration=meta["stream_duration"], rep=meta["rep"], status=meta.get("status"),
                   expected=expected, n_events=len(ev), run_dir=str(d), clock_offset_ms=load_clock(d))
        if system in ("faas-single", "faas-batch"):
            row["cold_verified"] = any(b >= t0 - 1_000_000_000 for b in boots)
        else:
            row["cold_verified"] = True      # Beaver: brand-new containers; Dataflow: brand-new job
        if len(ev):
            t0m = float(t0)                                    # stream start (load generator clock)
            t0_ref = float(ev["created"].max()) if system == "faas-batch" else t0m
            t1 = float(ev["received"].min())
            t2 = float(ev["processed"].max())
            span = float(ev["created"].max() - ev["created"].min())
            lat = (ev["processed"] - ev["created"]) / 1e6
            first = ev.loc[ev["created"].idxmin()]
            row.update(t0_ref_ns=t0_ref, t1_ns=t1, t2_ns=t2,
                       cold_start_penalty_ms=(t1 - t0_ref) / 1e6, total_ms=(t2 - t0m) / 1e6, stream_span_ms=span / 1e6,
                       theta=float(np.clip((t1 - t0_ref) / (t2 - t0_ref), 0, 1)) if t2 > t0_ref else np.nan,
                       first_event_latency_ms=(first["received"] - first["created"]) / 1e6,
                       overhead_notebook_ms=((t2 - t0m) - span) / 1e6, overhead_total_ms=(t2 - t0m) / 1e6,
                       overhead_last_ms=(t2 - float(ev["created"].max())) / 1e6, overhead_mean_ms=lat.mean(),
                       min_recv_minus_created_ms=((ev["received"] - ev["created"]) / 1e6).min())
        row["overhead_ms"] = row.get("overhead_%s_ms" % overhead_def, np.nan)
        out.append(row)
    return pd.DataFrame(out)


def summarize12(df: pd.DataFrame) -> pd.DataFrame:
    if df.empty:
        return df
    good = df[(df["n_events"] > 0)]
    g = good.groupby(["system", "duration"])
    rows = []
    for (s, d), x in g:
        r = dict(system=s, duration=d, reps=len(x), reps_cold_ok=int(x["cold_verified"].sum()))
        for m in ["cold_start_penalty_ms", "theta", "overhead_ms"]:
            r[f"{m}_median"], r[f"{m}_min"], r[f"{m}_max"] = x[m].median(), x[m].min(), x[m].max()
        rows.append(r)
    return pd.DataFrame(rows)


def _errbars(sub, col):
    med = sub[f"{col}_median"].values
    return np.vstack([np.maximum(0, med - sub[f"{col}_min"].values), np.maximum(0, sub[f"{col}_max"].values - med)])


def plot_exp12(summary: pd.DataFrame, out: Path, slide_duration):
    if summary.empty:
        print("no Exp.1/2 data to plot"); return
    durs = sorted(summary["duration"].unique())
    xs = np.arange(len(durs))
    systems = [s for s in ps.SYSTEM_ORDER if s in set(summary["system"])]

    # --- coldstart_penalty.pdf : theta over stream duration (lines)
    fig, ax = plt.subplots(figsize=ps.FIGSIZE_SMALL, constrained_layout=True)
    for sysname in systems:
        sub = summary[summary["system"] == sysname].sort_values("duration")
        x = [durs.index(d) for d in sub["duration"]]
        ax.errorbar(x, sub["theta_median"], yerr=_errbars(sub, "theta"), color=ps.COLOR[sysname], marker=ps.MARKER[sysname],
                    ms=ps.MS_LINE, ls=ps.DASH[sysname], capsize=2, elinewidth=0.7, label=ps.label(sysname),
                    markeredgewidth=0)
    ax.set_ylabel("Cold start penalty"); ax.set_ylim(0, 1)
    ax.set_xlabel("Stream duration [s]"); ax.set_xlim(-0.5, len(durs) - 0.5)
    ax.set_xticks(xs); ax.set_xticklabels([ps.num(d) for d in durs])
    ax.legend(loc="best")
    ps.save(fig, out / "coldstart_penalty.pdf")

    # --- coldstart_duration.pdf : absolute t1 - t0 [ms], grouped log bars (supplement to the theta plot)
    _grouped_bars(summary, durs, systems, "cold_start_penalty_ms", "Cold start duration [ms]", out / "coldstart_duration.pdf")

    # --- overhead.pdf : processing overhead, grouped hatched log bars
    _grouped_bars(summary, durs, systems, "overhead_ms", "Processing overhead [ms]", out / "overhead.pdf")


def _grouped_bars(summary, durs, systems, col, ylabel, path):
    fig, ax = plt.subplots(figsize=ps.FIGSIZE_SMALL, constrained_layout=True)
    n = len(systems); bw = 0.7 / max(n, 1)
    x = np.arange(len(durs))
    vals_all = []
    for i, sysname in enumerate(systems):
        sub = summary[summary["system"] == sysname].set_index("duration").reindex(durs)
        med = sub[f"{col}_median"].values.astype(float)
        err = np.vstack([np.nan_to_num(med - sub[f"{col}_min"].values), np.nan_to_num(sub[f"{col}_max"].values - med)])
        vals_all += [v for v in med if np.isfinite(v) and v > 0]
        ax.bar(x + (i - (n - 1) / 2) * bw, med, width=bw, label=ps.label(sysname), facecolor=ps.COLOR[sysname],
               edgecolor="black", hatch=ps.HATCH[sysname], linewidth=0.8, yerr=err, error_kw=dict(elinewidth=0.6, capsize=1.5))
    ax.set_yscale("log"); ps.log_axis(ax, "y")
    if vals_all:
        lo, hi = min(vals_all), max(vals_all)
        ax.set_ylim(10 ** np.floor(np.log10(lo) - 0.3), 10 ** np.ceil(np.log10(hi) + 1.5))   # headroom for the legend
    ax.set_xticks(x); ax.set_xticklabels([ps.num(d) for d in durs])
    ax.set_xlabel("Stream duration [s]"); ax.set_ylabel(ylabel)
    ax.grid(axis="x", visible=False)
    ax.legend(loc="upper left", ncol=2, columnspacing=0.8, handlelength=1.4)
    ps.save(fig, path)


# --------------------------------------------------------------------------------------------
# Experiment 3
# --------------------------------------------------------------------------------------------

def mean_cpu_busy(path: Path):
    """Mean (100 - idle) from `vmstat -t 5` output, skipping the since-boot first sample."""
    if not path.exists():
        return np.nan
    idx, vals = None, []
    for line in path.read_text(errors="replace").splitlines():
        t = line.split()
        if "id" in t and "us" in t:
            idx = t.index("id"); continue
        if idx is not None and len(t) > idx and all(re.fullmatch(r"-?\d+", x) for x in t[: idx + 1]):
            vals.append(100 - int(t[idx]))
    vals = vals[1:]
    return float(np.mean(vals)) if vals else np.nan


def exp3_runs(root: Path) -> pd.DataFrame:
    out = []
    for mp in sorted(root.glob("exp3/s*/n*/r*/meta.json")):
        meta = load_meta(mp)
        if not meta:
            continue
        d = mp.parent
        ev = parse_events(read_text(d / "logs.txt.gz"), t0=meta["t0_ns"])
        ms, me = meta["measure_start_ns"], meta["measure_end_ns"]
        win = (me - ms) / 1e9
        row = dict(shards=meta["shards"], streams=meta["streams"], rep=meta["rep"], status=meta.get("status"),
                   sent=meta["sent"], publish_errors=meta.get("errors", 0), run_dir=str(d), clock_offset_ms=load_clock(d),
                   offered_rps=meta["sent_in_measure"] / win,
                   beaver_cpu_busy_pct=mean_cpu_busy(d / "beaver_vmstat.txt"),
                   loadgen_cpu_busy_pct=mean_cpu_busy(d / "loadgen_vmstat.txt"))
        if len(ev):
            inw = ev[(ev["processed"] >= ms) & (ev["processed"] < me)]
            lat = (inw["processed"] - inw["created"]) / 1e6
            # per-second throughput: processed events in consecutive 1 s bins of the measurement window
            nb = max(1, int(round(win)))
            bins = np.histogram(inw["processed"].values, bins=nb, range=(ms, me))[0].astype(float) * (nb / win)
            # backlog B(t) = created(<=t) - processed(<=t); growth over the window (events/s). ~0 => sustainable
            cr, pr = np.sort(ev["created"].values), np.sort(ev["processed"].values)
            bl = lambda t: np.searchsorted(cr, t, side="right") - np.searchsorted(pr, t, side="right")
            row.update(thr_s_p5=np.percentile(bins, 5), thr_s_p50=np.percentile(bins, 50), thr_s_min=bins.min(),
                       backlog_growth_rps=(bl(me) - bl(ms)) / win, backlog_end=int(bl(me)))
            row.update(throughput_rps=len(inw) / win, processed_total=len(ev),
                       unprocessed_frac=max(0.0, 1 - len(ev) / max(1, meta["sent"])),
                       lat_p50_ms=lat.quantile(0.5) if len(lat) else np.nan, lat_p99_ms=lat.quantile(0.99) if len(lat) else np.nan)
        lost = max(0, meta["sent"] - len(ev)) if len(ev) else meta["sent"]
        row["efficiency"] = row.get("throughput_rps", np.nan) / row["offered_rps"] if row["offered_rps"] else np.nan
        row["error_rate"] = (row["publish_errors"] + lost) / max(1, meta["sent"] + row["publish_errors"])
        row["publish_error_rate"] = row["publish_errors"] / max(1, meta["sent"] + row["publish_errors"])
        out.append(row)
    return pd.DataFrame(out)


def summarize3(df: pd.DataFrame, slo_p99_ms: float = 1000.0) -> pd.DataFrame:
    if df.empty or "throughput_rps" not in df:
        return pd.DataFrame()
    df = df.dropna(subset=["throughput_rps"]).copy()
    # sustainable (Karimov et al., ICDE'18; ShuffleBench): processed >= 99 % of the offered load, backlog not growing
    # (<= 1 % of the offered load per second) and p99 latency within the SLO
    df["sustainable"] = ((df["efficiency"] >= 0.99) & (df["backlog_growth_rps"] <= 0.01 * df["offered_rps"])
                         & (df["lat_p99_ms"] <= slo_p99_ms))
    rows = []
    for (s, n), x in df.groupby(["shards", "streams"]):
        r = dict(shards=s, streams=n, reps=len(x), sustainable=bool(x["sustainable"].all()))
        for m in ["throughput_rps", "offered_rps", "lat_p50_ms", "lat_p99_ms", "unprocessed_frac", "error_rate", "efficiency",
                  "thr_s_p5", "thr_s_p50", "thr_s_min", "backlog_growth_rps", "beaver_cpu_busy_pct", "loadgen_cpu_busy_pct"]:
            r[f"{m}_median"], r[f"{m}_min"], r[f"{m}_max"] = x[m].median(), x[m].min(), x[m].max()
        rows.append(r)
    return pd.DataFrame(rows)


def plot_exp3(summary: pd.DataFrame, out: Path, runs: pd.DataFrame = None, slo_p99_ms: float = 1000.0):
    if summary.empty:
        print("no Exp.3 data to plot"); return
    shards = sorted(int(x) for x in summary["shards"].unique())
    pts = summary.groupby("streams")["offered_rps_median"].median().sort_index()
    rate = float(np.median(pts.values / pts.index.values))               # events/s per stream
    xt = [float(v) for v in pts.values]
    xl = [ps.offered_label(int(n), rate) for n in pts.index]
    if len(xl) > 8:                       # many points: label every second tick so the labels stay horizontal
        xl = [l if i % 2 == 0 else "" for i, l in enumerate(xl)]
    xlabel = ps.offered_axis_label(rate)

    def draw(ax, col, ylabel, logy=False, scale=1.0, labelled=True):
        for s in shards:
            sub = summary[summary["shards"] == s].sort_values("offered_rps_median")
            if sub.empty:
                continue
            med = sub[f"{col}_median"].values * scale
            err = _errbars(sub, col) * scale
            ax.errorbar(sub["offered_rps_median"], med, yerr=err, color=ps.SHARD_COLOR.get(s, "k"),
                        marker=ps.SHARD_MARKER.get(s, "o"), ms=ps.MS_LINE, ls="-", lw=ps.LW, capsize=1.5, elinewidth=0.6,
                        markeredgewidth=0, zorder=2 + (len(shards) - shards.index(s)) * 0.1,
                        label=(f"{s} shard" + ("s" if s > 1 else "")) if labelled else None)
        ax.set_xscale("log"); ax.set_xticks(xt); ax.set_xticklabels(xl); ax.minorticks_off()
        ax.set_xlim(min(xt) * 0.8, max(xt) * 1.25)
        ax.set_xlabel(xlabel); ax.set_ylabel(ylabel)
        if logy:
            ax.set_yscale("log"); ps.log_axis(ax, "y")

    def single(col, ylabel, **kw):
        fig, ax = plt.subplots(figsize=ps.FIGSIZE_SMALL, constrained_layout=True)   # same size as the cold start plot
        draw(ax, col, ylabel, **kw)
        return fig, ax

    def add_legend(ax, loc="upper left"):   # framed legend inside the axes, full labels, as in the cold start plot
        ax.legend(loc=loc)

    pct = "[\\%]" if ps._USETEX else "[%]"

    # throughput (mean over the measurement window), log-log
    fig, ax = single("throughput_rps", "Throughput [events/s]", logy=True)
    add_legend(ax); ps.save(fig, out / "scalability_throughput.pdf")

    # per-second throughput: median of the 1 s bins, whisker = 5th percentile .. median (stability of the rate)
    fig, ax = single("thr_s_p5", "Per-second throughput, p5 [events/s]", logy=True)
    add_legend(ax); ps.save(fig, out / "scalability_throughput_p5_1s.pdf")

    # efficiency = processed / offered  (100 % = keeps up; falls when the system is saturated)
    fig, ax = single("efficiency", f"Processed / offered {pct}", scale=100.0)
    ax.axhline(100, color="gray", ls=":", lw=0.9, zorder=1); ax.set_ylim(0, 110); add_legend(ax, "lower left")
    ps.save(fig, out / "scalability_efficiency.pdf")

    # latency p50 / p99
    for col, name, fname in [("lat_p50_ms", "Median latency [ms]", "scalability_latency_p50.pdf"),
                             ("lat_p99_ms", "p99 latency [ms]", "scalability_latency_p99.pdf")]:
        fig, ax = single(col, name, logy=True)
        add_legend(ax); ps.save(fig, out / fname)

    # error rate [%]
    fig, ax = single("error_rate", f"Error rate {pct}", scale=100.0)
    ax.set_ylim(bottom=0); add_legend(ax); ps.save(fig, out / "scalability_error_rate.pdf")

    # CPU utilisation of the Beaver VM (context for the saturation point)
    if summary["beaver_cpu_busy_pct_median"].notna().any():
        fig, ax = single("beaver_cpu_busy_pct", f"Beaver VM CPU busy {pct}")
        ax.set_ylim(0, 100); add_legend(ax); ps.save(fig, out / "scalability_cpu.pdf")

    # maximum sustainable offered load per shard count (all lower loads sustainable too)
    best = {}
    for s in shards:
        sub = summary[summary["shards"] == s].sort_values("streams")
        m = 0.0; hit_max = False
        for _, r in sub.iterrows():
            if not r["sustainable"]:
                break
            m = r["offered_rps_median"]; hit_max = r["streams"] == sub["streams"].max()
        best[s] = (m, hit_max)
    fig, ax = plt.subplots(figsize=ps.FIGSIZE_SMALL, constrained_layout=True)
    for i, s in enumerate(shards):
        v, lb = best[s]
        ax.bar(i, v, color=ps.SHARD_COLOR.get(s, "gray"), edgecolor="black", linewidth=0.8, hatch="///" if lb else "")
        ax.text(i, v, (r"$\geq$" if lb and ps._USETEX else (">=" if lb else "")) + f"{v:.0f}", ha="center", va="bottom", fontsize=7)
    ax.set_xticks(range(len(shards))); ax.set_xticklabels([str(s) for s in shards])
    ax.set_xlabel("Shards"); ax.set_ylabel("Sustainable [events/s]"); ax.grid(axis="x", visible=False)
    ax.set_ylim(0, max(1, max(v for v, _ in best.values())) * 1.15)
    ps.save(fig, out / "scalability_sustainable.pdf")
    pd.DataFrame([dict(shards=s, max_sustainable_offered_rps=v, lower_bound_only=lb, slo_p99_ms=slo_p99_ms) for s, (v, lb) in best.items()]
                 ).to_csv(out / "exp3_max_sustainable.csv", index=False)

    # Theodolite's second scalability metric, "resource demand": fewest shards that sustain each load
    dem = []
    for n, g in summary.groupby("streams"):
        ok = g[g["sustainable"]]["shards"]
        dem.append(dict(streams=int(n), offered_rps=float(g["offered_rps_median"].median()), min_shards_sustaining=int(ok.min()) if len(ok) else np.nan))
    dem = pd.DataFrame(dem).sort_values("streams"); dem.to_csv(out / "exp3_resource_demand.csv", index=False)
    fig, ax = plt.subplots(figsize=ps.FIGSIZE_SMALL, constrained_layout=True)
    d2 = dem.dropna()
    ax.step(d2["offered_rps"], d2["min_shards_sustaining"], where="post", color=ps.COLOR["beaver"], marker="o", ms=ps.MS, lw=ps.LW, markeredgewidth=0)
    ax.set_xscale("log"); ax.set_xticks(xt); ax.set_xticklabels(xl); ax.minorticks_off()
    ax.set_yscale("log", base=2); ax.set_yticks(shards); ax.set_yticklabels([str(v) for v in shards]); ax.minorticks_off()
    ax.set_xlabel(xlabel); ax.set_ylabel("Required shards")
    ps.save(fig, out / "scalability_resource_demand.pdf")

    # time series of processed events per second (1 s bins) for the highest tested load, one line per shard count
    if runs is not None and not runs.empty:
        top = runs["streams"].max()
        fig, ax = plt.subplots(figsize=(ps.FIGSIZE_SMALL[0], 2.35), constrained_layout=True)
        for s in shards:
            rr = runs[(runs["shards"] == s) & (runs["streams"] == top)].sort_values("rep")
            if rr.empty:
                continue
            d = Path(rr.iloc[0]["run_dir"]); meta = load_meta(d / "meta.json")
            ev = parse_events(read_text(d / "logs.txt.gz"), t0=meta["t0_ns"])
            if ev.empty:
                continue
            sec = ((ev["processed"].values - meta["t0_ns"]) / 1e9).astype(int)
            cnt = np.bincount(sec[sec >= 0])
            ax.plot(np.arange(len(cnt)), cnt, color=ps.SHARD_COLOR.get(s, "k"), lw=ps.LW, label=f"{s} shard" + ("s" if s > 1 else ""))
        if not ax.lines:
            plt.close(fig); return
        ms_rel = (meta["measure_start_ns"] - meta["t0_ns"]) / 1e9; me_rel = (meta["measure_end_ns"] - meta["t0_ns"]) / 1e9
        ax.axvspan(ms_rel, me_rel, color=ps.COLOR["beaver"], alpha=0.18, lw=0)
        ax.set_xlabel("Time since start [s]"); ax.set_ylabel("Processed [events/s]"); ax.set_ylim(bottom=0)
        ax.legend(loc="upper right", ncol=2, columnspacing=0.8, handlelength=1.4)
        ps.save(fig, out / "scalability_timeseries_peak_load.pdf")

    # overview: 4 metrics side by side (two-column width)
    fig, axs = plt.subplots(1, 4, figsize=(ps.FIGSIZE_LARGE[0], 2.5), constrained_layout=True)
    draw(axs[0], "throughput_rps", "Throughput [events/s]", logy=True, labelled=True)
    draw(axs[1], "lat_p50_ms", "Median latency [ms]", logy=True, labelled=False)
    draw(axs[2], "lat_p99_ms", "p99 latency [ms]", logy=True, labelled=False)
    draw(axs[3], "error_rate", f"Error rate {pct}", scale=100.0, labelled=False)
    axs[3].set_ylim(bottom=0)
    for a in axs:
        a.set_xlabel("")
    fig.supxlabel(xlabel, fontsize=9)
    h, l = axs[0].get_legend_handles_labels()
    axs[3].legend(h, l, loc="center left")   # framed, inside the axes; the error-rate panel is empty left of saturation
    ps.save(fig, out / "scalability_overview.pdf")


# --------------------------------------------------------------------------------------------
# Sanity report
# --------------------------------------------------------------------------------------------

def check_report(r12: pd.DataFrame, r3: pd.DataFrame) -> int:
    problems = 0
    print("\n=== Exp 1/2 run check ===")
    if r12.empty:
        print("no Exp 1/2 runs found")
    for _, r in r12.iterrows():
        flags = []
        if r["n_events"] == 0:
            flags.append("NO EVENTS in logs (check Cloud Logging filter / docker logs)")
        elif r["n_events"] < r["expected"]:
            flags.append(f"incomplete: {r['n_events']}/{r['expected']} events")
        if not r["cold_verified"]:
            flags.append("NOT COLD (no BENCH_BOOT after t0) -> raise CR_IDLE_WAIT, re-run")
        if r.get("min_recv_minus_created_ms", 0) < 0:
            flags.append(f"negative tReceived-tCreated ({r['min_recv_minus_created_ms']:.2f} ms): clock skew between machines")
        if r["status"] != "ok":
            flags.append(f"status={r['status']}")
        co = r.get("clock_offset_ms")
        if r["system"] == "beaver" and co is not None and not pd.isna(co) and co > CLOCK_WARN_MS:
            flags.append(f"clock offset between VMs {co:.2f} ms > {CLOCK_WARN_MS} ms: overhead differences of that size are not trustworthy")
        elif r["system"] == "beaver" and (co is None or pd.isna(co)):
            flags.append("no clock.txt (clock offset unknown)")
        problems += bool(flags)
        pen = r.get("cold_start_penalty_ms", float("nan")); ov = r.get("overhead_ms", float("nan"))
        print(f"{r['system']:12s} d={r['duration']:<3} r={r['rep']:<2} events={r['n_events']}/{r['expected']:<5} "
              f"penalty={pen:10.1f} ms overhead={ov:10.1f} ms  {'OK' if not flags else ' | '.join(flags)}")
    print("\n=== Exp 3 run check ===")
    if r3.empty:
        print("no Exp 3 runs found")
    for _, r in r3.iterrows():
        flags = []
        if "throughput_rps" not in r or pd.isna(r.get("throughput_rps")):
            flags.append("NO EVENTS in logs")
        else:
            if r["unprocessed_frac"] > 0.01:
                flags.append(f"{100*r['unprocessed_frac']:.1f}% unprocessed (saturated: expected at high load)")
            if r["loadgen_cpu_busy_pct"] > 70:
                flags.append(f"LOAD GENERATOR busy {r['loadgen_cpu_busy_pct']:.0f}% -> result may be generator-limited")
        if r["publish_error_rate"] > 0:
            flags.append(f"publish error rate {100*r['publish_error_rate']:.2f}%")
        print(f"shards={r['shards']:<2} streams={r['streams']:<5} r={r['rep']:<2} offered={r['offered_rps']:8.0f}/s "
              f"throughput={r.get('throughput_rps', float('nan')):8.0f}/s beaverCPU={r['beaver_cpu_busy_pct']:5.1f}% "
              f"lgCPU={r['loadgen_cpu_busy_pct']:5.1f}%  {'OK' if not flags else ' | '.join(flags)}")
    return problems


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", required=True, help="results/<campaign> directory")
    ap.add_argument("--out", required=True, help="output directory for csv + pdf")
    ap.add_argument("--overhead-def", choices=["notebook", "total", "last", "mean"], default="notebook")
    ap.add_argument("--slo-p99-ms", type=float, default=1000.0, help="p99 latency bound for the sustainable-throughput criterion (Exp. 3)")
    ap.add_argument("--usetex", choices=["auto", "on", "off"], default="auto", help="LaTeX text rendering (auto: only if libertine.sty is installed)")
    ap.add_argument("--slide-duration", type=int, default=0, help="stream duration used for fig1 (default: smallest)")
    ap.add_argument("--check", action="store_true", help="print a per-run sanity report")
    a = ap.parse_args()

    root, out = Path(a.results), Path(a.out)
    if not root.exists():
        sys.exit(f"{root} does not exist (did you run ./bench.sh fetch?)")
    out.mkdir(parents=True, exist_ok=True)

    r12 = exp12_runs(root, a.overhead_def)
    r3 = exp3_runs(root)
    s12, s3 = summarize12(r12), summarize3(r3, a.slo_p99_ms)
    r12.to_csv(out / "exp12_runs.csv", index=False); s12.to_csv(out / "exp12_summary.csv", index=False)
    r3.to_csv(out / "exp3_runs.csv", index=False); s3.to_csv(out / "exp3_summary.csv", index=False)
    if a.check:
        n = check_report(r12, r3)
        print(f"\n{n} run(s) with findings")
    print("plot style:", ps.setup(a.usetex))
    try:
        plot_exp12(s12, out, a.slide_duration)
        plot_exp3(s3, out, r3, a.slo_p99_ms)
    except RuntimeError as e:          # LaTeX installed but incomplete (missing package): redo without it
        if a.usetex == "off":
            raise
        print(f"LaTeX rendering failed ({str(e).splitlines()[0]}); retrying without LaTeX")
        plt.close("all")
        print("plot style:", ps.setup("off"))
        plot_exp12(s12, out, a.slide_duration)
        plot_exp3(s3, out, r3, a.slo_p99_ms)
    print(f"\nwrote tables + PDF figures to {out}")


if __name__ == "__main__":
    main()
