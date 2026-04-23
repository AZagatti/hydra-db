package inmemory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/azagatti/hydra-db/internal/memory"
)

// Provider is a thread-safe, map-backed memory.Provider for development
// and single-process deployments.
type Provider struct {
	mu       sync.RWMutex
	memories map[string]*memory.Memory
}

// NewProvider creates an empty in-memory provider.
func NewProvider() *Provider {
	return &Provider{
		memories: make(map[string]*memory.Memory),
	}
}

// Write stores a new Memory record, rejecting duplicates by ID.
func (p *Provider) Write(_ context.Context, mem *memory.Memory) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.memories[mem.ID]; exists {
		return fmt.Errorf("memory %q already exists", mem.ID)
	}
	p.memories[mem.ID] = mem
	return nil
}

// Read retrieves a Memory record by ID and updates its AccessedAt timestamp.
func (p *Provider) Read(_ context.Context, id string) (*memory.Memory, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	mem, ok := p.memories[id]
	if !ok {
		return nil, fmt.Errorf("memory %q not found", id)
	}
	mem.AccessedAt = time.Now()
	return mem, nil
}

// Delete removes a Memory record by ID.
func (p *Provider) Delete(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.memories[id]; !ok {
		return fmt.Errorf("memory %q not found", id)
	}
	delete(p.memories, id)
	return nil
}

// Search returns Memory records matching the query, sorted newest-first and
// capped by the query's Limit (defaulting to 50).
func (p *Provider) Search(_ context.Context, query memory.SearchQuery) ([]*memory.Memory, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var results []*memory.Memory
	for _, mem := range p.memories {
		if !matchesQuery(mem, query) {
			continue
		}
		results = append(results, mem)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func matchesQuery(mem *memory.Memory, q memory.SearchQuery) bool {
	if q.Type != "" && mem.Type != q.Type {
		return false
	}
	if len(q.Tags) > 0 && !containsAllTags(mem.Tags, q.Tags) {
		return false
	}
	if q.ActorID != "" && mem.ActorID != q.ActorID {
		return false
	}
	if q.TenantID != "" && mem.TenantID != q.TenantID {
		return false
	}
	if !q.Since.IsZero() && mem.CreatedAt.Before(q.Since) {
		return false
	}
	if q.MinConfidence > 0 && mem.Confidence < q.MinConfidence {
		return false
	}
	return true
}

func containsAllTags(memoryTags, queryTags []string) bool {
	tagSet := make(map[string]struct{}, len(memoryTags))
	for _, t := range memoryTags {
		tagSet[t] = struct{}{}
	}
	for _, t := range queryTags {
		if _, ok := tagSet[t]; !ok {
			return false
		}
	}
	return true
}

var _ memory.Provider = (*Provider)(nil)
