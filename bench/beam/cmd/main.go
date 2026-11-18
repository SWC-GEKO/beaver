// beam-bench: the Dataflow benchmark = Beam pipeline + load generator in ONE binary.
//
// Derived from evaluation/beam/cmd/main.go. Changes (measurement-relevant ones marked *):
//   - constants -> flags (-bench_project, -bench_topic, ...)
//   - credentials: Application Default Credentials (VM service account), no keys.json
//   - * t0 is taken right before the job is submitted, so the cold-start penalty of the
//     "no job running beforehand" setup includes staging, submission and worker start-up
//   - * the publisher loop is single-threaded (the original shared one rand.Rand between
//     goroutines, which is a data race, and message ids could repeat)
//   - the process waits for the publisher so that --execute_async can be used
//
// Usage (see experiments/exp12_run.sh):
//
//	beam-bench -bench_project P -bench_topic T -bench_subscription S -bench_duration 10 \
//	  -bench_rate 10 -bench_keys 32 -bench_meta meta.json \
//	  --runner=dataflow --project=P --region=europe-west3 --staging_location=gs://... \
//	  --temp_location=gs://... --job_name=... --execute_async --max_num_workers=1 ...
package main

import (
	"beam/pkg"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/pubsub/v2"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/io/pubsubio"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/x/beamx"
)

var (
	benchProject = flag.String("bench_project", "", "GCP project (Pub/Sub)")
	benchTopic   = flag.String("bench_topic", "", "Pub/Sub topic id to publish to")
	benchSub     = flag.String("bench_subscription", "", "Pub/Sub subscription id the job reads from")
	benchDur     = flag.Int("bench_duration", 10, "stream duration [s]")
	benchRate    = flag.Int("bench_rate", 10, "events per second")
	benchKeys    = flag.Int("bench_keys", 32, "number of distinct keys")
	benchSeed    = flag.Int64("bench_seed", 42, "PRNG seed")
	benchMeta    = flag.String("bench_meta", "", "write meta json here")
)

type meta struct {
	Mode        string `json:"mode"`
	T0NS        int64  `json:"t0_ns"`
	TEndNS      int64  `json:"t_end_ns"`
	DurationSec int    `json:"duration_sec"`
	Rate        int    `json:"rate"`
	Keys        int    `json:"keys"`
	Seed        int64  `json:"seed"`
	Sent        int64  `json:"sent"`
	Errors      int64  `json:"errors"`
	LastCreated int64  `json:"t_last_created_ns"`
}

func init() { beam.RegisterFunction(keyFn) }

func keyFn(b []byte) (string, pkg.Message) {
	m := pkg.Deserialize(b)
	return m.Key, m
}

func main() {
	flag.Parse()
	beam.Init() // on Dataflow workers this runs the SDK harness and never returns

	if *benchProject == "" || *benchTopic == "" || *benchSub == "" {
		log.Fatal("need -bench_project, -bench_topic and -bench_subscription")
	}

	ctx := context.Background()

	// Build the pipeline first (cheap), so that submission starts right after t0.
	p := beam.NewPipeline()
	s := p.Root()
	pc := pubsubio.Read(s, *benchProject, pubsubio.ReadOptions{Subscription: *benchSub})
	keyed := beam.ParDo(s, keyFn, pc)
	beam.ParDo0(s, pkg.NewStatsFn(), keyed)

	client, err := pubsub.NewClient(ctx, *benchProject)
	if err != nil {
		log.Fatalf("pubsub client: %v", err)
	}
	defer client.Close()
	publisher := client.Publisher(*benchTopic)
	defer publisher.Stop()

	t0 := time.Now().UnixNano()

	// Publisher runs concurrently with job submission.
	var (
		sent, errs  atomic.Int64
		lastCreated int64
		pubDone     = make(chan struct{})
		wg          sync.WaitGroup
	)
	go func() {
		defer close(pubDone)
		rng := rand.New(rand.NewSource(*benchSeed))
		ticker := time.NewTicker(time.Second / time.Duration(*benchRate))
		defer ticker.Stop()
		end := time.NewTimer(time.Duration(*benchDur) * time.Second)
		defer end.Stop()
		var id int64
		for {
			select {
			case <-end.C:
				wg.Wait()
				return
			case <-ticker.C:
				created := time.Now().UnixNano()
				key := fmt.Sprintf("%d", rng.Intn(*benchKeys))
				val := float64(rng.Intn(100) + 1)
				m := pkg.NewMessage(key, id, t0, created, val)
				id++
				lastCreated = created
				res := publisher.Publish(ctx, &pubsub.Message{Data: m.Serialize()})
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, err := res.Get(ctx); err != nil {
						errs.Add(1)
						log.Printf("publish error: %v", err)
						return
					}
					sent.Add(1)
				}()
			}
		}
	}()

	// Submit the job (with --execute_async this returns once the job is created).
	log.Printf("submitting Dataflow job (t0=%d)", t0)
	if err := beamx.Run(ctx, p); err != nil {
		log.Fatalf("beamx.Run: %v", err)
	}
	log.Println("job submitted")

	<-pubDone
	m := meta{Mode: "beam", T0NS: t0, TEndNS: time.Now().UnixNano(), DurationSec: *benchDur, Rate: *benchRate,
		Keys: *benchKeys, Seed: *benchSeed, Sent: sent.Load(), Errors: errs.Load(), LastCreated: lastCreated}
	log.Printf("publisher done: sent=%d errors=%d", m.Sent, m.Errors)
	if *benchMeta != "" {
		b, _ := json.MarshalIndent(m, "", "  ")
		if err := os.WriteFile(*benchMeta, b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
