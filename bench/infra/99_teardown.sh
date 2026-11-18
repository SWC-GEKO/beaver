#!/usr/bin/env bash
# Laptop: delete the billable resources. Use --all to also remove VPC, SA, bucket, registry.
set -uo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
ALL=0; [ "${1:-}" = "--all" ] && ALL=1
P=(--project "$PROJECT_ID" --quiet)

log "Cloud Run services"
for s in "$CR_SERVICE_SINGLE" "$CR_SERVICE_BATCH"; do gcloud run services delete "$s" --region "$REGION" "${P[@]}" 2>/dev/null; done
log "leftover Dataflow jobs"
for id in $(gcloud dataflow jobs list --region "$REGION" --status=active --filter="name~^bench-" --format='value(JOB_ID)' --project "$PROJECT_ID" 2>/dev/null); do
  gcloud dataflow jobs cancel "$id" --region "$REGION" "${P[@]}"
done
log "leftover Pub/Sub topics/subscriptions"
for s in $(gcloud pubsub subscriptions list --filter="name~bench-" --format='value(name)' --project "$PROJECT_ID" 2>/dev/null); do gcloud pubsub subscriptions delete "$s" "${P[@]}"; done
for t in $(gcloud pubsub topics list --filter="name~bench-" --format='value(name)' --project "$PROJECT_ID" 2>/dev/null); do gcloud pubsub topics delete "$t" "${P[@]}"; done
log "Valkey"
gcloud memorystore instances delete "$VALKEY_NAME" --location "$REGION" "${P[@]}" 2>/dev/null
gcloud network-connectivity service-connection-policies delete bench-valkey-policy --region "$REGION" "${P[@]}" 2>/dev/null
log "VMs"
gcloud compute instances delete "$LG_VM" "$BEAVER_VM" --zone "$ZONE" "${P[@]}" 2>/dev/null

if [ "$ALL" = 1 ]; then
  log "network, SA, bucket, registry"
  gcloud compute firewall-rules delete bench-allow-iap-ssh bench-allow-internal "${P[@]}" 2>/dev/null
  gcloud compute networks subnets delete "$SUBNET" --region "$REGION" "${P[@]}" 2>/dev/null
  gcloud compute networks delete "$NETWORK" "${P[@]}" 2>/dev/null
  gcloud iam service-accounts delete "$RUNNER_SA" "${P[@]}" 2>/dev/null
  gcloud storage rm -r "gs://$BUCKET" 2>/dev/null
  gcloud artifacts repositories delete "$AR_REPO" --location "$REGION" "${P[@]}" 2>/dev/null
fi
log "teardown done (results already fetched? ./bench.sh fetch)"
