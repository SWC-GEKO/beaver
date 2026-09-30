package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"github.com/redis/go-redis/v9"
)

type Payload struct {
	MessageID int64   `json:"messageId"`
	T0        int64   `json:"t0"`
	TCreated  int64   `json:"tCreated"`
	Value     float64 `json:"value"`
}

func init() {
	functions.HTTP("StatsFunction", stats)
}

var client = redis.NewClient(&redis.Options{
	Addr: "localhost:6379",
})

func stats(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := r.Header.Get("Key")

	tReceived := time.Now().UnixNano()

	var p Payload
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		log.Fatalf("reading from request body failed with: %v", err)
	}

	if err := json.Unmarshal(bytes, &p); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		log.Fatalf("unmarshalling request into payload failed with err: %v", err)
	}

	vals, err := client.HGetAll(ctx, key).Result()
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		log.Fatalf("fetching values for key: %s, failed with err: %v", key, err)
	}

	counter := int64(0)
	mean := float64(0)
	mean2 := float64(0)

	if c, ok := vals["count"]; ok {
		counter, err = strconv.ParseInt(c, 10, 64)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			log.Fatalf("failed to parse count %q: %v", c, err)
		}
	}

	if m, ok := vals["mean"]; ok {
		mean, err = strconv.ParseFloat(m, 64)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			log.Fatalf("failed to parse mean %q: %v", m, err)
		}
	}

	if m2, ok := vals["mean2"]; ok {
		mean2, err = strconv.ParseFloat(m2, 64)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			log.Fatalf("failed to parse mean2 %q: %v", m2, err)
		}
	}

	counter++
	delta := p.Value - mean
	mean += delta / float64(counter)
	delta2 := p.Value - mean
	mean2 += delta * delta2

	var stddev float64
	if counter > 1 {
		if variance := mean2 / float64(counter-1); variance > 0 {
			stddev = math.Sqrt(variance)
		}
	}

	_, err = client.HSet(ctx, key,
		"count", counter,
		"mean", mean,
		"mean2", mean2,
	).Result()

	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		log.Fatalf("failed to update state for key %s: %v", key, err)
	}

	tProcessed := time.Now().UnixNano()

	// Log Scheme:
	// @@@ messageId key t0 tCreated tReceived tProcessed count rollingAvg rollingStddev @@@
	l := fmt.Sprintf("@@@ %d %s %d %d %d %d %d %.2f %.2f @@@",
		p.MessageID, key, p.T0, p.TCreated,
		tReceived, tProcessed, counter, mean, stddev)

	log.Println(l)

	rw.WriteHeader(http.StatusOK)
}
