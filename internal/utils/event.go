package utils

import (
	"errors"

	"github.com/SWC-GEKO/beaver/spec/api"
	"github.com/cespare/xxhash/v2"
	"github.com/nats-io/nats.go"
)

func GetKeyFromMsg(msg *nats.Msg) (string, error) {
	k := msg.Header.Get("Key")
	if k == "" {
		return "", errors.New("message does not have a dedicated key")
	}

	return k, nil
}

func ParseEventFromMsg(msg *nats.Msg) *api.Event {
	var e api.Event
	e.Body = msg.Data
	for k, v := range msg.Header {
		e.Headers[k] = v
	}

	return &e
}

func GetShard(key string, vshards int) int {
	return int(xxhash.Sum64String(key)) % vshards
}
