package sdk

import (
	"log"

	"github.com/SWC-GEKO/beaver/spec/contracts"
)

type Runtime struct {
	Host     string
	Port     string
	function *function
}

type function struct {
	name   string
	path   string
	fnType contracts.FunctionType
	zip    string
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

func (rt *Runtime) Upload(name string, path string, functionType contracts.FunctionType) {
	if rt.function != nil {
		log.Fatalf("runtime has already a Function registered")
	}

	f := function{
		name:   name,
		path:   path,
		fnType: functionType,
	}

	rt.function = &f
}
