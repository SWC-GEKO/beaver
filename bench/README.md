# Beaver benchmark toolkit (GCP, europe-west3)

Provisions the cloud resources, runs the three benchmarks, collects the logs and draws the plots.

| Experiment | What | Systems | Output |
|---|---|---|---|
| 1 | Cold-start penalty | Beaver, FaaS-Single (Cloud Run + Valkey), FaaS-Batched (Cloud Run), Beam on Dataflow | `coldstart_penalty.pdf` (θ lines), `coldstart_duration.pdf` |
| 2 | Processing overhead over theoretical minimum | same four | `overhead.pdf` |
| 3 | Scalability, 1/2/4/8 shards × 2…1024 concurrent streams | Beaver only | `scalability_{throughput,latency_p50,latency_p99,error_rate,cpu,overview}.pdf` (x = offered events/s) |

Exp. 1 and 2 use the same runs (4 systems × durations 10/30/60 s × repetitions).

## 0. Install

All commands are run from `bench/` (the scripts expect `../evaluation/...` next to it).
Laptop needs: `gcloud` (logged in: `gcloud auth login`), `ssh`, `tar`, `python3` with `pandas matplotlib numpy`.
Copy `config.env.example` to `config.env` and set `PROJECT_ID`; region defaults to `europe-west3`. Everything is overridable per call.

## 1. Provision (once, ~30 min, mostly waiting)

```bash
cd bench
./bench.sh setup        # APIs, VPC, firewall, service account, bucket, Artifact Registry   (~2 min)
./bench.sh valkey       # Memorystore for Valkey                                             (~10 min)
./bench.sh vms          # load-generator VM + Beaver VM (+ ssh key in .secrets/)             (~3 min)
./bench.sh bootstrap    # uploads the repo, installs Go/docker, builds binaries, router image,
                        # control plane, Cloud Run images and services                       (~15 min)
```
All steps are idempotent: if one fails, fix the cause and run it again.

## 2. Smoke test (do this before anything else, ~30–45 min, small cost)

```bash
./bench.sh smoke      # runs detached in tmux on the VM and polls; safe against dropped ssh connections
```
Runs one tiny run per system plus two Exp. 3 configs (`PROFILE=test`), downloads the results and prints a per-run check.
**Every line must say `OK`** (saturation notes in Exp. 3 at high load are expected). Typical findings:

| Finding | Meaning / fix |
|---|---|
| `NO EVENTS in logs` (dataflow) | Cloud Logging filter does not match the worker logs. Inspect: `gcloud logging read 'resource.type="dataflow_step" AND resource.labels.job_name="<job from results/smoke/exp12/dataflow/d10/r1/meta.json>"' --limit 5 --format json`, then adjust the filter in `experiments/exp12_run.sh` (`run_dataflow`). |
| `NOT COLD` (faas-*) | A warm instance survived. Raise `CR_IDLE_WAIT` in `config.env` and re-run (delete `results/<campaign>/exp12/faas-*` first, finished runs are skipped). The default 960 s is the documented 15-min idle limit plus a margin. |
| `negative tReceived-tCreated` | Clock skew between VMs and Cloud Run/Dataflow. Fine for cold start (seconds), too coarse for Beaver's ms-level overhead; use a loadgen-clock-only overhead metric in that case. |
| `clock offset between VMs … ms` | Every run stores the `chronyc tracking` offset of both VMs in `clock.txt`; the check warns above 1 ms (Beaver runs). Cloud Run and Dataflow clocks cannot be measured this way. |
| `LOAD GENERATOR busy >70 %` | Exp. 3 plateau is the generator, not Beaver: bigger `LG_MACHINE`, or lower `STREAM_RATE`. |

**Cloud Run cold-start calibration (once, ~40 min):** `./bench.sh calibrate-cr` deploys fresh revisions, waits 2, 5, 10 and 15 minutes (`CALIBRATE_WAITS`), calls each once and reports which were cold. Set `CR_IDLE_WAIT` to the smallest cold wait plus ~20 %.

