package sdk

import (
	"log"

	"github.com/SWC-GEKO/beaver/spec/api"
)

func Stateless(name string, function api.StatelessFunction) {
	if err := Default().RegisterStateless(name, function); err != nil {
		log.Fatalf("failure to register function: %s", err)
	}
}

// TODO: Stateful-Functions are not fully implemented -> maybe the API will change

func Stateful(name string, function api.StatefulFunction) {
	if err := Default().RegisterStateful(name, function); err != nil {
		log.Fatalf("failure to register function: %s", err)
	}
}
