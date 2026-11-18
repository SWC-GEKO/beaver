#!/usr/bin/env bash
# Whole Exp. 3 matrix (resumable). Repetition-major order.
set -uo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
log "Exp3 matrix: shards=[$SHARDS] streams=[$CONCURRENCY] reps=$REPS_EXP3 profile=$PROFILE"
failed=0
for rep in $(seq 1 "$REPS_EXP3"); do
  for s in $SHARDS; do
    for n in $CONCURRENCY; do
      if ! "$BENCH_DIR/experiments/exp3_run.sh" "$s" "$n" "$rep"; then
        log "!! FAILED: shards=$s streams=$n r=$rep (continuing; re-run to retry)"; failed=$((failed+1))
      fi
    done
  done
done
log "Exp3 finished, failed runs: $failed"