## 3. Final campaign (unattended, hours)

```bash
PROFILE=final ./bench.sh push          # picks up config changes
PROFILE=final ./bench.sh run all       # Exp 1/2 then Exp 3, in a tmux session on the VM (detached!)
```
You can close the laptop. The campaign runs inside `tmux` on the load-generator VM and survives ssh drops.

```bash
./bench.sh status      # runs done / incomplete, last log lines
./bench.sh logs        # follow the campaign log
./bench.sh attach      # live tmux view (detach: Ctrl-b d)
./bench.sh stop        # abort (the current run is lost, finished ones stay)
```
**Resume after any failure:** run the same `run` command again — finished runs (`meta.json` with `status: ok`) are skipped.
Run only one experiment: `PROFILE=final ./bench.sh run exp12` or `run exp3`. Narrow the matrix with env vars, e.g.
`PROFILE=final SYSTEMS="beaver dataflow" DURATIONS=60 REPS_EXP12=5 ./bench.sh run exp12`.
Runs go repetition-major (rep 1 of everything, then rep 2 …), so drift is spread across systems.

Rough time (estimates, not measured): Exp 1/2 ≈ 10–12 h with 10 reps (Dataflow ≈ 10–15 min per run dominates; `REPS_EXP12=5` halves it), Exp 3 ≈ 5–6 h.

```bash
./bench.sh fetch && ./bench.sh analyze          # CAMPAIGN=final by default for PROFILE=final
# -> figures/final/*.pdf (vector, for LaTeX), exp12_runs.csv, exp12_summary.csv, exp3_runs.csv, exp3_summary.csv
```
Between sessions: `./bench.sh vms-stop` / `vms-start`. When completely finished and results are fetched: `./bench.sh teardown` (add `--all` to also remove VPC, bucket, registry, service account).
Valkey, Dataflow jobs and Cloud Run are only billed while they exist/run; the teardown deletes them.

## 4. How a run works

* **Beaver:** per run the control plane is restarted, old function containers/images removed, a *new* function uploaded (image built, not running), then the first event triggers `docker compose up` → real cold start. Logs: `docker logs` of the processors.
* **FaaS-Single/Batched:** a fresh Cloud Run revision always has one instance (started by the deploy) that lives up to ~15 min, so a revision is only cold after it has idled out. To avoid waiting 15 min before *every* run, `experiments/exp12_predeploy.sh` deploys **all revisions of the matrix up front** (no traffic, one tag each, e.g. `d30r4`); they idle out together, so the campaign waits once. Each run calls its own revision through the tag URL, sleeps only if that revision is younger than `CR_IDLE_WAIT` (default 960 s), runs `FLUSHALL` on Valkey (Single), and uses it exactly once (the record is consumed; a repeated run falls back to deploy + wait). The container logs `BENCH_BOOT <ns>`; a run counts as cold only if a boot happened after t0 (`cold_verified`). Expected overhead: ~45 min of deploys + one ~15 min idle period for the final matrix.
* **Dataflow:** per run a fresh Pub/Sub topic+subscription, then `beam-bench` takes t0, starts publishing and submits the job (`--execute_async`, 1 worker). No job exists beforehand. Job is cancelled and Pub/Sub deleted afterwards.
* **Exp. 3:** per (shards, streams, rep) a fresh Beaver function with `replication = shards`, `vshards = 128` fixed; function is primed (containers up) before the clock starts; N streams × `STREAM_RATE` events/s with uniform random keys from 4096; warm-up → measurement window → cool-down → drain. Throughput = events *processed* inside the window ÷ window length. CPU of both VMs is sampled (`vmstat`) for the saturation check.

## 5. Metric definitions

