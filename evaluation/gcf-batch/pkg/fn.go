package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

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

func init() {
	functions.HTTP("StatsFunctionBatched", stats)
}

func stats(rw http.ResponseWriter, r *http.Request) {
	tReceived := time.Now().UnixNano()

	var p Payload
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		log.Printf("reading from request body failed with: %v", err)
	}

	if err = json.Unmarshal(bytes, &p); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		log.Printf("unmarshalling request into payload failed with err: %v", err)
	}

	type State struct {
		Counter int64
		Mean    float64
		Mean2   float64
	}

	state := make(map[string]State)

	for _, e := range p.Events {
		s, ok := state[e.Key]
		if !ok {
			s = State{}
		}

		s.Counter++
		delta := e.Value - s.Mean
		s.Mean += delta / float64(s.Counter)
		delta2 := e.Value - s.Mean
		s.Mean2 += delta * delta2

		var stddev float64
		if s.Counter > 1 {
			if variance := s.Mean2 / float64(s.Counter-1); variance > 0 {
				stddev = math.Sqrt(variance)
			}
		}

		tProcessed := time.Now().UnixNano()

		l := fmt.Sprintf("@@@ %d %s %d %d %d %d %d %.2f %.2f @@@",
			e.MessageID, e.Key, e.T0, e.TCreated,
			tReceived, tProcessed, s.Counter, s.Mean, stddev)
		log.Println(l)
	}

	rw.WriteHeader(http.StatusOK)
}
