package sdk

import (
	"errors"

	"github.com/SWC-GEKO/beaver/spec/api"
	"github.com/SWC-GEKO/beaver/spec/contracts"
)

type RegisteredFunction struct {
	Name string
	Type contracts.FunctionType

	Stateless api.StatelessFunction
	Stateful  api.StatefulFunction
}

type Registry struct {
	function *RegisteredFunction
}

var defaultRegistry = New()

func Default() *Registry {
	return defaultRegistry
}

func New() *Registry {
	return &Registry{}
}

func (r *Registry) Reset() {
	r.function = nil
}

func (r *Registry) RegisterStateless(name string, s api.StatelessFunction) error {
	if name == "" {
		return errors.New("function must have a name")
	}

	if r.function != nil {
		return errors.New("function already exists in registry")
	}

	f := RegisteredFunction{
		Name:      name,
		Type:      contracts.STATELESS,
		Stateless: s,
	}

	r.function = &f
	return nil
}

func (r *Registry) RegisterStateful(name string, s api.StatefulFunction) error {
	if name == "" {
		return errors.New("function must have a name")
	}

	if r.function != nil {
		return errors.New("function already exists in registry")
	}

	f := RegisteredFunction{
		Name:     name,
		Type:     contracts.STATEFUL,
		Stateful: s,
	}

	r.function = &f
	return nil
}

// Get returns a copy of the underlying function. This is required as each nats-topic gets consumed by a single thread!

func (r *Registry) Get() RegisteredFunction {
	return *r.function
}
