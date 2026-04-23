package memory

import (
	"context"

	"github.com/azagatti/hydra-db/internal/body"
)

// Plane is the memory plane that exposes Store, Recall, Search, and Forget
// operations by delegating to a Provider backend.
type Plane struct {
	provider Provider
}

// NewPlane creates a memory Plane backed by the given provider.
func NewPlane(provider Provider) *Plane {
	return &Plane{provider: provider}
}

// Name implements body.Head, identifying this plane as "memory".
func (p *Plane) Name() string { return "memory" }

// Start is a no-op; the plane is ready immediately after construction.
func (p *Plane) Start(_ context.Context) error { return nil }

// Stop is a no-op; the plane holds no long-lived resources.
func (p *Plane) Stop(_ context.Context) error { return nil }

// Health reports the memory plane as healthy.
func (p *Plane) Health() body.HealthReport {
	return body.HealthReport{
		Head:   p.Name(),
		Status: body.HealthHealthy,
	}
}

// Store persists a Memory record through the provider.
func (p *Plane) Store(ctx context.Context, mem *Memory) error {
	return p.provider.Write(ctx, mem)
}

// Recall retrieves a single Memory record by ID.
func (p *Plane) Recall(ctx context.Context, id string) (*Memory, error) {
	return p.provider.Read(ctx, id)
}

// Search returns Memory records matching the given query.
func (p *Plane) Search(ctx context.Context, query SearchQuery) ([]*Memory, error) {
	return p.provider.Search(ctx, query)
}

// Forget deletes a Memory record by ID.
func (p *Plane) Forget(ctx context.Context, id string) error {
	return p.provider.Delete(ctx, id)
}

var _ body.Head = (*Plane)(nil)
