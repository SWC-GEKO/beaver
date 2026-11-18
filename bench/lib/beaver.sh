#!/usr/bin/env bash
# Helpers that run ON THE LOAD-GENERATOR VM and drive the Beaver VM over ssh.
# shellcheck shell=bash
# shellcheck source=common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

BEAVER_SSH_KEY="${BEAVER_SSH_KEY:-$HOME/.ssh/id_bench}"
bsh() {
  ssh -i "$BEAVER_SSH_KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
      -o LogLevel=ERROR -o ServerAliveInterval=30 "bench@${BEAVER_IP:?BEAVER_IP not set (state.env)}" "$@"
}

# Remove every function deployed by a previous run, restart the control plane clean.
beaver_reset() {
  log "beaver: reset (stop control plane, remove old function containers/images, ensure global NATS)"
  bsh 'bash -s' <<'EOS'
set -u
pkill -x controlplane 2>/dev/null || true
docker ps -aq --filter "name=stats-" | xargs -r docker rm -f >/dev/null 2>&1 || true
docker network ls -q --filter "name=stats-" | xargs -r docker network rm >/dev/null 2>&1 || true
docker volume ls -q --filter "name=stats-" | xargs -r docker volume rm >/dev/null 2>&1 || true
docker images --format '{{.Repository}}:{{.Tag}}' | grep '^stats-' | xargs -r docker rmi -f >/dev/null 2>&1 || true
cd ~/beaver && rm -rf internal/controlplane/test-registry tmp compose.yaml
: > ~/cp.log
docker network inspect global-net >/dev/null 2>&1 || docker network create global-net >/dev/null
docker compose -f cmd/global-message-bus/docker-compose.yaml up -d >/dev/null 2>&1
for i in $(seq 1 60); do
  (echo > /dev/tcp/127.0.0.1/4222) >/dev/null 2>&1 && break
  sleep 1
done
# stream FUNCTIONS is created by the nats-init container; give it a moment
sleep 3
EOS
}

beaver_start_cp() {
  log "beaver: starting control plane"
  bsh "BENCH_ROUTER_CPUS=${BEAVER_ROUTER_CPUS} BENCH_ROUTER_MEM_MIB=${BEAVER_ROUTER_MEM_MIB} BENCH_PROCESSOR_CPUS=${BEAVER_PROCESSOR_CPUS} BENCH_PROCESSOR_MEM_MIB=${BEAVER_PROCESSOR_MEM_MIB} bash -s" <<'EOS'
mkdir -p ~/.docker; [ -f ~/.docker/config.json ] || echo '{}' > ~/.docker/config.json   # control plane image build fails without them
cd ~/beaver || exit 1
[ -x ~/bin/controlplane ] || { echo "controlplane binary missing (run bootstrap)" >&2; exit 1; }
(setsid nohup ~/bin/controlplane >> ~/cp.log 2>&1 < /dev/null &)
for i in $(seq 1 60); do
  curl -fs localhost:8080/health >/dev/null 2>&1 && exit 0
  sleep 1
done
echo "control plane did not become healthy; tail of ~/cp.log:" >&2
tail -20 ~/cp.log >&2
exit 1
EOS
}

# beaver_upload <replication> -> prints unique function name
beaver_upload() {
  local repl="$1"
  "$LG_BIN" beaver-upload -cp "http://${BEAVER_IP}:8080" -dir "$BENCH_DIR/beaver-fn" \
      -name stats -replication "$repl" -vshards "$BEAVER_VSHARDS"
}

beaver_processor_count() {   # beaver_processor_count <unique>
  bsh "docker ps --filter name=${1}-processor -q | wc -l" 2>/dev/null | tr -d '[:space:]'
}

beaver_loops_ready() {       # all processors up AND subscribed -> 0
  local unique="$1" want="$2" up ready
  up=$(beaver_processor_count "$unique")
  [ "${up:-0}" -ge "$want" ] || return 1
  ready=$(bsh "for c in \$(docker ps --filter name=${unique}-processor -q); do docker logs \$c 2>&1 | grep -c 'starting event-loop'; done | awk '\$1>0' | wc -l" 2>/dev/null | tr -d '[:space:]')
  [ "${ready:-0}" -ge "$want" ]
}

