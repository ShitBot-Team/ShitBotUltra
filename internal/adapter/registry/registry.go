package registry

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
)

type Registry struct {
	mu        sync.RWMutex
	factories map[string]adapter.Factory
}

func New() *Registry { return &Registry{factories: map[string]adapter.Factory{}} }
func (r *Registry) Register(kind string, factory adapter.Factory) error {
	if !adapter.ValidID(kind) || factory == nil {
		return errors.New("invalid adapter registration")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.factories[kind]; ok {
		return fmt.Errorf("adapter %s is already registered", kind)
	}
	r.factories[kind] = factory
	return nil
}
func (r *Registry) Create(kind string, cfg adapter.Config) (adapter.Adapter, error) {
	r.mu.RLock()
	factory := r.factories[kind]
	r.mu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("adapter %s is not registered", kind)
	}
	return factory(cfg)
}
