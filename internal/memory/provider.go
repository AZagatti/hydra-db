package memory

import (
	"context"
	"time"
)

// SearchQuery constrains a memory search by type, tags, ownership, time range,
// and confidence threshold.
type SearchQuery struct {
	Type          Type
	Tags          []string
	ActorID       string
	TenantID      string
	Limit         int
	Since         time.Time
	MinConfidence float64
}

// Provider is the storage backend contract for persisting and retrieving
// Memory records.
type Provider interface {
	Write(ctx context.Context, mem *Memory) error
	Read(ctx context.Context, id string) (*Memory, error)
	Search(ctx context.Context, query SearchQuery) ([]*Memory, error)
	Delete(ctx context.Context, id string) error
}
