package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func newClient() *http.Client {
	tr := &http.Transport{
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 256,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	return &http.Client{Transport: tr, Timeout: 10 * time.Minute}
}

// authHeader returns "Bearer <token>" if LG_AUTH_TOKEN is set (private Cloud Run services).
func authHeader() string {
	if t := os.Getenv("LG_AUTH_TOKEN"); t != "" {
		return "Bearer " + t
	}
	return ""
}

func post(c *http.Client, url string, body []byte, key string) (int, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Key", key)
	}
	if a := authHeader(); a != "" {
		req.Header.Set("Authorization", a)
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// ---------------------------------------------------------------------------
// FaaS-Single: one HTTP request per event, fired concurrently at a fixed rate.
// ---------------------------------------------------------------------------

func runHTTPSingle(args []string) error {
	fs := flag.NewFlagSet("http-single", flag.ExitOnError)
	url := fs.String("url", "", "function URL, e.g. https://<svc>.run.app/StatsFunction")
	dur := fs.Int("duration", 10, "stream duration [s]")
	rate := fs.Int("rate", 10, "events per second")
	keys := fs.Int("keys", 32, "number of distinct keys")
	seed := fs.Int64("seed", 42, "PRNG seed")
	meta := fs.String("meta", "", "write meta json here")
	_ = fs.Parse(args)
	if *url == "" || *rate <= 0 || *keys <= 0 || *dur <= 0 {
		return errors.New("need -url, positive -rate/-keys/-duration")
	}

	c := newClient()
	rng := newRNG(*seed)
	ticker := time.NewTicker(time.Second / time.Duration(*rate))
	defer ticker.Stop()
	end := time.NewTimer(time.Duration(*dur) * time.Second)
	defer end.Stop()

	var wg sync.WaitGroup
	var sent, ok, nonOK, errs, rttSumUS int64
	t0 := nowNS()
	var id, lastCreated int64
loop:
	for {
		select {
		case <-end.C:
			break loop
		case <-ticker.C:
			created := nowNS()
			key := keyOf(rng, *keys)
			body, _ := json.Marshal(Payload{MessageID: id, T0: t0, TCreated: created, Value: valueOf(rng)})
			id++
			lastCreated = created
			atomic.AddInt64(&sent, 1)
			wg.Add(1)
			go func(key string, body []byte, created int64) {
				defer wg.Done()
				code, err := post(c, *url, body, key)
				atomic.AddInt64(&rttSumUS, (nowNS()-created)/1000)
				switch {
				case err != nil:
					atomic.AddInt64(&errs, 1)
					log.Printf("request error: %v", err)
				case code >= 200 && code < 300:
					atomic.AddInt64(&ok, 1)
				default:
					atomic.AddInt64(&nonOK, 1)
					log.Printf("non-2xx status: %d", code)
				}
			}(key, body, created)
		}
	}
	wg.Wait()
	m := Meta{Mode: "http-single", T0NS: t0, TEndNS: nowNS(), DurationSec: *dur, Rate: *rate, Keys: *keys, Seed: *seed,
		Sent: sent, Errors: errs, OK2xx: ok, NonOK: nonOK, LastCreated: lastCreated}
	if sent > 0 {
		m.ClientRTTAvg = float64(rttSumUS) / float64(sent) / 1000.0
	}
	log.Printf("done: sent=%d ok=%d non2xx=%d errors=%d avgRTT=%.1fms", sent, ok, nonOK, errs, m.ClientRTTAvg)
	return writeJSON(*meta, m)
}

// ---------------------------------------------------------------------------
// FaaS-Batched: events are buffered for the whole stream, then sent in ONE request.
// ---------------------------------------------------------------------------

func runHTTPBatch(args []string) error {
	fs := flag.NewFlagSet("http-batch", flag.ExitOnError)
	url := fs.String("url", "", "function URL, e.g. https://<svc>.run.app/StatsFunctionBatched")
	dur := fs.Int("duration", 10, "stream duration [s]")
	rate := fs.Int("rate", 10, "events per second")
	keys := fs.Int("keys", 32, "number of distinct keys")
	seed := fs.Int64("seed", 42, "PRNG seed")
	meta := fs.String("meta", "", "write meta json here")
	_ = fs.Parse(args)
	if *url == "" || *rate <= 0 || *keys <= 0 || *dur <= 0 {
		return errors.New("need -url, positive -rate/-keys/-duration")
	}

	rng := newRNG(*seed)
	ticker := time.NewTicker(time.Second / time.Duration(*rate))
	defer ticker.Stop()
	end := time.NewTimer(time.Duration(*dur) * time.Second)
	defer end.Stop()

	t0 := nowNS()
	var events []BatchEvent
	var id, lastCreated int64
loop:
	for {
		select {
		case <-end.C:
			break loop
		case <-ticker.C:
			created := nowNS()
			events = append(events, BatchEvent{Key: keyOf(rng, *keys), MessageID: id, T0: t0, TCreated: created, Value: valueOf(rng)})
			id++
			lastCreated = created
		}
	}

	body, err := json.Marshal(struct {
		Events []BatchEvent `json:"events"`
	}{events})
	if err != nil {
		return err
	}
	c := newClient()
	postStart := nowNS()
	code, perr := post(c, *url, body, "")
	postEnd := nowNS()

	m := Meta{Mode: "http-batch", T0NS: t0, TEndNS: postEnd, DurationSec: *dur, Rate: *rate, Keys: *keys, Seed: *seed,
		Sent: int64(len(events)), LastCreated: lastCreated, BatchPostNS: postStart, BatchRespNS: postEnd}
	switch {
	case perr != nil:
		m.Errors = 1
		log.Printf("batch request error: %v", perr)
	case code >= 200 && code < 300:
		m.OK2xx = 1
	default:
		m.NonOK = 1
		log.Printf("batch non-2xx status: %d", code)
	}
	log.Printf("done: events=%d status=%d post=%.1fms", len(events), code, float64(postEnd-postStart)/1e6)
	return writeJSON(*meta, m)
}
