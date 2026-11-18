#!/usr/bin/env bash
# Prepares the LOAD-GENERATOR / ORCHESTRATOR VM: toolchain, binaries, docker auth, Cloud Run images.
set -euo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
"$BENCH_DIR/vm/bootstrap_common.sh"

[ -f "$HOME/.ssh/id_bench" ] || die "$HOME/.ssh/id_bench missing (./bench.sh bootstrap copies it)"
chmod 600 "$HOME/.ssh/id_bench"

log "building lg (load generator)"
( cd "$BENCH_DIR/loadgen" && go mod tidy && go build -o "$HOME/bin/lg" . )
log "building beam-bench (Dataflow pipeline + load generator)"
( cd "$BENCH_DIR/beam" && go mod tidy && go build -o "$HOME/bin/beam-bench" ./cmd )

log "docker auth for Artifact Registry"
gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet >/dev/null

log "checking access to the Beaver VM (${BEAVER_IP:-unset})"
ssh -i "$HOME/.ssh/id_bench" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
    -o ConnectTimeout=10 "bench@${BEAVER_IP:?}" 'echo ssh-to-beaver-ok'

# docker group is only active in new sessions; use sg so this script can continue
log "building + deploying Cloud Run images"
sg docker -c "$BENCH_DIR/deploy/cloudrun/build_push.sh"
log "load-generator VM ready"