# Send one event so the control plane starts the function, wait until all processors run.
beaver_prime() {
  local unique="$1" want="$2"
  log "beaver: priming $unique (waiting for $want processors)"
  "$LG_BIN" nats-stream -nats-url "nats://${BEAVER_IP}:4222" -subject "FUNCTIONS.${unique}" \
      -duration 1 -rate 1 -keys 1 >/dev/null 2>&1 || true
  wait_for "$BEAVER_DOCKER_PRIME_TIMEOUT" 3 "processors ready" beaver_loops_ready "$unique" "$want" \
    || die "function $unique did not come up"
  sleep 3
}

# beaver_count_events <unique>  -> number of @@@ lines currently in the processor logs
beaver_count_events() {
  bsh "for c in \$(docker ps -a --filter name=${1}-processor -q); do docker logs \$c 2>&1 | grep -ac '@@@'; done | awk '{s+=\$1} END {print s+0}'" 2>/dev/null | tr -d '[:space:]'
}

beaver_wait_events() {       # beaver_wait_events <unique> <expected> <timeout>
  local unique="$1" expected="$2" timeout="$3" n
  local start; start=$(date +%s)
  while :; do
    n=$(beaver_count_events "$unique"); n="${n:-0}"
    [ "$n" -ge "$expected" ] && { log "beaver: $n/$expected events processed"; return 0; }
    if (( $(date +%s) - start > timeout )); then log "beaver: timeout, only $n/$expected events processed"; return 1; fi
    sleep 3
  done
}

beaver_collect_logs() {      # beaver_collect_logs <unique> <outfile.gz>
  bsh "for c in \$(docker ps -a --filter name=${1}-processor -q); do docker logs \$c 2>&1; done | grep -a '@@@'" | gzip > "$2"
}

beaver_snapshot() {          # beaver_snapshot <unique> <outfile> : container + disk state for the run record
  bsh "docker ps -a --filter name=${1} --format '{{.Names}} {{.Status}}'; df -h / | tail -1" > "$2" 2>&1 || true
}

beaver_cleanup() {
  local unique="$1"
  bsh "docker rm -f \$(docker ps -aq --filter name=${unique}) >/dev/null 2>&1; docker network rm ${unique}-local-net >/dev/null 2>&1; docker volume rm ${unique}-nats-vol >/dev/null 2>&1; docker rmi -f ${unique}:latest >/dev/null 2>&1; true" || true
}

# Resource sampling on the Beaver host while an experiment runs (saturation check)
beaver_sampler_start() {
  bsh 'bash -s' <<'EOS'
rm -f ~/s_vmstat.txt ~/s_dstats.txt
(setsid nohup vmstat -t 5 > ~/s_vmstat.txt 2>&1 < /dev/null & echo $! > ~/s_vmstat.pid)
(setsid nohup bash -c 'while true; do date +%s; docker stats --no-stream --format "{{.Name}} {{.CPUPerc}} {{.MemUsage}}"; sleep 3; done' > ~/s_dstats.txt 2>&1 < /dev/null & echo $! > ~/s_dstats.pid)
EOS
}
beaver_sampler_stop() {      # beaver_sampler_stop <outdir>
  local out="$1"
  # shellcheck disable=SC2016  # remote shell must expand $(...)
  bsh 'kill $(cat ~/s_vmstat.pid ~/s_dstats.pid) 2>/dev/null; true'
  bsh 'cat ~/s_vmstat.txt' > "$out/beaver_vmstat.txt" 2>/dev/null || true
  bsh 'cat ~/s_dstats.txt' 2>/dev/null | gzip > "$out/beaver_dstats.txt.gz" || true
}

# clock_snapshot <outfile> : chrony offset of both VMs (seconds, signed; "nan" if chrony is unavailable).
# Overhead of Beaver / FaaS-Single is computed across two machines' clocks; analyze.py warns above 1 ms.
clock_snapshot() {
  # shellcheck disable=SC2016  # awk program is meant to be single-quoted
  local awkp='/^System time/ {v=$4; if ($6=="slow") v=-v; print v; f=1} END {if(!f) print "nan"}'
  {
    printf 'lg %s\n' "$(chronyc tracking 2>/dev/null | awk "$awkp" || echo nan)"
    printf 'beaver %s\n' "$(bsh "chronyc tracking 2>/dev/null | awk '$awkp'" 2>/dev/null || echo nan)"
  } > "$1"
}
