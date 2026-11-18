#!/usr/bin/env bash
# Everything, in one tmux session on the load-generator VM: Exp1/2 then Exp3.
set -uo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
mkdir -p "$RESULTS_DIR"
"$BENCH_DIR/experiments/exp12_all.sh"
"$BENCH_DIR/experiments/exp3_all.sh"
log "ALL DONE ($(find "$RESULTS_DIR" -name meta.json | wc -l) runs recorded in $RESULTS_DIR)"
