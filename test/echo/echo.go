package echo

import (
	"encoding/json"
	"log"

	"github.com/SWC-GEKO/beaver/sdk/api"
)

type Function struct{}

func (f Function) Exec(event *api.Event) (*api.Event, error) {
	data := struct {
		Text string `json:"text"`
	}{}

	if err := json.Unmarshal(event.Body, &data); err != nil {
		return nil, err
	}

	log.Println("Received: ", data.Text)

	return event, nil
}
