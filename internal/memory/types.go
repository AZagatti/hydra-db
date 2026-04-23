package memory

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Type classifies the kind of memory record, mirroring human memory
// categories for agent cognition.
type Type string

const (
	// Episodic stores time-bounded events and interactions.
	Episodic Type = "episodic"
	// Semantic stores general knowledge and facts.
	Semantic Type = "semantic"
	// Operational stores procedural and configuration knowledge.
	Operational Type = "operational"
	// Working stores transient, short-lived scratch-pad data.
	Working Type = "working"
)

// Memory is the fundamental unit stored by the memory plane, carrying typed
// content, ownership metadata, and confidence scoring.
type Memory struct {
	ID         string            `json:"id"`
	Type       Type              `json:"type"`
	Content    json.RawMessage   `json:"content"`
	Tags       []string          `json:"tags,omitempty"`
	Confidence float64           `json:"confidence"`
	ActorID    string            `json:"actor_id"`
	TenantID   string            `json:"tenant_id"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	AccessedAt time.Time         `json:"accessed_at"`
	ExpiresAt  time.Time         `json:"expires_at,omitempty"`
}

// NewMemory creates a Memory record with a unique ID, full confidence, and
// timestamps initialized to now.
func NewMemory(memType Type, content json.RawMessage, actorID, tenantID string) *Memory {
	now := time.Now()
	return &Memory{
		ID:         uuid.New().String(),
		Type:       memType,
		Content:    content,
		Confidence: 1.0,
		ActorID:    actorID,
		TenantID:   tenantID,
		CreatedAt:  now,
		AccessedAt: now,
	}
}
