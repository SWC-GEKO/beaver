package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	Seed       int64
	KeySpace   int // Number of Keys possible!
	RatePerSec int // Events Per Second
	Duration   int // in seconds
	NatsUrl    string
	Subject    string
}

var cfg = &Config{}

func parseFlags() {
	duration := flag.Int("duration", 60, "duration of the benchmark in seconds")
	seed := flag.Int64("seed", 42, "PRNG seed -- reuse to reproduce the same key/value sequence")
	keySpace := flag.Int("keys", 10, "number of possible keys")
	rate := flag.Int("rate", 10, "events per second")
	natsURL := flag.String("nats-url", "nats://localhost:4222", "NATS server URL")
	subject := flag.String("subject", "events", "NATS subject to publish to")
	flag.Parse()

	cfg.Seed = *seed
	cfg.KeySpace = *keySpace
	cfg.RatePerSec = *rate
	cfg.Duration = *duration
	cfg.NatsUrl = *natsURL
	cfg.Subject = *subject
}

func main() {
	parseFlags()

	if cfg.RatePerSec <= 0 {
		log.Fatalf("rate must be positive, got %d", cfg.RatePerSec)
	}
	if cfg.KeySpace <= 0 {
		log.Fatalf("keys must be positive, got %d", cfg.KeySpace)
	}

	interval := time.Second / time.Duration(cfg.RatePerSec)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	timer := time.NewTimer(time.Duration(cfg.Duration) * time.Second)
	defer timer.Stop()

	rng := rand.New(rand.NewSource(cfg.Seed))
	t0 := time.Now().UnixNano()

	pub, err := NewPublisher(cfg.NatsUrl, cfg.Subject, t0)
	if err != nil {
		panic(err)
	}
	defer pub.Close()

	var wg sync.WaitGroup
	var errCount int64

loop:
	for {
		select {
		case <-timer.C:
			log.Println("benchmark duration elapsed, waiting for in-flight publishes...")
			break loop
		case <-ticker.C:
			value := float64(rng.Intn(100) + 1) // 1..100
			key := fmt.Sprintf("key-%d", rng.Intn(cfg.KeySpace))

			wg.Add(1)
			go func(key string, value float64) {
				defer wg.Done()
				if err := pub.Publish(key, value); err != nil {
					atomic.AddInt64(&errCount, 1)
					log.Printf("publish error: %v", err)
				}
			}(key, value)
		}
	}

	wg.Wait()
	log.Printf("finished executing benchmark (%d publish errors)", errCount)
}
