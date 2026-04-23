package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/azagatti/hydra-db/internal/body"
)

// Adapter converts between CLI-style JSON input and Hydra Envelopes, with
// human-readable output formatting.
type Adapter struct{}

// NewAdapter creates a new Adapter.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Name returns "cli", identifying this adapter's transport.
func (a *Adapter) Name() string {
	return "cli"
}

// ConvertToEnvelope parses CLI JSON input into an Envelope, defaulting the
// actor kind to human.
func (a *Adapter) ConvertToEnvelope(raw []byte, metadata map[string]string) (*body.Envelope, error) {
	var input struct {
		Action   string `json:"action"`
		Input    string `json:"input"`
		ActorID  string `json:"actor_id"`
		TenantID string `json:"tenant_id"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("cli adapter: parse json: %w", err)
	}
	if input.Action == "" {
		return nil, fmt.Errorf("cli adapter: action is required")
	}

	identity := body.Identity{
		ID:       input.ActorID,
		Kind:     body.ActorHuman,
		TenantID: input.TenantID,
	}
	tenant := body.Tenant{
		ID: input.TenantID,
	}

	env := body.NewEnvelope(body.EnvelopeRequest, input.Action, identity, tenant)

	payload, _ := json.Marshal(map[string]string{"input": input.Input})
	env.Payload = payload

	if metadata != nil {
		env.Metadata = make(map[string]string, len(metadata))
		for k, v := range metadata {
			env.Metadata[k] = v
		}
	}

	return env, nil
}

// ConvertFromEnvelope formats an Envelope as a human-readable CLI output,
// prefixing errors with "ERROR:" and pretty-printing JSON payloads.
func (a *Adapter) ConvertFromEnvelope(env *body.Envelope) ([]byte, error) {
	if env.Type == body.EnvelopeError {
		errMsg := strings.TrimSpace(string(env.Payload))
		errMsg = strings.Trim(errMsg, `"`)
		return []byte(fmt.Sprintf("ERROR: %s", errMsg)), nil
	}

	var prettyPayload strings.Builder
	if len(env.Payload) > 0 {
		var indented any
		if err := json.Unmarshal(env.Payload, &indented); err == nil {
			formatted, err := json.MarshalIndent(indented, "", "  ")
			if err == nil {
				prettyPayload.Write(formatted)
			}
		}
	}

	return []byte(fmt.Sprintf("Action: %s\nResult: %s", env.Action, prettyPayload.String())), nil
}
