<p>
   <img src="resources/img.png" alt="Beaver logo"/>
</p>

<h1 align="center">Beaver</h1>

<p align="center"><b>Serverless stream processing for short-running, lightweight streams.</b><br/>
FaaS-style deployment and scale-to-zero, plus per-key state and data locality through virtual sharding.</p>

---

Beaver is a research prototype from the [Scalable Software Systems (3S)](https://www.tu.berlin/3s) group at TU Berlin. It builds on the *Stream functions* model, in which one function instance handles a whole stream instead of one event. Beaver removes that model's scaling limit, a single instance per stream, with **function sharding**. Events are hashed by key onto a fixed number of *virtual shards*. Those shards are spread over a configurable number of *physical shards* (processor containers). Each processor keeps the state for its keys in memory, so a function can hold rolling aggregates per key without an external database.

Typical workload: many short streams that each last seconds to minutes and carry low event rates, for example security-camera feeds, IoT sessions or user sessions. Classic stream processors (Flink, Beam/Dataflow) start too slowly for these streams. Classic FaaS has no stream semantics and keeps no state between events.

## Contents
- [Architecture](#architecture)
- [Programming model](#programming-model)
- [Quickstart (local, single host)](#quickstart-local-single-host)
- [Sending events](#sending-events)
- [Configuration](#configuration)
- [Evaluation](#evaluation)
- [Repository layout](#repository-layout)
- [Status and known limitations](#status-and-known-limitations)
- [Roadmap](#roadmap)

## Architecture

```
                 ┌────────────────────────── Control Plane (:8080) ──────────────────────────┐
  sdk.Runtime ──►│ POST /upload → build processor image → save FunctionRecord (YAML registry)│
                 │ Observer: watches FUNCTIONS.> on the global bus, starts a function on its │
                 │ first event (docker compose up), stops it after 10 min without events    │
                 └───────────────────────────────────────────────────────────────────────────┘
                                                │ starts / stops
  producers ──► Global NATS JetStream           ▼
                stream FUNCTIONS          ┌─────────── per-function Compose project ───────────┐
                subject FUNCTIONS.<fn> ──►│ Router ──xxhash(Key) % vShards──► local NATS        │
                                          │                 function.0 … function.<vShards-1>  │
                                          │   ┌───────────────┬───────────────┐                │
                                          │   ▼               ▼               ▼                │
                                          │ processor-0   processor-1  …  processor-(R-1)      │
                                          │ (subset of virtual shards each, in-memory state)   │
                                          │   └──► function.out / function.dlq (local NATS)    │
                                          └────────────────────────────────────────────────────┘
```

| Component | Code | Role |
|---|---|---|
| **Control plane** | `cmd/controlplane`, `internal/controlplane` | HTTP API (`/health`, `/upload`). Builds the processor image from the uploaded code, stores a `FunctionRecord` in a file-based registry and registers the function with the observer. |
| **Observer** | `internal/controlplane/observer.go` | A JetStream consumer on `FUNCTIONS.>`. Starts a function on its first event (cold start) and stops it when idle. The function states are `Idle → Active → Stopping`. |
| **Global message bus** | `cmd/global-message-bus` | A NATS server with JetStream and the `FUNCTIONS` stream (15 min retention). This is the entry point for all events. |
| **Router** | `internal/docker/runtime/router` | One per function. Consumes `FUNCTIONS.<uniqueName>`, reads the `Key` header and publishes each event to `function.<xxhash(key) mod vShards>` on the function's local NATS. |
| **Processor** | `internal/docker/runtime/processor` | Wraps the user function. Subscribes to its virtual-shard topics, with one event loop per shard, and calls `Exec`. It publishes results to `function.out` and failures to `function.dlq`. |
| **Local NATS** | generated | A private message bus for each function on its own bridge network. |

Virtual shards are spread evenly over the `replication` processors by `calcTopics` (`internal/controlplane/function.go`). With `vShards = 128` and `replication = 4`, each processor serves 32 virtual shards. Because the number of virtual shards is fixed at deploy time, the design leaves room for future elastic scaling: a virtual shard and its state can move to another processor without rehashing.

## Programming model

The only interface you implement is `api.Function` (`spec/api`):

```go
type Event struct {
    Headers map[string]string
    Body    []byte
}

type Function interface {
    Exec(ctx context.Context, event *Event) (*Event, error)
}
```

**One function = one directory = one Go module**, written in `package main`. It must contain an `init()` that registers the function, and it must **not** contain a `main()`, because the platform links your code with its processor `main`.

```go
// myfn/function.go
package main

import (
    "context"
    "sync"

    beaver "github.com/SWC-GEKO/beaver/sdk"
    "github.com/SWC-GEKO/beaver/spec/api"
)

func init() {
    beaver.RegisterFunction("counter", &Counter{counts: map[string]int{}})
}

type Counter struct {
    mu     sync.Mutex      // REQUIRED: Exec is called concurrently (one goroutine per virtual shard)
    counts map[string]int  // per-key state, lives in the processor's memory
}

func (c *Counter) Exec(ctx context.Context, e *api.Event) (*api.Event, error) {
    key := e.Headers["Key"]

    c.mu.Lock()
    c.counts[key]++
    c.mu.Unlock()

    return &api.Event{Headers: e.Headers, Body: e.Body}, nil
}
```

```
// myfn/go.mod
module myfn

go 1.26

require github.com/SWC-GEKO/beaver v0.0.0-<commit>
```

> ⚠️ **Concurrency:** a processor serves several virtual shards and runs one goroutine per shard, and all of them share **one** instance of your function. Any state in your struct must be synchronised. Events with the same key always land on the same shard, so they are processed in order relative to each other.

A full example is the rolling mean and standard deviation per key (Welford) in [`bench/beaver-fn/stats.go`](bench/beaver-fn/stats.go).

## Quickstart (local, single host)

Requirements: Go ≥ 1.26, Docker with Compose v2. The control plane resolves paths relative to the repository root, so run it from there.

```bash
# 1. Shared network + global message bus (creates the FUNCTIONS stream)
docker network create global-net
docker compose -f cmd/global-message-bus/docker-compose.yaml up -d

# 2. Router image (the name is referenced by every function record)
docker build -t stateless-router:latest internal/docker/runtime/router

# 3. Control plane on :8080 (expects NATS on localhost:4222)
go run ./cmd/controlplane
```

**Deploy a function** with the SDK. This is a small program outside the function directory:

```go
// deploy/main.go
package main

import beaver "github.com/SWC-GEKO/beaver/sdk"

func main() {
    rt := beaver.NewRuntime("127.0.0.1", "8080")

    // name, path to the function directory, replication (= physical shards), virtual shards
    rt.Add("counter", "/abs/path/to/myfn", 4, 128)

    if err := rt.Start(); err != nil { // zips the directory and POSTs it to /upload
        panic(err)
    }
}
```

The control plane builds the image `<name>-<uuid>:latest` and stores the record in `internal/controlplane/test-registry/<name>-<uuid>.yaml`. The function is **not running yet**: it starts on its first event. Find the generated unique name in the control-plane log or in the registry directory. (The `/upload` response body also contains it, but the SDK does not return it yet.)

## Sending events

Publish to the global JetStream subject `FUNCTIONS.<uniqueName>` and set the **`Key`** header. The key decides the virtual shard and therefore which state the event sees.

```go
nc, _ := nats.Connect("nats://127.0.0.1:4222")
js, _ := jetstream.New(nc)

msg := nats.NewMsg("FUNCTIONS.counter-<uuid>")
msg.Header.Set("Key", "camera-17")
msg.Data = []byte(`{"value": 42}`)

_, err := js.PublishMsg(ctx, msg)
```

The first event triggers the cold start (`docker compose up` of local NATS, router and processors). When a function receives no events for **10 minutes** it is stopped (`idleTimeout` in `observer.go`).

**Output:** results go to `function.out` and failures to `function.dlq` on the function's *local* NATS, which only that function's network can reach. No consumer outside the function can read them yet (see [limitations](#status-and-known-limitations)). The benchmark function therefore reports results through structured `@@@ … @@@` log lines.

## Configuration

| Setting | Where | Default |
|---|---|---|
| Control-plane address | `cmd/controlplane/main.go` (`addr`) | `:8080` |
| Global NATS URL / stream | `cmd/controlplane/main.go` (`natsUrl`, `stream`) | `:4222` / `FUNCTIONS` |
| Registry directory | `cmd/controlplane/main.go` (`dir`) | `internal/controlplane/test-registry` |
| Build scratch directory | `internal/controlplane`, `internal/docker` (`TmpDir`) | `./tmp` |
| Router / NATS images, network names | `DefaultRecord` in `internal/controlplane/registry.go` | `stateless-router:latest`, `nats:latest`, `global-net` |
| Idle timeout before stop | `internal/controlplane/observer.go` | 10 min, checked every 30 s |
| Physical / virtual shards | per function, `rt.Add(name, path, replication, vShards)` | — |

## Evaluation

[`bench/`](bench/README.md) is a reproducible benchmark toolkit for GCP (`europe-west3`). It provisions the infrastructure, runs three experiments unattended, collects the logs and produces the PDF figures used in the thesis. Beaver is compared with Cloud Run functions plus Memorystore/Valkey for state (*FaaS-Single*), Cloud Run functions with all events batched into one request (*FaaS-Batched*), and Apache Beam on Cloud Dataflow.

Results of the final campaign (5 repetitions each, median; from `bench/figures/final/*.csv`):

| System | Cold-start delay t₁−t₀ (60 s stream) | Processing overhead (60 s stream) |
|---|---|---|
| **Beaver** | ≈ 2.5 s | ≈ 102 ms |
| FaaS-Single (Cloud Run + Valkey) | ≈ 0.66 s | ≈ 107 ms |
| FaaS-Batched (Cloud Run) | ≈ 0.2 s after the stream ends¹ | ≈ 305 ms |
| Beam on Dataflow | ≈ 96 s | ≈ 37 s |

¹ The batch can only be sent once the stream has ended, so the first result appears only after the whole stream duration.

- **Cold start:** about 97 % lower than Dataflow, while keeping per-key state without an external store. Cloud Run's managed cold start is still faster (see roadmap).
- **Scalability (Exp. 3, processors capped at 1 vCPU each):** the highest sustainable offered load (p99 < 1 s) grows with the number of physical shards: ≈ 1.3k / 2.6k / 5.1k / 20.5k events/s for 1 / 2 / 4 / 8 shards. Without CPU caps, a single processor already sustains the generator's maximum of ≈ 10k events/s with p99 ≈ 20 ms.

See [`bench/README.md`](bench/README.md) for setup, metric definitions and every deviation from the original code that the benchmark makes (CPU limits, quiet logging, the patched test function).

## Repository layout

```
cmd/controlplane/          control-plane entry point (HTTP API)
cmd/global-message-bus/    docker-compose for the global NATS JetStream bus
internal/controlplane/     control plane, observer, function lifecycle, file registry
internal/docker/           image build + compose runner; runtime/{router,processor} sources
internal/utils/            zip / copy helpers
sdk/                       client SDK: RegisterFunction, Runtime (upload)
spec/api, spec/contracts   public function interface, upload contract
evaluation/                original evaluation functions and load generators (thesis)
bench/                     GCP benchmark toolkit: infra, experiments, load generator, analysis
```

## Status and known limitations

Beaver is a **research prototype**. Known gaps, roughly by importance:

- **Single host only.** Functions run as Docker Compose projects on the control-plane machine. There is no multi-node placement.
- **No elastic scaling.** `replication` and `vShards` are fixed at deploy time. The design allows migrating virtual shards, but this is not implemented.
- **State is volatile.** Per-key state lives in processor memory and is lost when the function idles out, restarts or crashes. There is no checkpointing.
- **Delivery semantics.** The router acks the global JetStream message as soon as it has published to *core* NATS on the local bus. If a processor is not subscribed yet (during a cold start) or crashes, events can be lost. On restart, the router's non-durable consumer replays the global stream (up to its 15 min retention), so events can also be processed twice. The `SEQ` start sequence that the observer passes to the router is not used yet.
- **No output path.** `function.out` stays on the function's private network.
- **Concurrent cold starts.** `RunProject` writes every project to the same `./compose.yaml`, so two functions starting at the same moment can race.
- **Operational rough edges.** The router image must be built by hand. Paths and URLs are hard-coded constants. `/upload` returns the plain name instead of `contracts.UploadResponse`, and the SDK ignores it. Images are built on the control plane with the full Go toolchain (`go mod tidy` at build time), which makes images large and slows down cold starts. Shutdown is basic. Tests depend on absolute local paths.
