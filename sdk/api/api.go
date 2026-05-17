package api

import "context"

type Event struct {
	Headers map[string]any
	Body    []byte
}

type FunctionContext[K comparable, S any] interface {
	Set(key K, state S) error
	Get(key K) (S, bool)
	// Configuration (Connection-Function) is missing
}

type StatelessFunction interface {
	Exec(event *Event) (*Event, error)
}

type StatefulFunction[K comparable, S any] interface {
	Exec(ctx *FunctionContext[K, S], event *Event) (*Event, error)
	KeyBy(event Event) (K, error)
}

type Sink interface {
	Open(ctx context.Context) error
	Write(ctx context.Context, event *Event) error
	Close(ctx context.Context) error
}
