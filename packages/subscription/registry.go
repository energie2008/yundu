package subscription

import (
	"sync"

	"github.com/airport-panel/subscription/renderer"
)

type Renderer = renderer.Renderer

type Registry struct {
	mu        sync.RWMutex
	renderers map[string]Renderer
}

func NewRegistry() *Registry {
	return &Registry{
		renderers: make(map[string]Renderer),
	}
}

func (r *Registry) Register(rend Renderer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.renderers == nil {
		r.renderers = make(map[string]Renderer)
	}
	r.renderers[rend.Name()] = rend
}

func (r *Registry) Get(name string) Renderer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.renderers[name]
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.renderers))
	for n := range r.renderers {
		names = append(names, n)
	}
	return names
}
