package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/azagatti/hydra-db/internal/memory"
)

// MemoryWriteInput describes a memory write operation.
type MemoryWriteInput struct {
	MemoryType string         `json:"type"`
	Content    map[string]any `json:"content"`
	ActorID    string         `json:"actor_id"`
	Confidence float64        `json:"confidence,omitempty"`
	Tags       []string       `json:"tags,omitempty"`
}

// MemoryWrite is a tool that persists a Memory record to the Memory Plane.
type MemoryWrite struct {
	plane *memory.Plane
}

// NewMemoryWrite creates a MemoryWrite tool backed by the given *memory.Plane.
func NewMemoryWrite(plane *memory.Plane) *MemoryWrite {
	return &MemoryWrite{plane: plane}
}

// Name returns "memory_write".
func (t *MemoryWrite) Name() string { return "memory_write" }

// Execute stores a memory record via the Memory Plane.
func (t *MemoryWrite) Execute(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var input MemoryWriteInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("memory_write: unmarshal input: %w", err)
	}

	memType, err := parseMemoryType(input.MemoryType)
	if err != nil {
		return nil, fmt.Errorf("memory_write: %w", err)
	}

	content, err := json.Marshal(input.Content)
	if err != nil {
		return nil, fmt.Errorf("memory_write: marshal content: %w", err)
	}

	actorID := input.ActorID
	if actorID == "" {
		actorID = "agent"
	}

	confidence := input.Confidence
	if confidence == 0 {
		confidence = 0.8
	}

	mem := memory.NewMemory(memType, content, actorID, "default")
	mem.Confidence = confidence
	if err := t.plane.Store(ctx, mem); err != nil {
		return nil, fmt.Errorf("memory_write: store: %w", err)
	}

	return json.Marshal(map[string]any{
		"ok":   true,
		"id":   mem.ID,
		"type": input.MemoryType,
		"actor": actorID,
	})
}

// MemorySearchInput describes a memory search operation.
type MemorySearchInput struct {
	Type    string   `json:"type,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Limit   int      `json:"limit,omitempty"`
	ActorID string   `json:"actor_id,omitempty"`
}

// MemorySearch is a tool that queries the Memory Plane.
type MemorySearch struct {
	plane *memory.Plane
}

// NewMemorySearch creates a MemorySearch tool backed by the given *memory.Plane.
func NewMemorySearch(plane *memory.Plane) *MemorySearch {
	return &MemorySearch{plane: plane}
}

// Name returns "memory_search".
func (t *MemorySearch) Name() string { return "memory_search" }

// Execute queries memory records via the Memory Plane.
func (t *MemorySearch) Execute(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var input MemorySearchInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("memory_search: unmarshal input: %w", err)
	}

	memType, _ := parseMemoryType(input.Type)

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	results, err := t.plane.Search(ctx, memory.SearchQuery{
		Type:    memType,
		Limit:   limit,
		ActorID: input.ActorID,
	})
	if err != nil {
		return nil, fmt.Errorf("memory_search: search: %w", err)
	}

	out := make([]map[string]any, 0, len(results))
	for _, m := range results {
		out = append(out, map[string]any{
			"id":         m.ID,
			"type":       m.Type,
			"actor_id":   m.ActorID,
			"confidence": m.Confidence,
			"tags":       m.Tags,
			"content":    m.Content,
		})
	}

	return json.Marshal(map[string]any{
		"count":   len(out),
		"results": out,
	})
}

// parseMemoryType converts a string to memory.Type.
func parseMemoryType(s string) (memory.Type, error) {
	switch s {
	case "", "episodic":
		return memory.Episodic, nil
	case "semantic":
		return memory.Semantic, nil
	case "operational":
		return memory.Operational, nil
	case "working":
		return memory.Working, nil
	default:
		return memory.Episodic, fmt.Errorf("unknown memory type %q", s)
	}
}
