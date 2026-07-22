package executor

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	executors map[string]RuntimeExecutor
}

func NewRegistry() *Registry {
	return &Registry{
		executors: make(map[string]RuntimeExecutor),
	}
}

func (r *Registry) Register(runtimeType string, executor RuntimeExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[runtimeType] = executor
}

func (r *Registry) Get(runtimeType string) (RuntimeExecutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.executors[runtimeType]
	if !ok {
		return nil, fmt.Errorf("executor not found for runtime type: %s", runtimeType)
	}
	return e, nil
}
