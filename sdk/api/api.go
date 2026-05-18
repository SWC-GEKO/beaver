package api

import "context"

type Event struct {
	Headers map[string]any
	Body    []byte
}

type State[K comparable, S any] interface {
}

type StatelessFunction interface {
	Exec(ctx context.Context, event *Event) (*Event, error)
}

type StatefulFunction[K comparable, S any] interface {
	KeyBy(ctx context.Context, event Event) (K, error)
	Exec(ctx context.Context, state *State[K, S], event *Event) (*Event, error)
}

type Sink interface {
	Open(ctx context.Context) error
	Write(ctx context.Context, event *Event) error
	Close(ctx context.Context) error
}
