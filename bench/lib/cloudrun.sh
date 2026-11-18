#!/usr/bin/env bash
# Cloud Run helpers (FaaS-Single / FaaS-Batched). Run on the load-generator VM.
# shellcheck shell=bash
# shellcheck source=common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

cr_image() { echo "${REGION}-docker.pkg.dev/${PROJECT_ID}/${AR_REPO}/$1:latest"; }

# cr_deploy_revision <service> <image> <suffix> <single|batch> [tag]
# With a tag the revision is deployed WITHOUT traffic and reachable at its own tag URL (see cr_tag_url).
# New revision => no warm instance of the new code (cold start per run).
cr_deploy_revision() {   # kind: "single" adds Valkey env + VPC egress
  local svc="$1" img="$2" suffix="$3" kind="${4:-}" tag="${5:-}"
  local extra=() maxi="$CR_MAX_INSTANCES_BATCH"
  if [ "$CR_AUTH" = "public" ]; then extra+=(--allow-unauthenticated); else extra+=(--no-allow-unauthenticated); fi
  [ -n "$tag" ] && extra+=(--no-traffic --tag "$tag")
  if [ "$kind" = "single" ]; then
    maxi="$CR_MAX_INSTANCES_SINGLE"
    extra+=(--set-env-vars "VALKEY_ADDR=${VALKEY_ADDR:?VALKEY_ADDR not set (state.env)}"
            --network "$NETWORK" --subnet "$SUBNET" --vpc-egress private-ranges-only)
  fi
  gcloud run deploy "$svc" --project "$PROJECT_ID" --region "$REGION" --image "$img" \
    --revision-suffix "$suffix" --service-account "$RUNNER_SA" \
    --execution-environment gen2 \
    --cpu "$CR_CPU" --memory "$CR_MEMORY" --concurrency "$CR_CONCURRENCY" \
    --min-instances 0 --max-instances "$maxi" --timeout 300 \
    "${extra[@]}" --quiet >&2
}

# cr_tag_url <service> <tag> -> https URL of the tagged (no-traffic) revision
cr_tag_url() {
  gcloud run services describe "$1" --project "$PROJECT_ID" --region "$REGION" --format=json \
    | jq -r --arg t "$2" '[.status.traffic[]? | select(.tag==$t) | .url][0] // empty'
}

cr_url() { gcloud run services describe "$1" --project "$PROJECT_ID" --region "$REGION" --format='value(status.url)'; }

# Identity token for the private service (audience = service URL).
cr_token() {   # only used when CR_AUTH=private
  curl -fsS -H 'Metadata-Flavor: Google' \
    "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?audience=$1"
}

valkey_flush() {
  need redis-cli
  local host="${VALKEY_ADDR%:*}" port="${VALKEY_ADDR##*:}"
  redis-cli -h "$host" -p "$port" FLUSHALL >/dev/null
}
