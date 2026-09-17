package sdk

import (
	"errors"

	"github.com/SWC-GEKO/beaver/spec/api"
)

type Registry struct {
	function *api.Function
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

func (r *Registry) Register(name string, fn api.Function) error {
	if name == "" {
		return errors.New("function must have a name")
	}

	if r.function != nil {
		return errors.New("function already exists in registry")
	}

	r.function = &fn
	return nil
}

func (r *Registry) Get() api.Function {
	return *r.function
}
