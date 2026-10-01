package lg

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
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
	Events []Event `json:"events"`
}

type Event struct {
	Key       string  `json:"key"`
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

	rng := rand.New(rand.NewSource(cfg.Seed))
	t0 := time.Now().UnixNano()
	msgId := int64(0)

	var p Payload

loop:
	for {
		select {
		case <-timer.C:
			b, err := json.Marshal(p)
			if err != nil {
				log.Printf("marshalling payload failed with err: %v", err)
				break loop
			}

			body := bytes.NewReader(b)
			r, err := http.NewRequest("POST", cfg.Url, body)
			if err != nil {
				log.Printf("creating http request failed with err: %v", err)
				break loop
			}

			c := http.Client{}
			resp, err := c.Do(r)
			if err != nil {
				log.Printf("posting payload failed with err: %v", err)
				break loop
			}
			log.Println("StatusCode: ", resp.StatusCode)

		case <-ticker.C:
			value := float64(rng.Intn(100) + 1)
			key := fmt.Sprintf("%d", rng.Intn(cfg.KeySpace))

			e := Event{
				Key:       key,
				MessageID: msgId,
				T0:        t0,
				TCreated:  time.Now().UnixNano(),
				Value:     value,
			}
			msgId++

			p.Events = append(p.Events, e)
		}

	}
	log.Printf("finished executing benchmark")
}