* `t0` = start of the stream on the load generator. Event timestamps `tCreated` (load generator) and `tReceived/tProcessed` (function side) come from the function's `@@@` log lines.
* **Cold-start penalty θ** = `(t1 − t0) / (t2 − t0)` with `t1 = min tReceived`, `t2 = max tProcessed`, `t0` = stream start (as in the thesis notebook). For **FaaS-Batched** `t0 = max tCreated` (the batch is only sent after the stream ended; the notebook's `var_analyze`). `coldstart_duration.pdf` shows the absolute `t1 − t0`.
* **Processing overhead** (default `--overhead-def notebook`) = `(t2 − t0) − (max tCreated − min tCreated)`, i.e. total duration minus the stream's own span (the notebook's `processing_overhead`; theoretical minimum = the stream duration itself). Alternatives: `total` = `t2 − t0` literally, `last` = `t2 − max tCreated`, `mean` = mean per-event latency.
* **Exp. 3** (per run, measurement window only): offered = events sent ÷ window (x axis); throughput = events processed ÷ window; latency p50/p99 = `tProcessed − tCreated`; **error rate** = (publish errors + events sent but never processed) ÷ (sent + publish errors); CPU busy of the Beaver VM from `vmstat`. Bars/markers = median over repetitions, whiskers = min/max.
* Plot style (`analysis/plotstyle.py`): ACM sizes (3.33×2.06 / 7.00×2.06 in), palette `#2D3250 #7077A1 #F6B17A #662549` (Beaver `#F6B17A`, FaaS `#7077A1`, Batch `#2D3250`, Stream `#662549`; shards 1/2/4/8 use the same four in order), hatched bars, `$10^k$` log ticks, PDF with embedded fonts. LaTeX text + Linux Libertine is used automatically if `libertine.sty` is installed (`texlive-fonts-extra`); otherwise a serif fallback. `--usetex on|off|auto`. If LaTeX is installed but incomplete the script falls back automatically.
* Dataflow job start-up is part of its penalty/overhead by design (no pre-running job).

## 6. Deviations from the original evaluation code

1. **`evaluation/beaver/stats.go` has a data race.** The processor starts one goroutine per virtual shard, all calling the *same* `Function`, whose `KeyState` map is unguarded → `fatal error: concurrent map writes` under load. The benchmark uploads `bench/beaver-fn/` instead (identical logic + a mutex, diff documented in the file header).
2. **Router logs 5 lines per event** (`router.go`). At thousands of events/s this can bottleneck on the docker log driver, not on Beaver. `config.env: BEAVER_ROUTER_QUIET=1` (default) builds the router image with those 4 per-message `log` statements stripped (the image is built from a temp copy; repo untouched). Set it to `0` to measure the original.
2b. **Control plane built with in-place patches that are reverted right after the build** (`vm/bootstrap_beaver.sh`; nothing stays modified in the clone):
    * `BEAVER_QUIET=1` (default) removes the per-event `log.Printf("received message in subject …")` of `observer.go` (same reason as the router). `~/cp.log` is truncated at every run reset.
    * **Container sizes of a deployed function.** A new file `bench_resources.go` plus one call in `createFunctionProject` set CPU/memory limits on the generated compose project. **The router is deployed in a bigger container than the processors** so that it is never the bottleneck: router `BEAVER_ROUTER_CPUS=4` / `BEAVER_ROUTER_MEM_MIB=2048`, each processor `BEAVER_PROCESSOR_CPUS=1` / `BEAVER_PROCESSOR_MEM_MIB=512` (0 = unlimited). The function's local NATS container is not limited. Consequence: a processor can use at most one core, and 8 processors + router need up to 12 vCPUs (`e2-standard-16` has 16). Without this patch Beaver would run every container unlimited, so any limit is a deliberate change of what is measured. If the control plane build fails with a patch error, upstream changed: adapt the patterns in the script.
