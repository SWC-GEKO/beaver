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
	functions map[string]RegisteredFunction
}

var defaultRegistry = New()

func Default() *Registry {
	return defaultRegistry
}

func New() *Registry {
	return &Registry{
		functions: make(map[string]RegisteredFunction),
	}
}

func (r *Registry) Reset() {
	r.functions = map[string]RegisteredFunction{}
}

func (r *Registry) RegisterStateless(name string, s api.StatelessFunction) error {
	if name == "" {
		return errors.New("function must have a name")
	}

	if _, ok := r.functions[name]; ok {
		return errors.New("function already exists in registry")
	}

	f := RegisteredFunction{
		Name:      name,
		Type:      contracts.STATELESS,
		Stateless: s,
	}

	r.functions[f.Name] = f
	return nil
}

func (r *Registry) RegisterStateful(name string, s api.StatefulFunction) error {
	if name == "" {
		return errors.New("function must have a name")
	}

	if _, ok := r.functions[name]; ok {
		return errors.New("function already exists in registry")
	}

	f := RegisteredFunction{
		Name:     name,
		Type:     contracts.STATEFUL,
		Stateful: s,
	}

	r.functions[f.Name] = f
	return nil
}

func (r *Registry) Get(name string) (RegisteredFunction, bool) {
	f, ok := r.functions[name]
	return f, ok
}

func (r *Registry) GetAll() []RegisteredFunction {
	var fns []RegisteredFunction

	for _, f := range r.functions {
		fns = append(fns, f)
	}

	return fns
}
