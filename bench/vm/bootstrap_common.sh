#!/usr/bin/env bash
# Installs the toolchain on a fresh Debian 12 VM (both VMs). Idempotent.
set -euo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"

log "apt packages"
sudo apt-get update -qq
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git curl jq tmux unzip zip ca-certificates procps \
  redis-tools python3 >/dev/null

if ! command -v docker >/dev/null; then
  log "installing docker"
  curl -fsSL https://get.docker.com | sudo sh >/dev/null
fi
sudo usermod -aG docker "$USER"
mkdir -p "$HOME/.docker"
if ! command -v chronyc >/dev/null; then
  log "installing chrony (clock offset check) with the GCE metadata time server"
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq chrony >/dev/null
  echo 'server metadata.google.internal iburst minpoll 4 maxpoll 4' | sudo tee /etc/chrony/sources.d/gce.sources >/dev/null 2>&1 || \
    echo 'server metadata.google.internal iburst minpoll 4 maxpoll 4' | sudo tee -a /etc/chrony/chrony.conf >/dev/null
  sudo systemctl restart chrony || sudo systemctl restart chronyd || true
fi
[ -f "$HOME/.docker/config.json" ] || echo '{}' > "$HOME/.docker/config.json"   # the control plane's image build needs both

# local log driver = cheaper than json-file and size-capped; `docker logs` keeps working
if ! grep -q '"local"' /etc/docker/daemon.json 2>/dev/null; then
  echo '{"log-driver":"local","log-opts":{"max-size":"512m","max-file":"3"}}' | sudo tee /etc/docker/daemon.json >/dev/null
  sudo systemctl restart docker
fi

if ! /usr/local/go/bin/go version 2>/dev/null | grep -q "go${GO_VERSION}"; then
  log "installing Go ${GO_VERSION}"
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz
  sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tgz && rm /tmp/go.tgz
fi
mkdir -p "$HOME/bin"
log "common bootstrap done: $(/usr/local/go/bin/go version)"
