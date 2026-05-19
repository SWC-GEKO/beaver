package echo2

import (
	"context"

	"github.com/SWC-GEKO/beaver/sdk/api"
)

type Fn struct {
}

func (f Fn) KeyBy(ctx context.Context, event api.Event) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (f Fn) Exec(ctx context.Context, state *api.State[string, string], event *api.Event) (*api.Event, error) {
	//TODO implement me
	panic("implement me")
}
