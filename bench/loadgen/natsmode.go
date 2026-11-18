package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func newMsg(key, subject string, p Payload) (*nats.Msg, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	m := nats.NewMsg(subject)
	m.Data = b
	m.Header.Set("Key", key) // the router shards on this header
	return m, nil
}

type pub struct {
	nc      *nats.Conn
	js      jetstream.JetStream
	errs    atomic.Int64
	sent    atomic.Int64
	pending int
}

func newPub(url string, maxPending int) (*pub, error) {
	p := &pub{}
	nc, err := nats.Connect(url, nats.Timeout(5*time.Second), nats.MaxReconnects(-1))
	if err != nil {
		return nil, err
	}
	js, err := jetstream.New(nc,
		jetstream.WithPublishAsyncMaxPending(maxPending),
		jetstream.WithPublishAsyncErrHandler(func(_ jetstream.JetStream, m *nats.Msg, err error) {
			p.errs.Add(1)
			log.Printf("async publish error: %v", err)
		}),
	)
	if err != nil {
		nc.Close()
		return nil, err
	}
	p.nc, p.js = nc, js
	return p, nil
}

func (p *pub) publish(m *nats.Msg) {
	if _, err := p.js.PublishMsgAsync(m); err != nil {
		p.errs.Add(1)
		log.Printf("publish error: %v", err)
		return
	}
	p.sent.Add(1)
}

func (p *pub) close() {
	select {
	case <-p.js.PublishAsyncComplete():
	case <-time.After(30 * time.Second):
		log.Println("timeout waiting for outstanding publish acks")
	}
	p.nc.Close()
}

// ---------------------------------------------------------------------------
// Exp. 1/2: one stream, fixed rate, Beaver.
// ---------------------------------------------------------------------------

