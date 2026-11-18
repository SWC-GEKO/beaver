package main

import (
	"fmt"
	"math/rand"
	"time"
)

// Payload is the event body shared with the Beaver and FaaS-Single functions
// (identical JSON fields as in evaluation/*).
type Payload struct {
	MessageID int64   `json:"messageId"`
	T0        int64   `json:"t0"`
	TCreated  int64   `json:"tCreated"`
	Value     float64 `json:"value"`
}

// BatchEvent is the per-event shape of the FaaS-Batched request body.
type BatchEvent struct {
	Key       string  `json:"key"`
	MessageID int64   `json:"messageId"`
	T0        int64   `json:"t0"`
	TCreated  int64   `json:"tCreated"`
	Value     float64 `json:"value"`
}

// Meta is written by every load-generating mode.
type Meta struct {
	Mode         string  `json:"mode"`
	T0NS         int64   `json:"t0_ns"`
	TEndNS       int64   `json:"t_end_ns"`
	DurationSec  int     `json:"duration_sec"`
	Rate         int     `json:"rate"`
	Keys         int     `json:"keys"`
	Seed         int64   `json:"seed"`
	Sent         int64   `json:"sent"`
	Errors       int64   `json:"errors"`
	OK2xx        int64   `json:"ok_2xx,omitempty"`
	NonOK        int64   `json:"non_2xx,omitempty"`
	LastCreated  int64   `json:"t_last_created_ns"`
	BatchPostNS  int64   `json:"batch_post_start_ns,omitempty"`
	BatchRespNS  int64   `json:"batch_resp_ns,omitempty"`
	ClientRTTAvg float64 `json:"client_rtt_avg_ms,omitempty"`
}

func newRNG(seed int64) *rand.Rand { return rand.New(rand.NewSource(seed)) }

func keyOf(rng *rand.Rand, n int) string { return fmt.Sprintf("%d", rng.Intn(n)) }

func valueOf(rng *rand.Rand) float64 { return float64(rng.Intn(100) + 1) }

func nowNS() int64 { return time.Now().UnixNano() }
