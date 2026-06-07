package api

import "context"

type Event struct {
	Headers map[string]string
	Body    []byte
}

type State interface {
}

type StatelessFunction interface {
	Exec(ctx context.Context, event *Event) (*Event, error)
}

type StatefulFunction interface {
	Exec(ctx context.Context, state *State, event *Event) (*Event, error)
}

type Sink interface {
	Open(ctx context.Context) error
	Write(ctx context.Context, event *Event) error
	Close(ctx context.Context) error
}
