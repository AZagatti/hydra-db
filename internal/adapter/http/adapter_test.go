package http

import (
	"encoding/json"
	"testing"

	"github.com/azagatti/hydra-db/internal/body"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdapter_Name(t *testing.T) {
	t.Parallel()
	a := NewAdapter()
	assert.Equal(t, "http", a.Name())
}

func TestAdapter_ConvertToEnvelope(t *testing.T) {
	t.Parallel()
	a := NewAdapter()

	raw := []byte(`{"action":"chat","actor_id":"user-1","actor_kind":"human","tenant_id":"org-1","message":"hello"}`)
	metadata := map[string]string{"source": "test"}

	env, err := a.ConvertToEnvelope(raw, metadata)
	require.NoError(t, err)
	require.NotNil(t, env)

	assert.Equal(t, "chat", env.Action)
	assert.Equal(t, body.EnvelopeRequest, env.Type)
	assert.Equal(t, "user-1", env.Actor.ID)
	assert.Equal(t, body.ActorHuman, env.Actor.Kind)
	assert.Equal(t, "org-1", env.Actor.TenantID)
	assert.Equal(t, "org-1", env.Tenant.ID)
	assert.Equal(t, "test", env.Metadata["source"])

	var payload map[string]any
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	assert.Equal(t, "hello", payload["message"])
}

func TestAdapter_ConvertToEnvelope_MissingAction(t *testing.T) {
	t.Parallel()
	a := NewAdapter()

	raw := []byte(`{"actor_id":"user-1"}`)
	_, err := a.ConvertToEnvelope(raw, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "action is required")
}

func TestAdapter_ConvertToEnvelope_InvalidJSON(t *testing.T) {
	t.Parallel()
	a := NewAdapter()

	_, err := a.ConvertToEnvelope([]byte(`{invalid`), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse json")
}

func TestAdapter_ConvertFromEnvelope(t *testing.T) {
	t.Parallel()
	a := NewAdapter()

	env := body.NewEnvelope(body.EnvelopeResponse, "chat", body.Identity{
		ID: "user-1", Kind: body.ActorHuman, TenantID: "org-1",
	}, body.Tenant{ID: "org-1"})

	data, err := a.ConvertFromEnvelope(env)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result))
	assert.Equal(t, "chat", result["action"])
	assert.Equal(t, "response", result["type"])
}
