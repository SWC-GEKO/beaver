#!/usr/bin/env bash
# One run of Experiment 3 (Beaver scalability).
#   usage: exp3_run.sh <physical_shards> <concurrent_streams> <rep>
# Fresh Beaver function per run (replication = physical shards, virtual shards constant).
# Load: N concurrent streams x STREAM_RATE events/s, uniform random keys.
# Phases: warmup -> measurement window -> cooldown -> drain.   Runs ON THE LOAD-GENERATOR VM.
set -euo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
# shellcheck source=../lib/beaver.sh
source "$BENCH_DIR/lib/beaver.sh"

shards="${1:?shards}"; streams="${2:?streams}"; rep="${3:?rep}"
outdir="$RESULTS_DIR/exp3/s${shards}/n${streams}/r${rep}"
mkdir -p "$outdir"

if [ -f "$outdir/meta.json" ] && jq -e '.status=="ok"' "$outdir/meta.json" >/dev/null 2>&1; then
  log "SKIP shards=$shards streams=$streams r=$rep (already done)"; exit 0
fi
rm -f "$outdir"/*
exec > >(tee -a "$outdir/run.log") 2>&1
log "=== EXP3 shards=$shards streams=$streams rep=$rep ==="
status="ok"

beaver_reset
beaver_start_cp
unique="$(beaver_upload "$shards")" || die "upload failed"
log "beaver function: $unique"
beaver_prime "$unique" "$shards"      # containers are up before the clock starts

clock_snapshot "$outdir/clock.txt"
beaver_sampler_start
(vmstat -t 5 > "$outdir/loadgen_vmstat.txt" 2>&1 & echo $! > "$outdir/.vmstat.pid")

"$LG_BIN" nats-scale -nats-url "nats://${BEAVER_IP}:4222" -subject "FUNCTIONS.${unique}" \
  -streams "$streams" -stream-rate "$STREAM_RATE" -keyspace "$KEYSPACE_EXP3" \
  -warmup "$WARMUP_SEC" -measure "$MEASURE_SEC" -cooldown "$COOLDOWN_SEC" -seed "$SEED" \
  -meta "$outdir/lg_meta.json"

kill "$(cat "$outdir/.vmstat.pid")" 2>/dev/null || true; rm -f "$outdir/.vmstat.pid"
sleep "$DRAIN_SEC"                     # let the shards drain what is still queued
beaver_sampler_stop "$outdir"

sent=$(jq .sent "$outdir/lg_meta.json")
processed=$(beaver_count_events "$unique"); processed="${processed:-0}"
log "sent=$sent processed(after drain)=$processed"
[ "$processed" -ge "$sent" ] || log "note: backlog not fully drained (this is data, not an error - see unprocessed fraction)"

beaver_collect_logs "$unique" "$outdir/logs.txt.gz"
beaver_snapshot "$unique" "$outdir/beaver_snapshot.txt"
beaver_cleanup "$unique"

jq -n --slurpfile lg "$outdir/lg_meta.json" --arg u "$unique" \
  --argjson shards "$shards" --argjson streams "$streams" --argjson rep "$rep" \
  --argjson vs "$BEAVER_VSHARDS" --argjson processed "$processed" --arg status "$status" --arg profile "$PROFILE" \
  '$lg[0] + {beaver_unique:$u, shards:$shards, streams:$streams, rep:$rep, vshards:$vs,
             processed_total:$processed, status:$status, profile:$profile}' > "$outdir/meta.json"
log "=== DONE exp3 shards=$shards streams=$streams rep=$rep ==="
