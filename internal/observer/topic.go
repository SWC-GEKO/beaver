package observer

import (
	"context"
	"sync"
)

type state int

const (
	Idle state = iota
	Active
)

type Topic struct {
	name  string
	state state
	mtx   sync.Mutex
	stop  context.CancelFunc
}

func NewTopic(name string) *Topic {
	return &Topic{
		name:  name,
		state: Idle,
		mtx:   sync.Mutex{},
		stop:  nil,
	}
}
