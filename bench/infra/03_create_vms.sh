#!/usr/bin/env bash
# Laptop: ssh key pair + the two VMs. Writes LG_IP / BEAVER_IP (internal) to state.env.
set -euo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
need gcloud
need ssh-keygen

mkdir -p "$BENCH_DIR/.secrets"
if [ ! -f "$SSH_KEY" ]; then
  log "generating benchmark ssh key"
  ssh-keygen -t ed25519 -N "" -f "$SSH_KEY" -C bench >/dev/null
fi
PUB="$(cat "${SSH_KEY}.pub")"

create_vm() {  # name machine disk_gb
  local name="$1" machine="$2" disk="$3"
  if gcloud compute instances describe "$name" --zone "$ZONE" --project "$PROJECT_ID" >/dev/null 2>&1; then
    log "VM $name exists"; return
  fi
  log "creating VM $name ($machine, ${disk}GB)"
  gcloud compute instances create "$name" --project "$PROJECT_ID" --zone "$ZONE" \
    --machine-type "$machine" --network "$NETWORK" --subnet "$SUBNET" \
    --image-family debian-12 --image-project debian-cloud \
    --boot-disk-size "${disk}GB" --boot-disk-type pd-balanced \
    --service-account "$RUNNER_SA" --scopes cloud-platform \
    --metadata "enable-oslogin=FALSE,ssh-keys=bench:${PUB}"
}
create_vm "$LG_VM" "$LG_MACHINE" 100
create_vm "$BEAVER_VM" "$BEAVER_MACHINE" 100

ip() { gcloud compute instances describe "$1" --zone "$ZONE" --project "$PROJECT_ID" --format='value(networkInterfaces[0].networkIP)'; }
state_set LG_IP "$(ip "$LG_VM")"
state_set BEAVER_IP "$(ip "$BEAVER_VM")"
log "LG_IP=$(ip "$LG_VM")  BEAVER_IP=$(ip "$BEAVER_VM")  (saved to state.env)"
log "waiting for ssh via IAP ..."
for vm in "$LG_VM" "$BEAVER_VM"; do
  for _ in $(seq 1 20); do gssh "$vm" true 2>/dev/null && break; sleep 10; done
done
log "03_create_vms done"
