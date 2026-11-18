#!/usr/bin/env bash
# Laptop: Memorystore for Valkey (state store of FaaS-Single). Cluster mode disabled, 1 shard, no replica.
# Takes ~5-10 minutes. Writes VALKEY_ADDR to state.env.
set -euo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
need gcloud
need python3

POLICY="bench-valkey-policy"
SUBNET_URI="projects/${PROJECT_ID}/regions/${REGION}/subnetworks/${SUBNET}"
NETWORK_URI="projects/${PROJECT_ID}/global/networks/${NETWORK}"

log "service connection policy (Private Service Connect for Memorystore)"
if gcloud network-connectivity service-connection-policies describe "$POLICY" --region "$REGION" --project "$PROJECT_ID" >/dev/null 2>&1; then
  log "policy $POLICY already exists - skipping"
else
  gcloud network-connectivity service-connection-policies create "$POLICY" \
    --network "$NETWORK" --region "$REGION" --service-class gcp-memorystore \
    --subnets "$SUBNET_URI" --project "$PROJECT_ID"
fi

log "Valkey instance $VALKEY_NAME (node type $VALKEY_NODE)"
if gcloud memorystore instances describe "$VALKEY_NAME" --location "$REGION" --project "$PROJECT_ID" >/dev/null 2>&1; then
  log "instance $VALKEY_NAME already exists - skipping creation, only reading its endpoint"
else
  gcloud memorystore instances create "$VALKEY_NAME" --location "$REGION" --project "$PROJECT_ID" \
    --node-type "$VALKEY_NODE" --shard-count 1 --replica-count 0 --mode cluster-disabled \
    --endpoints="[{\"connections\": [{\"pscAutoConnection\": {\"network\": \"${NETWORK_URI}\", \"projectId\": \"${PROJECT_ID}\"}}]}]"
fi

log "waiting for state ACTIVE"
until [ "$(gcloud memorystore instances describe "$VALKEY_NAME" --location "$REGION" --project "$PROJECT_ID" --format='value(state)')" = "ACTIVE" ]; do
  sleep 15
done

JSON="$(gcloud memorystore instances describe "$VALKEY_NAME" --location "$REGION" --project "$PROJECT_ID" --format=json)"
case "$JSON" in
  *"networks/${NETWORK}"*) ;;
  *) die "Valkey instance $VALKEY_NAME is not attached to network $NETWORK - delete it or change VALKEY_NAME in config.env" ;;
esac
ADDR="$(python3 - "$JSON" <<'PY'
import json, sys
d = json.loads(sys.argv[1])
def find(o):
    if isinstance(o, dict):
        if "ipAddress" in o:
            return o["ipAddress"], o.get("port", 6379)
        for v in o.values():
            r = find(v)
            if r: return r
    elif isinstance(o, list):
        for v in o:
            r = find(v)
            if r: return r
r = find(d)
print(f"{r[0]}:{r[1]}" if r else "")
PY
)"
[ -n "$ADDR" ] || die "could not find the endpoint IP in 'gcloud memorystore instances describe' - run it by hand and set VALKEY_ADDR in state.env"
state_set VALKEY_ADDR "$ADDR"
log "VALKEY_ADDR=$ADDR (saved to state.env)"
