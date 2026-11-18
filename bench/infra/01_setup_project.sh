#!/usr/bin/env bash
# Laptop: APIs, VPC, firewall, service account + roles, bucket, Artifact Registry. Idempotent.
set -euo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
need gcloud
gcloud config set project "$PROJECT_ID" >/dev/null

log "enabling APIs (takes a minute)"
gcloud services enable compute.googleapis.com run.googleapis.com artifactregistry.googleapis.com \
  pubsub.googleapis.com dataflow.googleapis.com memorystore.googleapis.com \
  networkconnectivity.googleapis.com logging.googleapis.com iap.googleapis.com \
  storage.googleapis.com iam.googleapis.com --project "$PROJECT_ID"

log "VPC $NETWORK / $SUBNET ($SUBNET_RANGE)"
gcloud compute networks describe "$NETWORK" --project "$PROJECT_ID" >/dev/null 2>&1 || \
  gcloud compute networks create "$NETWORK" --subnet-mode=custom --project "$PROJECT_ID"
gcloud compute networks subnets describe "$SUBNET" --region "$REGION" --project "$PROJECT_ID" >/dev/null 2>&1 || \
  gcloud compute networks subnets create "$SUBNET" --network "$NETWORK" --region "$REGION" \
    --range "$SUBNET_RANGE" --enable-private-ip-google-access --project "$PROJECT_ID"

log "firewall rules"
gcloud compute firewall-rules describe bench-allow-iap-ssh --project "$PROJECT_ID" >/dev/null 2>&1 || \
  gcloud compute firewall-rules create bench-allow-iap-ssh --network "$NETWORK" --direction INGRESS \
    --allow tcp:22 --source-ranges 35.235.240.0/20 --project "$PROJECT_ID"
gcloud compute firewall-rules describe bench-allow-internal --project "$PROJECT_ID" >/dev/null 2>&1 || \
  gcloud compute firewall-rules create bench-allow-internal --network "$NETWORK" --direction INGRESS \
    --allow tcp,udp,icmp --source-ranges "$SUBNET_RANGE" --project "$PROJECT_ID"

log "service account $RUNNER_SA"
gcloud iam service-accounts describe "$RUNNER_SA" --project "$PROJECT_ID" >/dev/null 2>&1 || \
  gcloud iam service-accounts create "$RUNNER_SA_NAME" --display-name "Beaver benchmark runner" --project "$PROJECT_ID"
for role in roles/run.admin roles/iam.serviceAccountUser roles/dataflow.admin roles/dataflow.worker \
            roles/pubsub.admin roles/storage.admin roles/artifactregistry.writer roles/logging.viewer \
            roles/logging.logWriter roles/compute.viewer roles/serviceusage.serviceUsageConsumer; do
  gcloud projects add-iam-policy-binding "$PROJECT_ID" --member "serviceAccount:$RUNNER_SA" \
    --role "$role" --condition=None --quiet >/dev/null
done

log "bucket gs://$BUCKET"
gcloud storage buckets describe "gs://$BUCKET" >/dev/null 2>&1 || \
  gcloud storage buckets create "gs://$BUCKET" --location "$REGION" --uniform-bucket-level-access --project "$PROJECT_ID"

log "Artifact Registry repo $AR_REPO"
gcloud artifacts repositories describe "$AR_REPO" --location "$REGION" --project "$PROJECT_ID" >/dev/null 2>&1 || \
  gcloud artifacts repositories create "$AR_REPO" --repository-format docker --location "$REGION" --project "$PROJECT_ID"

log "01_setup_project done"
