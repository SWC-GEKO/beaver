package main

import (
	"context"
	"log"

	beaver "github.com/SWC-GEKO/beaver/sdk"
	"github.com/SWC-GEKO/beaver/spec/api"
)

func init() {
	beaver.Stateless("my-func", &Function{})
}

type Function struct{}

func (f *Function) Exec(ctx context.Context, event *api.Event) (*api.Event, error) {
	log.Printf("Received: %s", event.Body)
	return event, nil
}
