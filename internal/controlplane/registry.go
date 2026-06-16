package controlplane

import (
	"errors"
	"sync"

	"github.com/SWC-GEKO/beaver/internal/composer"
	"github.com/SWC-GEKO/beaver/internal/docker"
)

type FunctionState int

const (
	// stateRegistered is the state between the deployment and the first run?
	idle FunctionState = iota
	// stateInvoked is the state between the observation of an incoming request and the function start-up
	starting
	// stateRunning is the state in which a function consumes events
	running
	// stateStopped is the state in which a function already consumed
	stopping
	// failed is when a function crashes or is not able to deploy!
	failed
)

type Registry struct {
	// Should I do an invoked/active vs. passive functions?
	functions map[string]*FunctionEntry
	mu        sync.RWMutex
	composer  *composer.Composer
}

type FunctionEntry struct {
	function *docker.Function
	state    FunctionState
}

func NewRegistry(c *composer.Composer) *Registry {
	return &Registry{
		functions: make(map[string]*FunctionEntry),
		mu:        sync.RWMutex{},
		composer:  c,
	}
}

func (r *Registry) Add(uniqueName string, f *docker.Function) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.composer.Add(f); err != nil {
		return err
	}

	if _, ok := r.functions[uniqueName]; ok {
		return ErrAlreadyExists
	}

	r.functions[uniqueName] = newFunctionEntry(f)
	return nil
}

func (r *Registry) Remove(uniqueName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.functions[uniqueName]; !ok {
		return ErrNotFound
	}

	delete(r.functions, uniqueName)
	return nil
}

func (r *Registry) Invoke(uniqueName string) error {
	r.mu.Lock()

	e, ok := r.functions[uniqueName]
	if !ok {
		r.mu.Unlock()
		return ErrNotFound
	}

	switch e.state {
	case running:
		r.mu.Unlock()
		return ErrAlreadyRunning
	case starting:
		r.mu.Unlock()
		return ErrAlreadyRunning
	case idle:
		e.state = starting
	default:
		// TODO: do better error handling!
		return errors.New("function is neither running/starting/idle")
	}

	r.mu.Unlock()

	if err := r.composer.Up(e.function.UniqueName); err != nil {
		r.mu.Lock()
		defer r.mu.Unlock()

		e.state = failed
		return errors.Join(err, ErrFunctionDeploymentFailed)
	}

	r.mu.Lock()
	e.state = running
	r.mu.Unlock()

	return nil
}

func (r *Registry) Stop(uniqueName string) error {
	// TODO: implement this which should stop a Function that is running
	// stateRunning -> stateStopped
	panic("implement me...")
}

func newFunctionEntry(f *docker.Function) *FunctionEntry {
	return &FunctionEntry{
		function: f,
		state:    idle,
	}
}
