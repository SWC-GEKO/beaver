#!/usr/bin/env bash
# Shared helpers. Sourced by every script, on the laptop and on the VMs.
# shellcheck shell=bash

BENCH_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_DIR="$(cd "$BENCH_DIR/.." && pwd)"; export REPO_DIR

# shellcheck disable=SC1091
source "$BENCH_DIR/config.env"
# shellcheck disable=SC1091
[ -f "$BENCH_DIR/state.env" ] && source "$BENCH_DIR/state.env"   # written by infra scripts

export PATH="/usr/local/go/bin:$HOME/go/bin:$HOME/bin:$PATH"
RESULTS_DIR="${RESULTS_DIR:-$BENCH_DIR/results/$CAMPAIGN}"
LG_BIN="${LG_BIN:-$HOME/bin/lg}"
BEAM_BIN="${BEAM_BIN:-$HOME/bin/beam-bench}"

log()  { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*" >&2; }
die()  { log "ERROR: $*"; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "missing command: $1"; }

# state.env keys are written with this helper (replaces an existing key)
state_set() {
  local k="$1" v="$2" f="$BENCH_DIR/state.env"
  touch "$f"
  grep -v "^export ${k}=" "$f" > "$f.tmp" || true
  printf 'export %s="%s"\n' "$k" "$v" >> "$f.tmp"
  mv "$f.tmp" "$f"
}

now_ns() { date +%s%N; }
iso_now() { date -u +%Y-%m-%dT%H:%M:%SZ; }

# Poll until a command succeeds: wait_for <timeout_s> <interval_s> <description> <cmd...>
wait_for() {
  local timeout="$1" interval="$2" desc="$3"; shift 3
  local start; start=$(date +%s)
  until "$@"; do
    if (( $(date +%s) - start > timeout )); then log "timeout waiting for: $desc"; return 1; fi
    sleep "$interval"
  done
}

# ---- laptop -> VM (IAP tunnel) --------------------------------------------------------
SSH_KEY="$BENCH_DIR/.secrets/bench_key"
gssh() {   # gssh <vm> [command...]
  local vm="$1"; shift
  gcloud compute ssh "bench@${vm}" --project "$PROJECT_ID" --zone "$ZONE" --tunnel-through-iap \
    --ssh-key-file "$SSH_KEY" --strict-host-key-checking=no --quiet -- "$@"
}
gscp() {   # gscp <local-path> <vm>:<remote-path>
  gcloud compute scp --project "$PROJECT_ID" --zone "$ZONE" --tunnel-through-iap \
    --ssh-key-file "$SSH_KEY" --strict-host-key-checking=no --quiet "$@"
}
