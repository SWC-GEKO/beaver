package sdk

import (
	"log"

	"github.com/SWC-GEKO/beaver/spec/api"
)

func RegisterFunction(name string, function api.Function) {
	if err := Default().Register(name, function); err != nil {
		log.Fatalf("failure to register function: %s", err)
	}
}
