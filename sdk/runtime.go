package sdk

import (
	"log"
)

type Runtime struct {
	Host     string
	Port     string
	function *function
}

type function struct {
	name          string
	path          string
	zip           string
	replication   int
	virtualShards int
}

func NewRuntime(host, port string) *Runtime {
	return &Runtime{
		Host: host,
		Port: port,
	}
}

func (rt *Runtime) Start() error {
	cnc, err := connect(rt.Host, rt.Port)
	if err != nil {
		return err
	}

	if err = cnc.upload(rt); err != nil {
		return err
	}

	return nil
}

func (rt *Runtime) Add(name string, path string, replication, vShards int) {
	if rt.function != nil {
		log.Fatalf("runtime has already a Function registered")
	}

	f := function{
		name:          name,
		path:          path,
		replication:   replication,
		virtualShards: vShards,
	}

	rt.function = &f
}
