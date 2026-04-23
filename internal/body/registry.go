package body

import (
	"context"
	"fmt"
	"sync"
)

// Head is the contract every Hydra head must satisfy. Heads are independently
// startable and stoppable units of functionality that report their own health.
type Head interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health() HealthReport
}

// Registry tracks all active heads and provides lifecycle management.
type Registry struct {
	mu    sync.RWMutex
	heads map[string]Head
}

// NewRegistry creates an empty head registry.
func NewRegistry() *Registry {
	return &Registry{
		heads: make(map[string]Head),
	}
}

// Register adds a head to the registry. Returns an error if a head with the
// same name is already registered.
func (r *Registry) Register(head Head) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := head.Name()
	if _, exists := r.heads[name]; exists {
		return fmt.Errorf("head %q already registered", name)
	}
	r.heads[name] = head
	return nil
}

// Get retrieves a head by name.
func (r *Registry) Get(name string) (Head, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	head, ok := r.heads[name]
	if !ok {
		return nil, fmt.Errorf("head %q not found", name)
	}
	return head, nil
}

// List returns all registered heads.
func (r *Registry) List() []Head {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Head, 0, len(r.heads))
	for _, h := range r.heads {
		out = append(out, h)
	}
	return out
}

// Names returns the names of all registered heads.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, 0, len(r.heads))
	for name := range r.heads {
		out = append(out, name)
	}
	return out
}

// StartAll starts every registered head in order, aborting on the first error.
func (r *Registry) StartAll(ctx context.Context) error {
	r.mu.RLock()
	heads := make([]Head, 0, len(r.heads))
	for _, h := range r.heads {
		heads = append(heads, h)
	}
	r.mu.RUnlock()

	for _, h := range heads {
		if err := h.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

// StopAll stops every registered head, collecting all errors rather than
// aborting on the first failure.
func (r *Registry) StopAll(ctx context.Context) error {
	r.mu.RLock()
	heads := make([]Head, 0, len(r.heads))
	for _, h := range r.heads {
		heads = append(heads, h)
	}
	r.mu.RUnlock()

	var errs []error
	for _, h := range heads {
		if err := h.Stop(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %v", errs)
	}
	return nil
}

// HealthAll collects health reports from every registered head.
func (r *Registry) HealthAll() []HealthReport {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]HealthReport, 0, len(r.heads))
	for _, h := range r.heads {
		out = append(out, h.Health())
	}
	return out
}

// IsHealthy returns true only when every registered head reports healthy.
func (r *Registry) IsHealthy() bool {
	for _, report := range r.HealthAll() {
		if report.Status != HealthHealthy {
			return false
		}
	}
	return true
}
