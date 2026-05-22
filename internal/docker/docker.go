package docker

import (
	"log"

	"github.com/SWC-GEKO/beaver/internal/fn"
)

// TODO: implement docker functionality

type Docker struct {
}

func NewDocker() *Docker {
	return nil
}

func (d *Docker) Create() (fn.Function, error) {
	// TODO: implement me...
	log.Println("creating a function is not implemented")
	return nil, nil
}
