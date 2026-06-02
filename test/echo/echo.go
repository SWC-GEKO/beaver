package echo

import (
	"context"
	"encoding/json"
	"log"

	beaver "github.com/SWC-GEKO/beaver/sdk"
	"github.com/SWC-GEKO/beaver/spec/api"
)

func init() {
	beaver.Stateless("my-func", &Function{})
}

type Function struct{}

func (f *Function) Exec(ctx context.Context, event *api.Event) (*api.Event, error) {
	data := struct {
		Text string `json:"text"`
	}{}

	if err := json.Unmarshal(event.Body, &data); err != nil {
		return nil, err
	}

	log.Println("Received: ", data.Text)

	return event, nil
}
