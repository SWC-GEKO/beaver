#!/usr/bin/env bash
# Calibrates CR_IDLE_WAIT. Runs ON THE LOAD-GENERATOR VM.
# For every wait W in CALIBRATE_WAITS: deploy a fresh revision of the FaaS-Batched service (no Valkey needed),
# wait W seconds, send ONE request, and check Cloud Logging for a BENCH_BOOT line after that request.
#   BENCH_BOOT after the request  -> the instance was cold  -> W is long enough
#   no BENCH_BOOT                  -> the deploy-time instance was still alive -> W is too short
# Result: the smallest W that was cold (use it, plus a margin, as CR_IDLE_WAIT).
set -euo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
# shellcheck source=../lib/cloudrun.sh
source "$BENCH_DIR/lib/cloudrun.sh"

svc="$CR_SERVICE_BATCH"; img="$(cr_image gcf-batch)"
url="$(cr_url "$svc")"
out="$RESULTS_DIR/calibrate-cr"; mkdir -p "$out"
: > "$out/result.tsv"; printf 'wait_s\tcold\trevision\n' >> "$out/result.tsv"
log "calibrating CR_IDLE_WAIT with waits: $CALIBRATE_WAITS  (this takes the sum of the waits plus deploys)"

for w in $CALIBRATE_WAITS; do
  suffix="cal$(date +%s)"
  log "--- wait=${w}s: deploying revision $svc-$suffix"
  cr_deploy_revision "$svc" "$img" "$suffix" batch
  sleep "$w"
  token=""; [ "$CR_AUTH" = "private" ] && token="$(cr_token "$url")"
  since="$(iso_now)"
  LG_AUTH_TOKEN="$token" "$LG_BIN" http-batch -url "${url}/StatsFunctionBatched" -duration 1 -rate 1 -keys 1 \
      -seed "$SEED" -meta "$out/lg_meta_${w}.json" >/dev/null 2>&1 || log "request failed (continuing)"
  flt="resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"$svc\" AND resource.labels.revision_name=\"$svc-$suffix\" AND textPayload:\"BENCH_BOOT\" AND timestamp>=\"$since\""
  cold="no"
  for _ in 1 2 3 4 5 6; do   # Cloud Logging ingestion delay
    n=$(gcloud logging read "$flt" --project "$PROJECT_ID" --limit 5 --format='value(timestamp)' --quiet 2>/dev/null | wc -l)
    if [ "$n" -gt 0 ]; then cold="yes"; break; fi
    sleep 15
  done
  log "wait=${w}s -> cold=$cold"
  printf '%s\t%s\t%s\n' "$w" "$cold" "$svc-$suffix" >> "$out/result.tsv"
done

echo; column -t -s $'\t' "$out/result.tsv"
best=$(awk -F'\t' 'NR>1 && $2=="yes" {print $1; exit}' "$out/result.tsv")
if [ -n "$best" ]; then
  echo; echo "Smallest wait that gave a cold start: ${best}s. Use e.g. CR_IDLE_WAIT=$((best + best / 5)) (20% margin)."
else
  echo; echo "No tested wait gave a cold start. Raise CALIBRATE_WAITS (max ~900) or use CR_IDLE_WAIT=900."
fi
echo "Note: one sample per wait. If a wait is cold once and warm once, take the larger one."
