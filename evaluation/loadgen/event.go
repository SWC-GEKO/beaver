package main

import (
	"encoding/json"

	"github.com/SWC-GEKO/beaver/spec/api"
)

type Payload struct {
	MessageID int64   `json:"messageId"`
	T0        int64   `json:"t0"`
	TCreated  int64   `json:"tCreated"`
	Value     float64 `json:"value"`
}

func NewEvent(key string, id, t0, tCreated int64, value float64) (*api.Event, error) {
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

	e := api.Event{
		Headers: make(map[string]string),
		Body:    bytes,
	}
	e.Headers["Key"] = key

	return &e, nil
}
