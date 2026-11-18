#!/usr/bin/env bash
# Deploys ALL Cloud Run revisions of the Exp. 1/2 matrix up front, without traffic, each with its own tag.
# Why: a fresh revision always has one instance (started by the deploy) that stays alive for up to ~15 min.
# Waiting that out before every run would cost ~15 min x (runs). Revisions deployed together idle out
# together, so the whole matrix waits ONCE. exp12_run.sh then calls each revision via its tag URL and only
# sleeps if the revision is younger than CR_IDLE_WAIT. Resumable: existing records are kept,
# finished runs are skipped. A record is consumed (deleted) when its run starts, so a repeated run
# always gets a revision that has never been called.
set -uo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
# shellcheck source=../lib/cloudrun.sh
source "$BENCH_DIR/lib/cloudrun.sh"

rec="$RESULTS_DIR/cr_predeploy"; mkdir -p "$rec"
n=0
for rep in $(seq 1 "$REPS_EXP12"); do
  for dur in $DURATIONS; do
    for sys in $SYSTEMS; do
      case "$sys" in
        faas-single) kind=single; svc="$CR_SERVICE_SINGLE"; img="$(cr_image gcf-single)" ;;
        faas-batch)  kind=batch;  svc="$CR_SERVICE_BATCH";  img="$(cr_image gcf-batch)" ;;
        *) continue ;;
      esac
      out="$RESULTS_DIR/exp12/$sys/d$dur/r$rep"
      if [ -f "$out/meta.json" ] && jq -e '.status=="ok"' "$out/meta.json" >/dev/null 2>&1; then continue; fi
      f="$rec/${sys}_d${dur}_r${rep}.env"
      [ -f "$f" ] && continue
      # every fresh revision keeps one instance alive for ~15 min: too many at once exhausts the regional Cloud Run
      # CPU quota (first campaign: 20 revisions x 1 vCPU, the 21st deploy failed). Stay below it; the rest falls back.
      live=$(find "$rec" -name '*.env' | wc -l)
      if [ "$live" -ge "${CR_PREDEPLOY_MAX:-16}" ]; then log "predeploy limit CR_PREDEPLOY_MAX=${CR_PREDEPLOY_MAX:-16} reached: $sys d=$dur r=$rep will deploy + wait when it runs"; continue; fi
      tag="d${dur}r${rep}"; suffix="${kind:0:1}${dur}r${rep}x$(date +%s)"
      log "predeploy $svc-$suffix (tag $tag)"
      if cr_deploy_revision "$svc" "$img" "$suffix" "$kind" "$tag"; then
        url="$(cr_tag_url "$svc" "$tag")"
        if [ -n "$url" ]; then
          printf 'PRE_REV=%s\nPRE_URL=%s\nPRE_EPOCH=%s\n' "$svc-$suffix" "$url" "$(date +%s)" > "$f"; n=$((n+1))
        else log "!! could not read tag URL for $svc tag $tag (run will fall back to deploy+wait)"; fi
      else log "!! predeploy failed for $svc $tag (run will fall back to deploy+wait)"; fi
    done
  done
done
log "predeployed $n revisions"
