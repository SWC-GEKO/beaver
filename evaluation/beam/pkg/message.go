package pkg

import "encoding/json"

type Message struct {
	Key       string  `json:"key"`
	MessageID int64   `json:"messageId"`
	T0        int64   `json:"t0"`
	TCreated  int64   `json:"tCreated"`
	Value     float64 `json:"value"`
}

func NewMessage(key string, id, t0, tCreated int64, value float64) *Message {
	return &Message{
		Key:       key,
		MessageID: id,
		T0:        t0,
		TCreated:  tCreated,
		Value:     value,
	}
}

func Deserialize(b []byte) Message {
	var m Message
	_ = json.Unmarshal(b, &m)
	return m
}

func (m *Message) Serialize() []byte {
	b, _ := json.Marshal(m)
	return b
}
