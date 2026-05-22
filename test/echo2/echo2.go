package echo2

import (
	"context"

	"github.com/SWC-GEKO/beaver/spec/api"
)

type Fn struct {
}

func (f Fn) KeyBy(ctx context.Context, event api.Event) (any, error) {
	//TODO implement me
	panic("implement me")
}

func (f Fn) Exec(ctx context.Context, state *api.State, event *api.Event) (*api.Event, error) {
	//TODO implement me
	panic("implement me")
}
