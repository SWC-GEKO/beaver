package main

import (
	"encoding/json"

	"github.com/nats-io/nats.go"
)

type Payload struct {
	MessageID int64   `json:"messageId"`
	T0        int64   `json:"t0"`
	TCreated  int64   `json:"tCreated"`
	Value     float64 `json:"value"`
}

func NewMsg(key, subject string, id, t0, tCreated int64, value float64) (*nats.Msg, error) {
	p := Payload{
		MessageID: id,
		T0:        t0,
		TCreated:  tCreated,
		Value:     value,
	}

	bytes, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}

	msg := nats.NewMsg(subject)
	msg.Data = bytes
	msg.Header.Set("Key", key)

	return msg, nil
}
