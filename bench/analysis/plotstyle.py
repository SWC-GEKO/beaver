"""Shared plot style (derived from the thesis analysis notebook): ACM figure sizes, Linux Libertine,
LaTeX text, hatched bars, PDF output. Palette: Paul Tol high/medium-contrast colours #DDAA33 #6699CC #004488 #994455
(colour-blind safe, distinct in grayscale; Beaver = the warm highlight colour)."""
import shutil, subprocess

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
from matplotlib.ticker import FuncFormatter

FIGSIZE_SMALL = (3.33, 2.06)     # one ACM column
FIGSIZE_LARGE = (7.00, 2.06)     # two columns

PALETTE = ["#DDAA33", "#6699CC", "#004488", "#994455"]   # Tol: gold, light blue, dark blue, wine
SYSTEM_ORDER = ["beaver", "faas-batch", "faas-single", "dataflow"]
LABEL = {"beaver": "Beaver", "faas-batch": "Batch", "faas-single": "FaaS", "dataflow": "Stream"}
COLOR = {"beaver": "#DDAA33", "faas-batch": "#004488", "faas-single": "#6699CC", "dataflow": "#994455"}
HATCH = {"beaver": "///", "faas-batch": "\\\\\\\\", "faas-single": "xx", "dataflow": ""}
MARKER = {"beaver": "o", "faas-batch": "s", "faas-single": "^", "dataflow": "D"}
DASH = {"beaver": "-", "faas-batch": "--", "faas-single": "-.", "dataflow": ":"}
# shard counts are ordinal: same palette sorted light -> dark, so the order survives grayscale printing
SHARD_COLOR = {1: "#DDAA33", 2: "#6699CC", 4: "#994455", 8: "#004488"}
SHARD_MARKER = {1: "o", 2: "s", 4: "^", 8: "D"}
LW = 1.6          # data line width
MS = 3.2          # marker size (uniform)
MS_LINE = 6.0     # marker size of the line plots (cold start penalty, scalability): as in Carl et al.'s figures, but without outline

_USETEX = False


def _have_libertine_tex() -> bool:
    if not shutil.which("latex") or not shutil.which("kpsewhich"):
        return False
    return bool(subprocess.run(["kpsewhich", "libertine.sty"], capture_output=True, text=True).stdout.strip())


def setup(usetex: str = "auto") -> str:
    """usetex: auto (LaTeX + libertine.sty found), on (LaTeX, libertine if available), off."""
    global _USETEX
    have_latex = bool(shutil.which("latex") and shutil.which("dvipng") or shutil.which("latex"))
    lib = _have_libertine_tex()
    _USETEX = (usetex == "on" and have_latex) or (usetex == "auto" and lib)
    rc = {"font.family": "serif", "font.size": 9, "axes.linewidth": 0.6, "axes.grid": True, "grid.alpha": 0.3,
          "grid.linewidth": 0.4, "axes.axisbelow": True, "legend.fontsize": 9, "legend.frameon": True,
          "legend.framealpha": 0.9, "legend.edgecolor": "#cccccc", "legend.borderpad": 0.3,
          "xtick.labelsize": 9, "ytick.labelsize": 9, "lines.linewidth": 1.6, "lines.markersize": 3.2, "lines.markeredgewidth": 0,
          "pdf.fonttype": 42, "ps.fonttype": 42, "savefig.bbox": "standard"}
    if _USETEX:
        rc.update({"text.usetex": True, "font.serif": ["Linux Libertine"],
                   "text.latex.preamble": r"\usepackage{libertine}" if lib else ""})
        msg = "LaTeX text, " + ("Linux Libertine" if lib else "default LaTeX font (libertine.sty not found)")
    else:
        rc.update({"text.usetex": False, "font.serif": ["Linux Libertine O", "Linux Libertine", "Libertinus Serif", "DejaVu Serif"],
                   "mathtext.fontset": "dejavuserif"})
        msg = "matplotlib text (no LaTeX+libertine found; install texlive-fonts-extra to match the thesis look, or pass --usetex on)"
    plt.rcParams.update(rc)
    return msg


def label(system: str) -> str:
    if system == "beaver" and _USETEX:
        return r"$\mathsf{\textsc{Beaver}}$"   # as in Carl et al.
    return LABEL[system]


def num(x) -> str:
    """Math-mode number (matches the notebook's $10$ style when LaTeX is on)."""
    return f"${x}$" if _USETEX else f"{x}"


def log_axis(ax, which="y"):
    """Powers of ten as $10^k$; if the axis spans less than ~1.5 decades also label the 2/5 minor ticks with plain numbers."""
    a = ax.yaxis if which == "y" else ax.xaxis
    lo, hi = a.get_view_interval()
    if lo > 0 and np.log10(hi / lo) < 1.0:        # make sure at least one decade is visible so ticks exist
        mid = np.sqrt(lo * hi)
        lo, hi = mid / 10 ** 0.55, mid * 10 ** 0.55
        (ax.set_ylim if which == "y" else ax.set_xlim)(lo, hi)

    def fmt(x, pos=None):
        if x <= 0:
            return "0"
        lo, hi = a.get_view_interval()
        narrow = np.log10(hi / lo) < 1.5
        e = int(np.floor(np.log10(x) + 1e-9))
        m = x / 10 ** e
        if abs(m - 1) < 1e-6:
            return f"$10^{{{e}}}$"
        if narrow and any(abs(m - k) < 1e-6 for k in (2, 5)):
            return f"{x:g}"
        return ""
    a.set_major_formatter(FuncFormatter(fmt))
    a.set_minor_formatter(FuncFormatter(fmt))


def offered_label(streams, rate=None):
    """x tick label for an offered load of streams*rate events/s: $2^k$ (the factor `rate` goes into the axis label)."""
    k = int(round(np.log2(streams))) if streams > 0 else -1
    if streams > 0 and 2 ** k == streams:
        return f"$2^{{{k}}}$"
    return f"{streams:g}"


def offered_axis_label(rate) -> str:
    """Axis label matching offered_label(): ticks are 2^k streams, each at `rate` events/s."""
    times = r"$\times$" if _USETEX else "\u00d7"
    return f"Offered load [{times}{rate:g} events/s]"


def save(fig, path):
    fig.savefig(path, format="pdf")
    plt.close(fig)
