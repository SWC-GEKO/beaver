package pkg

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/apache/beam/sdks/v2/go/pkg/beam/core/state"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/log"
)

type Stats struct {
	Count int64
	Mean  float64
	M2    float64
}

type StatsFn struct {
	State state.Value[Stats]
}

func NewStatsFn() *StatsFn {
	return &StatsFn{State: state.MakeValueState[Stats]("stats")}
}

func (f *StatsFn) ProcessElement(ctx context.Context, sp state.Provider, key string, m Message) error {
	tReceived := time.Now().UnixNano()

	s, _, err := f.State.Read(sp)
	if err != nil {
		return err
	}

	s.Count++
	delta := m.Value - s.Mean
	s.Mean += delta / float64(s.Count)
	delta2 := m.Value - s.Mean
	s.M2 += delta * delta2

	var stddev float64
	if s.Count > 1 {
		if variance := s.M2 / float64(s.Count-1); variance > 0 {
			stddev = math.Sqrt(variance)
		}
	}

	if err := f.State.Write(sp, s); err != nil {
		return err
	}

	tp := time.Now().UnixNano()
	log.Info(ctx, fmt.Sprintf("@@@ %d %s %d %d %d %d %d %.2f %.2f @@@",
		m.MessageID, key, m.T0, m.TCreated, tReceived, tp, s.Count, s.Mean, stddev))
	return nil
}
