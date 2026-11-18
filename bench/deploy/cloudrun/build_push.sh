#!/usr/bin/env bash
# Build + push the two Cloud Run images and create the services (first revision).
# Runs on the LOAD-GENERATOR VM (needs docker). Re-run after changing evaluation/gcf-*.
set -euo pipefail
# shellcheck source=../../lib/common.sh
source "$(dirname "$0")/../../lib/common.sh"
# shellcheck source=../../lib/cloudrun.sh
source "$BENCH_DIR/lib/cloudrun.sh"

for v in gcf-single gcf-batch; do
  img="$(cr_image "$v")"
  tmp="$(mktemp -d)"
  cp -r "$REPO_DIR/evaluation/$v/." "$tmp/"
  sed "s|__MODULE__|$v|g" "$BENCH_DIR/deploy/cloudrun/main.go.tmpl" > "$tmp/cmd/main.go"
  cp "$BENCH_DIR/deploy/cloudrun/Dockerfile" "$tmp/Dockerfile"
  log "building $img"
  docker build -t "$img" "$tmp"
  docker push "$img"
  rm -rf "$tmp"
done

log "creating services (first revision, CR_AUTH=${CR_AUTH})"
cr_deploy_revision "$CR_SERVICE_SINGLE" "$(cr_image gcf-single)" "init$(date +%s)" single
cr_deploy_revision "$CR_SERVICE_BATCH"  "$(cr_image gcf-batch)"  "init$(date +%s)" batch
for svc in "$CR_SERVICE_SINGLE" "$CR_SERVICE_BATCH"; do
  if [ "$CR_AUTH" = "private" ]; then
    gcloud run services add-iam-policy-binding "$svc" --project "$PROJECT_ID" --region "$REGION" \
      --member "serviceAccount:${RUNNER_SA}" --role roles/run.invoker --quiet >/dev/null
  fi
  log "$svc -> $(cr_url "$svc")"
done
