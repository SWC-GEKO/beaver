#!/usr/bin/env bash
# Whole Exp. 1+2 matrix. Resumable: finished runs are skipped, so just start it again after a failure.
# Order: repetition-major (all systems/durations for rep 1, then rep 2 ...) so that slow
# infrastructure drift/noise is spread over all systems instead of hitting one of them.
set -uo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
log "Exp1/2 matrix: systems=[$SYSTEMS] durations=[$DURATIONS] reps=$REPS_EXP12 profile=$PROFILE"
failed=0
case " $SYSTEMS " in *" faas-"*) "$BENCH_DIR/experiments/exp12_predeploy.sh" ;; esac
for rep in $(seq 1 "$REPS_EXP12"); do
  for dur in $DURATIONS; do
    for sys in $SYSTEMS; do
      if ! "$BENCH_DIR/experiments/exp12_run.sh" "$sys" "$dur" "$rep"; then
        log "!! FAILED: $sys d=$dur r=$rep (continuing; re-run this script to retry)"; failed=$((failed+1))
      fi
    done
  done
done
log "Exp1/2 finished, failed runs: $failed"
