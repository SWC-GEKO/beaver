package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	Seed       int64
	KeySpace   int // Number of Keys possible!
	RatePerSec int // Events Per Second
	Duration   int // in seconds
	Url        string
}

type Payload struct {
	MessageID int64   `json:"messageId"`
	T0        int64   `json:"t0"`
	TCreated  int64   `json:"tCreated"`
	Value     float64 `json:"value"`
}

var cfg = &Config{}

func parseFlags() {
	duration := flag.Int("duration", 60, "duration of the benchmark in seconds")
	seed := flag.Int64("seed", 42, "PRNG seed -- reuse to reproduce the same key/value sequence")
	keySpace := flag.Int("keys", 4, "number of possible keys")
	rate := flag.Int("rate", 1, "events per second")
	url := flag.String("url", "http://localhost:8082/StatsFunction", "Server URL")
	flag.Parse()

	cfg.Seed = *seed
	cfg.KeySpace = *keySpace
	cfg.RatePerSec = *rate
	cfg.Duration = *duration
	cfg.Url = *url
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

	c := http.Client{}

	rng := rand.New(rand.NewSource(cfg.Seed))
	t0 := time.Now().UnixNano()
	msgId := int64(0)

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
			key := fmt.Sprintf("%d", rng.Intn(cfg.KeySpace))

			wg.Add(1)
			go func(key string, value float64) {
				defer wg.Done()

				r, err := NewRequest(msgId, t0, value)
				if err != nil {
					atomic.AddInt64(&errCount, 1)
					log.Printf("creating request error: %v", err)
				}
				msgId++

				r.Header.Set("Key", key)

				resp, err := c.Do(r)
				if err != nil {
					atomic.AddInt64(&errCount, 1)
					log.Printf("publish error: %v", err)
				}
				log.Println(resp.StatusCode)

			}(key, value)
		}
	}

	wg.Wait()
	log.Printf("finished executing benchmark (%d publish errors)", errCount)
}

func NewRequest(msgId, t0 int64, value float64) (*http.Request, error) {
	p := Payload{
		MessageID: msgId,
		T0:        t0,
		TCreated:  time.Now().UnixNano(),
		Value:     value,
	}

	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	body := bytes.NewReader(b)

	req, err := http.NewRequest("POST", cfg.Url, body)
	if err != nil {
		return nil, err
	}

	return req, nil
}