3. `evaluation/beam/cmd/main.go` → `bench/beam/` : constants became flags, ADC instead of `keys.json`, t0 taken before job submission, publisher made single-threaded (the original shared one `rand.Rand` between goroutines, ids could repeat), process waits for the publisher so `--execute_async` works.
4. The original load generators had races/nil-derefs (`msgId++` in goroutines, `resp.StatusCode` after a failed request). They are replaced by `bench/loadgen` (`lg`), same JSON payloads and `Key` header.
5. **Cloud Run, not `gcloud functions deploy`:** the same functions-framework HTTP handlers run in a container on Cloud Run (a 2nd-gen Cloud Function *is* a Cloud Run service). This avoids buildpack/Go-version problems; `cmd/main.go` is swapped for a `$PORT`-aware one at build time. `CR_CONCURRENCY=1` = classic FaaS semantics. Max instances: `CR_MAX_INSTANCES_SINGLE=20`, `CR_MAX_INSTANCES_BATCH=3`. `CR_AUTH=public` (default) allows unauthenticated calls, so no token is fetched on the hot path.
6. Exp. 3 "concurrent requests" is implemented as **N concurrent streams, each at a fixed rate** (open loop); the plots use the **measured offered load [events/s]** as x axis. Beaver has no request/response path, so a closed loop would need code changes. If the curves never plateau, raise `STREAM_RATE`; if the generator saturates, lower it or enlarge `LG_MACHINE`.
7. The Beaver function image is built against the pinned SDK in `bench/beaver-fn/go.mod` (same pin as `evaluation/beaver/go.mod`, an older commit than `main`).

## 7. Common first-run problems

The analysis pipeline can be tested offline against synthetic data (`python3 analysis/selftest.py`). Likely first-run problems on a fresh project:
the patched control-plane build, Valkey PSC flags (`02_create_valkey.sh`, endpoint IP lookup), Dataflow log filter, Dataflow machine type acceptance (`DF_MACHINE`), Go `1.26.1` download URL (`GO_VERSION`), `--vpc-egress` / Direct VPC egress on Cloud Run, IAM roles. The smoke test exists to flush these out cheaply.

## 8. Layout

```
bench.sh            laptop control script          config.env   all knobs (PROFILE=test|final)
infra/              project setup, Valkey, VMs, teardown
vm/                 bootstrap scripts that run on the VMs
deploy/cloudrun/    Dockerfile + build/deploy of the two FaaS services
experiments/        exp12_run.sh, exp3_run.sh, *_all.sh (run on the load-generator VM)
loadgen/            lg: nats-stream | nats-scale | http-single | http-batch | beaver-upload
beam/               Dataflow pipeline + publisher ("beam-bench")
beaver-fn/          mutex-patched Beaver test function
lib/                shell helpers, Cloud Logging collector
analysis/           analyze.py (tables, PDF figures, --check), plotstyle.py (shared style), selftest.py
```

## 9. Re-running failed runs / extending Exp. 3

* **Failed FaaS runs** (`NO EVENTS` + `NOT COLD`): Cloud Run keeps one instance of every fresh revision alive for ~15 min; the predeploy created 20 revisions at once and the 21st deploy failed (regional CPU quota), after which the fallback path deployed an untagged revision that received 0 % traffic. Fixed: `CR_PREDEPLOY_MAX` (default 16) caps the revisions alive at once, the fallback deploys with a tag. Re-run only the unfinished runs (finished ones are skipped):
  `PROFILE=final REPS_EXP12=5 SYSTEMS="faas-single faas-batch" ./bench.sh run exp12`
* **Exp. 3 beyond saturation:** `PROFILE=final CONCURRENCY="128 512 2048 4096 8192" REPS_EXP3=1 ./bench.sh run exp3`, then the same with `REPS_EXP3=3` (rep 1 is skipped). `./bench.sh resize-lg e2-standard-16` first if `analyze --check` reports `LOAD GENERATOR busy`.
* Throughput definitions and figures: see `analysis/analyze.py` header; `scalability_sustainable.pdf` / `exp3_max_sustainable.csv` use `--slo-p99-ms` (default 1000).
