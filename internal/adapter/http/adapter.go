package http

import (
	"encoding/json"
	"fmt"

	"github.com/azagatti/hydra-db/internal/body"
)

// Adapter converts between JSON HTTP request bodies and Hydra Envelopes.
type Adapter struct{}

// NewAdapter creates a new Adapter.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Name returns "http", identifying this adapter's transport.
func (a *Adapter) Name() string {
	return "http"
}

// ConvertToEnvelope parses a JSON payload into an Envelope, extracting action,
// actor identity, and tenant information from the body fields.
func (a *Adapter) ConvertToEnvelope(raw []byte, metadata map[string]string) (*body.Envelope, error) {
	var input struct {
		Action    string `json:"action"`
		ActorID   string `json:"actor_id"`
		ActorKind string `json:"actor_kind"`
		TenantID  string `json:"tenant_id"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("http adapter: parse json: %w", err)
	}
	if input.Action == "" {
		return nil, fmt.Errorf("http adapter: action is required")
	}

	kind := body.ActorKind(input.ActorKind)
	if kind == "" {
		kind = body.ActorHuman
	}

	identity := body.Identity{
		ID:       input.ActorID,
		Kind:     kind,
		TenantID: input.TenantID,
	}
	tenant := body.Tenant{
		ID: input.TenantID,
	}

	env := body.NewEnvelope(body.EnvelopeRequest, input.Action, identity, tenant)
	env.Payload = json.RawMessage(raw)

	if metadata != nil {
		env.Metadata = make(map[string]string, len(metadata))
		for k, v := range metadata {
			env.Metadata[k] = v
		}
	}

	return env, nil
}

// ConvertFromEnvelope serializes an Envelope back to JSON for HTTP transport.
func (a *Adapter) ConvertFromEnvelope(env *body.Envelope) ([]byte, error) {
	data, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("http adapter: marshal envelope: %w", err)
	}
	return data, nil
}
