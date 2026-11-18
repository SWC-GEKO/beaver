#!/usr/bin/env bash
# Beaver benchmark control script (run on your LAPTOP, from the repo's bench/ directory).
#
#   ./bench.sh setup          project, APIs, VPC, service account, bucket, registry
#   ./bench.sh valkey         Memorystore for Valkey (~10 min)
#   ./bench.sh vms            create the load-generator VM and the Beaver VM
#   ./bench.sh bootstrap      push code, install toolchains, build binaries/images on both VMs
#   ./bench.sh smoke          cheap end-to-end test of every system (PROFILE=test)
#   ./bench.sh calibrate-cr   find the CR_IDLE_WAIT that really gives cold Cloud Run starts (~40 min)
#   ./bench.sh run <exp12|exp3|all>   start a campaign in tmux on the VM (detached)
#   ./bench.sh status         progress of the running/finished campaign
#   ./bench.sh logs           tail the campaign log
#   ./bench.sh attach         attach to the tmux session
#   ./bench.sh stop           stop the campaign (current run is aborted)
#   ./bench.sh fetch          copy results to ./results on your laptop
#   ./bench.sh analyze        tables + figures from ./results
#   ./bench.sh push           re-upload the repo (after code/config changes)
#   ./bench.sh ssh <lg|beaver>
#   ./bench.sh vms-stop | vms-start   pause/resume VMs between sessions (saves money)
#   ./bench.sh resize-lg <machine-type>   e.g. e2-standard-16: bigger load generator for very high Exp. 3 loads
#   ./bench.sh teardown [--all]
#
# Use PROFILE=final for the real measurements, e.g.:   PROFILE=final ./bench.sh run all
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$HERE/lib/common.sh"

SESSION="bench"
# variables forwarded to the VM when set on the laptop
FORWARD=(PROFILE CAMPAIGN SYSTEMS DURATIONS REPS_EXP12 REPS_EXP3 SHARDS CONCURRENCY RATE KEYS_EXP12
         CR_IDLE_WAIT CR_PREDEPLOY_MAX CR_CONCURRENCY CR_AUTH CALIBRATE_WAITS BEAVER_ROUTER_CPUS BEAVER_ROUTER_MEM_MIB BEAVER_PROCESSOR_CPUS BEAVER_PROCESSOR_MEM_MIB WARMUP_SEC MEASURE_SEC BEAVER_REPLICATION_EXP12 STREAM_RATE DF_MACHINE)

remote_env() { local s=""; for v in "${FORWARD[@]}"; do [ -n "${!v:-}" ] && s+="$v=$(printf '%q' "${!v}") "; done; echo "$s"; }

push() {
  need gcloud; need tar
  local tarball; tarball="$(mktemp /tmp/beaver-push.XXXXXX.tgz)"
  log "packing $REPO_DIR (without .git, results, secrets)"
  tar -czf "$tarball" -C "$(dirname "$REPO_DIR")" \
    --exclude='.git' --exclude='bench/results' --exclude='bench/.secrets' --exclude='bench/figures' \
    --exclude='*.zip' "$(basename "$REPO_DIR")"
  for vm in "$LG_VM" "$BEAVER_VM"; do
    log "uploading to $vm"
    gscp "$tarball" "bench@${vm}:/tmp/beaver-push.tgz"
    gssh "$vm" 'mkdir -p ~/beaver && tar -xzf /tmp/beaver-push.tgz -C ~/beaver --strip-components=1 && rm /tmp/beaver-push.tgz && chmod +x ~/beaver/bench/*.sh ~/beaver/bench/*/*.sh ~/beaver/bench/lib/*.py'
  done
  rm -f "$tarball"
}

