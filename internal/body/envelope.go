package body

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

// EnvelopeType classifies the kind of message flowing through Hydra.
type EnvelopeType string

const (
	// EnvelopeRequest represents an inbound request from an actor.
	EnvelopeRequest EnvelopeType = "request"
	// EnvelopeResponse represents an outbound reply to a request.
	EnvelopeResponse EnvelopeType = "response"
	// EnvelopeEvent represents a side-effect notification (e.g. resource created).
	EnvelopeEvent EnvelopeType = "event"
	// EnvelopeError represents a failure response.
	EnvelopeError EnvelopeType = "error"
)

// ActorKind enumerates the categories of actors that can originate requests.
type ActorKind string

const (
	// ActorHuman represents an end-user interacting through a client.
	ActorHuman ActorKind = "human"
	// ActorAgent represents an autonomous AI agent.
	ActorAgent ActorKind = "agent"
	// ActorTool represents a tool or integration invoking Hydra programmatically.
	ActorTool ActorKind = "tool"
	// ActorSystem represents internal system processes.
	ActorSystem ActorKind = "system"
)

// Identity represents the actor (human, agent, tool, or system) behind a request.
// Every envelope carries an identity so all heads can make policy decisions.
type Identity struct {
	ID       string    `json:"id" validate:"required"`
	Kind     ActorKind `json:"kind" validate:"required,oneof=human agent tool system"`
	Roles    []string  `json:"roles,omitempty"`
	TenantID string    `json:"tenant_id" validate:"required"`
}

// Tenant scopes all data and policy within a single customer or organization.
type Tenant struct {
	ID   string `json:"id" validate:"required"`
	Name string `json:"name,omitempty"`
}

// Envelope is the universal message wrapper that flows between heads.
// It carries identity, tracing, and routing metadata so every head has
// a consistent context without depending on transport-specific details.
type Envelope struct {
	ID        string            `json:"id" validate:"required"`
	TraceID   string            `json:"trace_id" validate:"required"`
	Actor     Identity          `json:"actor"`
	Tenant    Tenant            `json:"tenant"`
	Type      EnvelopeType      `json:"type" validate:"required,oneof=request response event error"`
	Action    string            `json:"action" validate:"required"`
	Payload   json.RawMessage   `json:"payload"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// validate is the shared struct validator used by Envelope.Validate.
var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())
}

// NewEnvelope creates a new envelope with a generated ID, trace ID, and timestamp.
func NewEnvelope(typ EnvelopeType, action string, actor Identity, tenant Tenant) *Envelope {
	return &Envelope{
		ID:        uuid.New().String(),
		TraceID:   uuid.New().String(),
		Actor:     actor,
		Tenant:    tenant,
		Type:      typ,
		Action:    action,
		Timestamp: time.Now(),
	}
}

// WithPayload marshals v into the envelope payload, returning a shallow copy
// so the original envelope remains unchanged.
func (e *Envelope) WithPayload(v any) (*Envelope, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	cp := *e
	cp.Payload = data
	return &cp, nil
}

// DecodePayload unmarshals the raw payload into target. Returns an error if
// the payload is empty or the JSON cannot be decoded.
func (e *Envelope) DecodePayload(target any) error {
	if len(e.Payload) == 0 {
		return fmt.Errorf("payload is empty")
	}
	if err := json.Unmarshal(e.Payload, target); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}
	return nil
}

// WithMetadata returns a copy of the envelope with the given key-value pair
// added to the metadata map. The original envelope is not modified.
func (e *Envelope) WithMetadata(key, value string) *Envelope {
	cp := *e
	if cp.Metadata == nil {
		cp.Metadata = make(map[string]string)
	} else {
		cp.Metadata = make(map[string]string, len(e.Metadata))
		for k, v := range e.Metadata {
			cp.Metadata[k] = v
		}
	}
	cp.Metadata[key] = value
	return &cp
}

// Validate checks that all required envelope fields are present and valid.
func (e *Envelope) Validate() error {
	return validate.Struct(e)
}
