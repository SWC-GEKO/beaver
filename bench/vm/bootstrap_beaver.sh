#!/usr/bin/env bash
# Prepares the BEAVER VM: docker, router image, control plane binary, global NATS.
set -euo pipefail
# shellcheck source=../lib/common.sh
source "$(dirname "$0")/../lib/common.sh"
"$BENCH_DIR/vm/bootstrap_common.sh"
cd "$REPO_DIR"

log "pre-pulling base images (so image pulls never count as cold start)"
for i in nats:latest natsio/nats-box:latest golang:1.26-alpine; do sudo docker pull -q "$i" >/dev/null; done

log "building stateless-router:latest (quiet=${BEAVER_ROUTER_QUIET})"
tmp="$(mktemp -d)"
cp -r internal/docker/runtime/router/. "$tmp/"
if [ "$BEAVER_ROUTER_QUIET" = "1" ]; then
  before=$(grep -c 'log\.' "$tmp/router.go")
  sed -i -e '/log.Printf("consumed message/d' -e '/log.Printf("calculated shard/d' \
         -e '/log.Printf("%+v", pubMsg)/d' -e '/log.Println("published msg to local NATS")/d' "$tmp/router.go"
  after=$(grep -c 'log\.' "$tmp/router.go")
  log "router.go: removed $((before-after)) per-message log statements (expected 4)"
  [ $((before-after)) -eq 4 ] || log "WARNING: router.go changed upstream, check the sed patterns in vm/bootstrap_beaver.sh"
fi
sudo docker build -q -t stateless-router:latest "$tmp" >/dev/null
rm -rf "$tmp"

log "building control plane (quiet=${BEAVER_QUIET}, resource limits via BENCH_* env)"
# The three files are patched IN PLACE (same module/build context as an unpatched build) and restored
# from backups right after the build, even on failure; nothing stays modified in $REPO_DIR.
# Patches (all disclosed in README section "Changes to Beaver while measured"):
#   1. internal/controlplane/bench_resources.go (new): applies cpu/memory limits from env
#      BENCH_ROUTER_CPUS/_MEM_MIB and BENCH_PROCESSOR_CPUS/_MEM_MIB to every service of the
#      generated compose project (router bigger than processors). The NATS container is untouched.
#   2. function.go: one call benchApplyResources(services) just before the project is assembled.
#   3. observer.go (BEAVER_QUIET=1): the per-event log.Printf("received message in subject") is removed.
cpsrc="$REPO_DIR"
bak="$(mktemp -d)"
cp "$cpsrc/internal/controlplane/function.go" "$cpsrc/internal/controlplane/observer.go" "$bak/"
restore_cp() {
  cp "$bak/function.go" "$bak/observer.go" "$cpsrc/internal/controlplane/"
  rm -f "$cpsrc/internal/controlplane/bench_resources.go"; rm -rf "$bak"
}
trap restore_cp EXIT
cat > "$cpsrc/internal/controlplane/bench_resources.go" <<'GOEOF'
package controlplane

import (
	"os"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
)

func benchEnvFloat(name string, def float64) float64 {
	if v := os.Getenv(name); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// benchApplyResources sets cpu / memory limits on the services of a function project.
// 0 means "no limit". Defaults: router 4 CPU / 2048 MiB, processors 1 CPU / 512 MiB.
func benchApplyResources(services map[string]types.ServiceConfig) {
	rCPU := benchEnvFloat("BENCH_ROUTER_CPUS", 4)
	rMem := benchEnvFloat("BENCH_ROUTER_MEM_MIB", 2048)
	pCPU := benchEnvFloat("BENCH_PROCESSOR_CPUS", 1)
	pMem := benchEnvFloat("BENCH_PROCESSOR_MEM_MIB", 512)
	for name, s := range services {
		var cpu, mem float64
		switch {
		case name == "router":
			cpu, mem = rCPU, rMem
		case strings.Contains(name, "-processor-"):
			cpu, mem = pCPU, pMem
		default:
			continue
		}
		if cpu > 0 {
			s.CPUS = float32(cpu)
		}
		if mem > 0 {
			s.MemLimit = types.UnitBytes(int64(mem) * 1024 * 1024)
		}
		services[name] = s
	}
}
GOEOF
fn="$cpsrc/internal/controlplane/function.go"
grep -c 'project := types.Project{' "$fn" | grep -qx 1 || die "function.go: expected exactly one 'project := types.Project{' (upstream changed; adapt vm/bootstrap_beaver.sh)"
grep -q 'services\b' "$fn" || die "function.go: no 'services' variable found (upstream changed)"
sed -i 's|^\(\s*\)project := types.Project{|\1benchApplyResources(services)\n\1project := types.Project{|' "$fn"
grep -q 'benchApplyResources(services)' "$fn" || die "patch of function.go failed"
if [ "$BEAVER_QUIET" = "1" ]; then
  obs="$cpsrc/internal/controlplane/observer.go"
  b=$(grep -c 'log\.' "$obs")
  sed -i '/log.Printf("received message in subject/d' "$obs"
  a=$(grep -c 'log\.' "$obs")
  [ $((b-a)) -eq 1 ] || die "observer.go: expected to remove exactly 1 log line, removed $((b-a)) (upstream changed)"
fi
unformatted="$(cd "$cpsrc" && gofmt -l internal/controlplane)"
[ -z "$unformatted" ] || die "gofmt complains about patched files: $unformatted"
( cd "$cpsrc" && go build -o "$HOME/bin/controlplane" ./cmd/controlplane ) || die "control plane build failed (see error above). If it is a dependency error (e.g. archive.Gzip), the unpatched repo does not build either: run  cd ~/beaver && go build ./cmd/controlplane  to check."
restore_cp; trap - EXIT

log "starting global NATS"
sudo docker network inspect global-net >/dev/null 2>&1 || sudo docker network create global-net >/dev/null
sudo docker compose -f cmd/global-message-bus/docker-compose.yaml up -d
sleep 5
sudo docker ps --format '{{.Names}}\t{{.Status}}'
log "beaver VM ready"