case "${1:-help}" in
  setup)     "$HERE/infra/01_setup_project.sh" ;;
  valkey)    "$HERE/infra/02_create_valkey.sh" ;;
  vms)       "$HERE/infra/03_create_vms.sh" ;;
  push)      push ;;
  bootstrap)
    push
    log "copying the Beaver ssh key to the load-generator VM"
    gscp "$SSH_KEY" "bench@${LG_VM}:/tmp/id_bench"
    gssh "$LG_VM" 'mkdir -p ~/.ssh && mv /tmp/id_bench ~/.ssh/id_bench && chmod 600 ~/.ssh/id_bench'
    log "bootstrapping the Beaver VM (docker, router image, control plane) - ~5-10 min"
    gssh "$BEAVER_VM" 'bash ~/beaver/bench/vm/bootstrap_beaver.sh'
    log "bootstrapping the load-generator VM (Go, binaries, Cloud Run images) - ~5-10 min"
    gssh "$LG_VM" 'bash ~/beaver/bench/vm/bootstrap_lg.sh'
    log "bootstrap finished. Next: ./bench.sh smoke"
    ;;
  smoke)
    push
    env_s="$(remote_env)"
    log "smoke test = PROFILE=test, CAMPAIGN=smoke (1 run per system, 2 Exp3 configs), running detached in tmux on $LG_VM"
    gssh "$LG_VM" "rm -f ~/smoke.done; tmux kill-session -t smoke 2>/dev/null; cd ~/beaver/bench && TERM=xterm-256color tmux new-session -d -s smoke '${env_s}PROFILE=test CAMPAIGN=smoke ./experiments/all.sh 2>&1 | tee -a ~/smoke.log; touch ~/smoke.done'"
    log "waiting for it to finish (a dropped connection no longer matters; Ctrl-C only stops this waiting, not the test)"
    until gssh "$LG_VM" 'test -f ~/smoke.done' >/dev/null 2>&1; do
      sleep 60
      log "still running: $(gssh "$LG_VM" 'tail -n 1 ~/smoke.log' 2>/dev/null | cut -c1-120)"
    done
    log "fetching + checking smoke results"
    CAMPAIGN=smoke "$0" fetch
    python3 "$HERE/analysis/analyze.py" --results "$HERE/results/smoke" --out "$HERE/figures/smoke" --check
    ;;
  run)
    what="${2:?exp12|exp3|all}"
    case "$what" in exp12) script=exp12_all.sh ;; exp3) script=exp3_all.sh ;; all) script=all.sh ;; *) die "unknown: $what" ;; esac
    env_s="$(remote_env)"
    log "starting '$script' in tmux session '$SESSION' on $LG_VM  (env: ${env_s:-defaults})"
    gssh "$LG_VM" "tmux kill-session -t $SESSION 2>/dev/null; cd ~/beaver/bench && TERM=xterm-256color tmux new-session -d -s $SESSION \"${env_s}./experiments/$script 2>&1 | tee -a ~/campaign.log\""
    log "running detached. Check with: ./bench.sh status   |   ./bench.sh logs"
    ;;
  status)
    gssh "$LG_VM" "tmux ls 2>&1 | grep $SESSION || echo 'no tmux session: campaign is not running'; echo; for d in ~/beaver/bench/results/*/; do echo \"== \$d\"; for e in exp12 exp3; do [ -d \$d\$e ] && echo \"  \$e: \$(find \$d\$e -name meta.json | wc -l) runs done, \$(grep -l '\"status\": \"incomplete\"' \$(find \$d\$e -name meta.json) 2>/dev/null | wc -l) incomplete\"; done; done; echo; tail -n 5 ~/campaign.log 2>/dev/null"
    ;;
  logs)      gssh "$LG_VM" 'tail -n 60 -f ~/campaign.log' ;;
  attach)    gssh "$LG_VM" -t 'TERM=xterm-256color tmux attach -t bench' ;;
  stop)      gssh "$LG_VM" "tmux kill-session -t $SESSION && echo stopped || echo 'not running'" ;;
  fetch)
    mkdir -p "$HERE/results"
    log "downloading results/$CAMPAIGN"
    gcloud compute scp --recurse --project "$PROJECT_ID" --zone "$ZONE" --tunnel-through-iap \
      --ssh-key-file "$SSH_KEY" --strict-host-key-checking=no --quiet \
      "bench@${LG_VM}:~/beaver/bench/results/${CAMPAIGN}" "$HERE/results/"
    ;;
  analyze)   python3 "$HERE/analysis/analyze.py" --results "$HERE/results/$CAMPAIGN" --out "$HERE/figures/$CAMPAIGN" ;;
  ssh)       gssh "$( [ "${2:-lg}" = beaver ] && echo "$BEAVER_VM" || echo "$LG_VM")" ;;
  resize-lg)
    mt="${2:?usage: ./bench.sh resize-lg <machine-type>, e.g. e2-standard-16}"
    gcloud compute instances stop "$LG_VM" --zone "$ZONE" --project "$PROJECT_ID"
    gcloud compute instances set-machine-type "$LG_VM" --machine-type "$mt" --zone "$ZONE" --project "$PROJECT_ID"
    gcloud compute instances start "$LG_VM" --zone "$ZONE" --project "$PROJECT_ID"
    log "load generator is now $mt; wait ~1 min before the next ./bench.sh command (ssh must come up)" ;;
  vms-stop)  gcloud compute instances stop "$LG_VM" "$BEAVER_VM" --zone "$ZONE" --project "$PROJECT_ID" ;;
  vms-start)
    gcloud compute instances start "$LG_VM" "$BEAVER_VM" --zone "$ZONE" --project "$PROJECT_ID"
    "$HERE/infra/03_create_vms.sh"   # refreshes internal IPs in state.env (they can change) + waits for ssh
    push
    ;;
  teardown)  "$HERE/infra/99_teardown.sh" "${2:-}" ;;
  help|*)    sed -n '2,25p' "$0" ;;
esac
