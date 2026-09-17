package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type Publisher struct {
	nc      *nats.Conn
	subject string
	t0      int64
	counter int64
}

func NewPublisher(natsUrl, subject string, t0 int64) (*Publisher, error) {
	c, err := nats.Connect(natsUrl, nats.Timeout(1*time.Second))
	if err != nil {
		return nil, err
	}

	return &Publisher{
		nc:      c,
		subject: subject,
		t0:      t0,
		counter: 0,
	}, nil
}

func (p *Publisher) Publish(key string, value float64) error {
	e, err := NewEvent(key, p.counter, p.t0, time.Now().UnixNano(), value)
	if err != nil {
		return err
	}
	p.counter++

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("not able to marshall event, err: %v", err)
	}

	return p.nc.Publish(p.subject, data)
}

func (p *Publisher) Close() error {
	return p.nc.Drain()
}
