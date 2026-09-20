package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"

	beaver "github.com/SWC-GEKO/beaver/sdk"
	"github.com/SWC-GEKO/beaver/spec/api"
)

func init() {
	beaver.RegisterFunction("stats", &Function{KeyState: make(map[string]*State)})
}

// Payload is the unit of input for all variants
type Payload struct {
	MessageID int64   `json:"messageId"`
	T0        int64   `json:"t0"`
	TCreated  int64   `json:"tCreated"`
	Value     float64 `json:"value"`
}

// Result is the unit of output for all variants
type Result struct {
	Key           string  `json:"key"`
	MessageId     int64   `json:"messageId"`
	T0            int64   `json:"t0"`
	TCreated      int64   `json:"tCreated"`
	TReceived     int64   `json:"tReceived"`
	TProcessed    int64   `json:"tProcessed"`
	Value         float64 `json:"value"`
	Count         int64   `json:"count"`
	RollingAvg    float64 `json:"rollingAvg"`
	RollingStdDev float64 `json:"rollingStdDev"`
}

type State struct {
	Count int64
	Mean  float64
	Mean2 float64 // sum of squares of differences from the current mean
}

type Function struct {
	KeyState map[string]*State
}

func (f Function) Exec(ctx context.Context, event *api.Event) (*api.Event, error) {
	key, ok := event.Headers["Key"]
	if !ok {
		log.Println("key not given, skipping event!")
		return nil, nil
	}

	tReceived := time.Now().UnixNano()

	e, err := NewEventFromJsonBytes(event.Body)
	if err != nil {
		log.Println("Parsing new Event from JSON failed with err: ", err)
	}

	res := f.Process(key, e)

	res.TReceived = tReceived
	res.TProcessed = time.Now().UnixNano()

	// log structure:
	// @@@ messageId key t0 tCreated tReceived tProcessed count rollingAvg rollingStddev @@@
	//@@@ 0 0 0 0 1789716849652907802 1789716849653115885 13 0.00 0.00 @@@
	l := fmt.Sprintf("@@@ %d %s %d %d %d %d %d %.2f %.2f @@@",
		e.MessageID, key, e.T0, e.TCreated,
		res.TReceived, res.TProcessed, res.Count, res.RollingAvg, res.RollingStdDev)

	log.Println(l)

	return nil, nil
}

// Process folds one Event into given per-key State using
// Welford's online algorithm and returns the updated State.
func (f *Function) Process(k string, e *Payload) *Result {
	s, ok := f.KeyState[k]
	if !ok {
		s = &State{}
	}

	s.Count++
	delta := e.Value - s.Mean
	s.Mean += delta / float64(s.Count)
	delta2 := e.Value - s.Mean
	s.Mean2 += delta * delta2

	f.KeyState[k] = s

	var stddev float64
	if s.Count > 1 {
		if variance := s.Mean2 / float64(s.Count-1); variance > 0 {
			stddev = math.Sqrt(variance)
		}
	}

	return &Result{
		Key:           k,
		T0:            e.T0,
		TCreated:      e.TCreated,
		Value:         e.Value,
		Count:         s.Count,
		RollingAvg:    s.Mean,
		RollingStdDev: stddev,
	}
}

func NewEventFromJsonBytes(b []byte) (*Payload, error) {
	var e Payload
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return &e, nil
}