func runNatsStream(args []string) error {
	fs := flag.NewFlagSet("nats-stream", flag.ExitOnError)
	natsURL := fs.String("nats-url", "nats://localhost:4222", "global NATS URL")
	subject := fs.String("subject", "", "FUNCTIONS.<unique-name> (from beaver-upload)")
	dur := fs.Int("duration", 10, "stream duration [s]")
	rate := fs.Int("rate", 10, "events per second")
	keys := fs.Int("keys", 32, "number of distinct keys")
	seed := fs.Int64("seed", 42, "PRNG seed")
	meta := fs.String("meta", "", "write meta json here")
	_ = fs.Parse(args)
	if *subject == "" || *rate <= 0 || *keys <= 0 || *dur <= 0 {
		return errors.New("need -subject, positive -rate/-keys/-duration")
	}

	p, err := newPub(*natsURL, 4096)
	if err != nil {
		return err
	}
	rng := newRNG(*seed)
	interval := time.Second / time.Duration(*rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	end := time.NewTimer(time.Duration(*dur) * time.Second)
	defer end.Stop()

	t0 := nowNS() // stream start: cold-start penalty is measured from here
	var id, lastCreated int64
loop:
	for {
		select {
		case <-end.C:
			break loop
		case <-ticker.C:
			created := nowNS()
			m, err := newMsg(keyOf(rng, *keys), *subject, Payload{MessageID: id, T0: t0, TCreated: created, Value: valueOf(rng)})
			if err != nil {
				p.errs.Add(1)
				continue
			}
			id++
			lastCreated = created
			p.publish(m)
		}
	}
	p.close()
	m := Meta{Mode: "nats-stream", T0NS: t0, TEndNS: nowNS(), DurationSec: *dur, Rate: *rate, Keys: *keys,
		Seed: *seed, Sent: p.sent.Load(), Errors: p.errs.Load(), LastCreated: lastCreated}
	log.Printf("done: sent=%d errors=%d", m.Sent, m.Errors)
	return writeJSON(*meta, m)
}

// ---------------------------------------------------------------------------
// Exp. 3: N concurrent streams, each at a fixed rate; phases warmup/measure/cooldown.
// ---------------------------------------------------------------------------

type ScaleMeta struct {
	Meta
	Streams        int   `json:"streams"`
	StreamRate     int   `json:"stream_rate"`
	KeySpace       int   `json:"keyspace"`
	WarmupSec      int   `json:"warmup_sec"`
	MeasureSec     int   `json:"measure_sec"`
	CooldownSec    int   `json:"cooldown_sec"`
	MeasureStartNS int64 `json:"measure_start_ns"`
	MeasureEndNS   int64 `json:"measure_end_ns"`
	SentInMeasure  int64 `json:"sent_in_measure"`
}

func runNatsScale(args []string) error {
	fs := flag.NewFlagSet("nats-scale", flag.ExitOnError)
	natsURL := fs.String("nats-url", "nats://localhost:4222", "global NATS URL")
	subject := fs.String("subject", "", "FUNCTIONS.<unique-name>")
	streams := fs.Int("streams", 16, "concurrent streams (the x axis of the scalability plot)")
	streamRate := fs.Int("stream-rate", 10, "events/s per stream")
	keyspace := fs.Int("keyspace", 4096, "keys are drawn uniformly from this space")
	warm := fs.Int("warmup", 20, "warmup [s]")
	measure := fs.Int("measure", 60, "measurement window [s]")
	cool := fs.Int("cooldown", 10, "cooldown [s]")
	seed := fs.Int64("seed", 42, "PRNG seed")
	meta := fs.String("meta", "", "write meta json here")
	_ = fs.Parse(args)
	if *subject == "" || *streams <= 0 || *streamRate <= 0 || *keyspace <= 0 {
		return errors.New("need -subject and positive -streams/-stream-rate/-keyspace")
	}

	p, err := newPub(*natsURL, 50000)
	if err != nil {
		return err
	}
	t0 := nowNS()
	mStart := t0 + int64(*warm)*int64(time.Second)
	mEnd := mStart + int64(*measure)*int64(time.Second)
	tEnd := mEnd + int64(*cool)*int64(time.Second)

	var id, sentMeasure, lastCreated atomic.Int64
	done := make(chan struct{})
	interval := time.Second / time.Duration(*streamRate)

	for i := 0; i < *streams; i++ {
		go func(i int) {
			rng := newRNG(*seed + int64(i))
			// de-synchronise the streams
			time.Sleep(time.Duration(rng.Int63n(int64(interval))))
			tk := time.NewTicker(interval)
			defer tk.Stop()
			for {
				select {
				case <-done:
					return
				case <-tk.C:
					created := nowNS()
					if created >= tEnd {
						return
					}
					m, err := newMsg(keyOf(rng, *keyspace), *subject,
						Payload{MessageID: id.Add(1) - 1, T0: t0, TCreated: created, Value: valueOf(rng)})
					if err != nil {
						p.errs.Add(1)
						continue
					}
					if created >= mStart && created < mEnd {
						sentMeasure.Add(1)
					}
					lastCreated.Store(created)
					p.publish(m)
				}
			}
		}(i)
	}

	time.Sleep(time.Until(time.Unix(0, tEnd)))
	close(done)
	p.close()

	m := ScaleMeta{
		Meta: Meta{Mode: "nats-scale", T0NS: t0, TEndNS: nowNS(), DurationSec: *warm + *measure + *cool,
			Rate: *streams * *streamRate, Keys: *keyspace, Seed: *seed, Sent: p.sent.Load(), Errors: p.errs.Load(),
			LastCreated: lastCreated.Load()},
		Streams: *streams, StreamRate: *streamRate, KeySpace: *keyspace,
		WarmupSec: *warm, MeasureSec: *measure, CooldownSec: *cool,
		MeasureStartNS: mStart, MeasureEndNS: mEnd, SentInMeasure: sentMeasure.Load(),
	}
	log.Printf("done: streams=%d sent=%d (measure window: %d) errors=%d", *streams, m.Sent, m.SentInMeasure, m.Errors)
	return writeJSON(*meta, m)
}

var _ = fmt.Sprintf
