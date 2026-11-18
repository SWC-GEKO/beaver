#!/usr/bin/env bash
# One run of Experiments 1+2 (cold-start penalty + processing overhead).
#   usage: exp12_run.sh <beaver|faas-single|faas-batch|dataflow> <duration_s> <rep>
# Runs ON THE LOAD-GENERATOR VM. Idempotent: a run with status "ok" is skipped.
set -euo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
# shellcheck source=../lib/beaver.sh
source "$BENCH_DIR/lib/beaver.sh"
# shellcheck source=../lib/cloudrun.sh
source "$BENCH_DIR/lib/cloudrun.sh"

system="${1:?system}"; dur="${2:?duration}"; rep="${3:?rep}"
outdir="$RESULTS_DIR/exp12/$system/d$dur/r$rep"
mkdir -p "$outdir"

if [ -f "$outdir/meta.json" ] && jq -e '.status=="ok"' "$outdir/meta.json" >/dev/null 2>&1; then
  log "SKIP $system d=$dur r=$rep (already done)"; exit 0
fi
rm -f "$outdir"/*   # discard partial results of a previous attempt

slug="$(echo "${system}-d${dur}-r${rep}-$(date +%m%d%H%M%S)" | tr 'A-Z_' 'a-z-')"
since="$(iso_now)"
run_extra='{}'                      # run-specific fields merged into meta.json
status="ok"
log "=== RUN $system d=${dur}s rep=$rep ($slug) ==="
exec > >(tee -a "$outdir/run.log") 2>&1

lg_common=(-duration "$dur" -rate "$RATE" -keys "$KEYS_EXP12" -seed "$SEED" -meta "$outdir/lg_meta.json")

# ------------------------------------------------------------------------------------------
run_beaver() {
  beaver_reset
  beaver_start_cp
  local unique
  unique="$(beaver_upload "$BEAVER_REPLICATION_EXP12")" || die "upload failed"
  log "beaver function: $unique"
  run_extra=$(jq -n --arg u "$unique" --argjson r "$BEAVER_REPLICATION_EXP12" --argjson v "$BEAVER_VSHARDS" \
          '{beaver_unique:$u, beaver_replication:$r, beaver_vshards:$v}')
  since="$(iso_now)"
  # cold start: function is registered (image built) but NOT running; first event starts it
  "$LG_BIN" nats-stream -nats-url "nats://${BEAVER_IP}:4222" -subject "FUNCTIONS.${unique}" "${lg_common[@]}"
  local sent; sent=$(jq .sent "$outdir/lg_meta.json")
  beaver_wait_events "$unique" "$sent" 180 || status="incomplete"
  beaver_collect_logs "$unique" "$outdir/logs.txt.gz"
  beaver_snapshot "$unique" "$outdir/beaver_snapshot.txt"
  beaver_cleanup "$unique"
}

run_faas() {   # run_faas single|batch
  local kind="$1" svc img path mode
  if [ "$kind" = "single" ]; then svc="$CR_SERVICE_SINGLE"; img="$(cr_image gcf-single)"; path="StatsFunction"; mode=http-single
  else svc="$CR_SERVICE_BATCH"; img="$(cr_image gcf-batch)"; path="StatsFunctionBatched"; mode=http-batch; fi
  local url rev token f="$RESULTS_DIR/cr_predeploy/${system}_d${dur}_r${rep}.env"
  if [ -f "$f" ]; then
    # shellcheck disable=SC1090
    source "$f"; rm -f "$f"          # consume: this revision is used exactly once
    rev="$PRE_REV"; url="$PRE_URL"
    local age=$(( $(date +%s) - PRE_EPOCH ))
    if [ "$age" -lt "$CR_IDLE_WAIT" ]; then
      log "predeployed revision $rev is ${age}s old; waiting $((CR_IDLE_WAIT - age))s more so its deploy-time instance is gone"
      sleep $((CR_IDLE_WAIT - age))
    else log "predeployed revision $rev is ${age}s old (>= ${CR_IDLE_WAIT}s): no wait needed"; fi
  else
    local suffix; suffix="r$(date +%s)"; rev="$svc-$suffix"
    log "no predeployed revision: deploying $rev and waiting ${CR_IDLE_WAIT}s"
    # deploy WITH a tag: after the first tagged/no-traffic deploy the service's traffic is pinned, so an untagged
    # deploy would get 0 % traffic and the requests would hit another revision (this broke the 10 runs of the first campaign)
    cr_deploy_revision "$svc" "$img" "$suffix" "$kind" "fb$suffix" || die "fallback deploy failed"
    url="$(cr_tag_url "$svc" "fb$suffix")"; [ -n "$url" ] || die "no tag URL for fb$suffix"
    sleep "$CR_IDLE_WAIT"
  fi
  [ "$kind" = "single" ] && valkey_flush
  token=""; [ "$CR_AUTH" = "private" ] && token="$(cr_token "$url")"
  since="$(iso_now)"
  LG_AUTH_TOKEN="$token" "$LG_BIN" "$mode" -url "${url}/${path}" "${lg_common[@]}"
  local expected; expected=$(jq '.sent - (.non_2xx // 0) - .errors' "$outdir/lg_meta.json")
  run_extra=$(jq -n --arg r "$rev" --arg u "$url" '{cloud_run_revision:$r, url:$u}')
  python3 "$BENCH_DIR/lib/collect_gcl.py" --project "$PROJECT_ID" --since "$since" --expected "$expected" \
    --timeout "$LOG_WAIT_SEC" --out "$outdir/logs.txt.gz" \
    --filter "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"$svc\" AND resource.labels.revision_name=\"$rev\" AND (textPayload:\"@@@\" OR textPayload:\"BENCH_BOOT\")" \
    || status="incomplete"
}

df_attempt() {   # df_attempt <attempt>  -> 0 ok, 4 = job failed (e.g. zone stockout), other = incomplete
  local att="$1" rc=0
  local job="bench-$slug-a$att" topic="bench-$slug-a$att" sub="bench-$slug-a$att-sub"
  job="${job:0:60}"; topic="${topic:0:240}"
  # Pub/Sub resources are always removed, even when submission fails (set -e would otherwise leave them behind)
  df_cleanup_pubsub() {
    gcloud pubsub subscriptions delete "$sub" --project "$PROJECT_ID" --quiet >/dev/null 2>&1 || true
    gcloud pubsub topics delete "$topic" --project "$PROJECT_ID" --quiet >/dev/null 2>&1 || true
  }
  trap df_cleanup_pubsub EXIT
  log "creating Pub/Sub topic+subscription"
  gcloud pubsub topics create "$topic" --project "$PROJECT_ID" --quiet >/dev/null
  gcloud pubsub subscriptions create "$sub" --topic "$topic" --project "$PROJECT_ID" \
    --ack-deadline 60 --message-retention-duration 1h --quiet >/dev/null
  since="$(iso_now)"
  # t0 is taken inside the binary right before job submission. NO Dataflow job exists beforehand.
  # --worker_binary = the prebuilt linux/amd64 binary: no cross-compile, so compile time is not part of the measured penalty.
  local df_zone=(); [ -n "${DF_ZONE:-}" ] && df_zone=(--worker_zone="$DF_ZONE")   # empty: Dataflow picks the zone in $REGION
  "$BEAM_BIN" \
    -bench_project "$PROJECT_ID" -bench_topic "$topic" -bench_subscription "$sub" \
    -bench_duration "$dur" -bench_rate "$RATE" -bench_keys "$KEYS_EXP12" -bench_seed "$SEED" \
    -bench_meta "$outdir/lg_meta.json" \
    --runner=dataflow --project="$PROJECT_ID" --region="$REGION" "${df_zone[@]}" \
    --staging_location="gs://${BUCKET}/staging" --temp_location="gs://${BUCKET}/temp" \
    --worker_binary="$BEAM_BIN" \
    --job_name="$job" --execute_async --num_workers=1 --max_num_workers=1 \
    --worker_machine_type="$DF_MACHINE" --disk_size_gb=30 \
    --network="$NETWORK" --subnetwork="regions/${REGION}/subnetworks/${SUBNET}" \
    --service_account_email="$RUNNER_SA"
  local expected job_id=""
  expected=$(jq '.sent' "$outdir/lg_meta.json")
  job_id="$(gcloud dataflow jobs list --project "$PROJECT_ID" --region "$REGION" --filter="name=$job" --format='value(JOB_ID)' | head -1)"
  run_extra=$(jq -n --arg j "$job" --arg id "$job_id" --arg m "$DF_MACHINE" '{dataflow_job:$j, dataflow_job_id:$id, dataflow_machine:$m}')
  log "dataflow job: $job ($job_id); waiting for results (up to ${DF_TIMEOUT_SEC}s)"
  python3 "$BENCH_DIR/lib/collect_gcl.py" --project "$PROJECT_ID" --since "$since" --expected "$expected" \
    --timeout "$DF_TIMEOUT_SEC" --poll 30 --out "$outdir/logs.txt.gz" --job-id "$job_id" --region "$REGION" \
    --filter "resource.type=\"dataflow_step\" AND resource.labels.job_name=\"$job\" AND (textPayload:\"@@@\" OR jsonPayload.message:\"@@@\")" \
    || rc=$?
  log "cancelling job and deleting Pub/Sub resources"
  if [ -n "$job_id" ]; then
    gcloud dataflow jobs cancel "$job_id" --project "$PROJECT_ID" --region "$REGION" --quiet >/dev/null 2>&1 || true
  fi
  df_cleanup_pubsub; trap - EXIT
  # do not start the next run before the job is really gone
  if [ -n "$job_id" ]; then
    wait_for 600 15 "dataflow job to stop" bash -c \
      "[[ \"\$(gcloud dataflow jobs describe $job_id --project $PROJECT_ID --region $REGION --format='value(currentState)')\" =~ CANCELLED|FAILED|DONE|DRAINED ]]" || true
  fi
  return "$rc"
}

run_dataflow() {
  local att rc=1
  for att in $(seq 1 "$DF_ATTEMPTS"); do
    log "dataflow attempt $att/$DF_ATTEMPTS"
    df_attempt "$att" && rc=0 || rc=$?
    [ "$rc" -eq 4 ] || break          # only a failed job (stockout, worker startup) is retried
    log "dataflow job failed (see Cloud Logging, e.g. ZONE_RESOURCE_POOL_EXHAUSTED); retrying with a fresh job"
  done
  run_extra=$(jq -c --argjson a "${att:-1}" '. + {dataflow_attempts:$a}' <<<"$run_extra")
  [ "$rc" -eq 0 ] || status="incomplete"
}

clock_snapshot "$outdir/clock.txt"
case "$system" in
  beaver)      run_beaver ;;
  faas-single) run_faas single ;;
  faas-batch)  run_faas batch ;;
  dataflow)    run_dataflow ;;
  *) die "unknown system '$system'" ;;
esac

jq -n --slurpfile lg "$outdir/lg_meta.json" --argjson extra "$run_extra" \
  --arg system "$system" --argjson dur "$dur" --argjson rep "$rep" --arg slug "$slug" --arg status "$status" \
  --arg profile "$PROFILE" --arg since "$since" \
  '$lg[0] + $extra + {system:$system, stream_duration:$dur, rep:$rep, run_slug:$slug, status:$status, profile:$profile, since:$since}' \
  > "$outdir/meta.json"
log "=== DONE $system d=${dur}s rep=$rep status=$status ==="
