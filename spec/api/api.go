package api

import "context"

type Event struct {
	Headers map[string]string
	Body    []byte
}

type Function interface {
	Exec(ctx context.Context, event *Event) (*Event, error)
}
