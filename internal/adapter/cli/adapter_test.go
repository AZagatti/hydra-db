package cli

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
	assert.Equal(t, "cli", a.Name())
}

func TestAdapter_ConvertToEnvelope(t *testing.T) {
	t.Parallel()
	a := NewAdapter()

	raw := []byte(`{"action":"query","input":"show tables","actor_id":"admin","tenant_id":"org-1"}`)
	metadata := map[string]string{"shell": "bash"}

	env, err := a.ConvertToEnvelope(raw, metadata)
	require.NoError(t, err)
	require.NotNil(t, env)

	assert.Equal(t, "query", env.Action)
	assert.Equal(t, body.EnvelopeRequest, env.Type)
	assert.Equal(t, "admin", env.Actor.ID)
	assert.Equal(t, body.ActorHuman, env.Actor.Kind)
	assert.Equal(t, "org-1", env.Tenant.ID)
	assert.Equal(t, "bash", env.Metadata["shell"])

	var payload map[string]string
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	assert.Equal(t, "show tables", payload["input"])
}

func TestAdapter_ConvertToEnvelope_MissingAction(t *testing.T) {
	t.Parallel()
	a := NewAdapter()

	_, err := a.ConvertToEnvelope([]byte(`{"input":"hello"}`), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "action is required")
}

func TestAdapter_ConvertFromEnvelope_Success(t *testing.T) {
	t.Parallel()
	a := NewAdapter()

	env := body.NewEnvelope(body.EnvelopeResponse, "query", body.Identity{
		ID: "admin", Kind: body.ActorHuman, TenantID: "org-1",
	}, body.Tenant{ID: "org-1"})
	payload, _ := json.Marshal(map[string]any{"tables": []string{"users", "orders"}})
	env.Payload = payload

	out, err := a.ConvertFromEnvelope(env)
	require.NoError(t, err)

	text := string(out)
	assert.Contains(t, text, "Action: query")
	assert.Contains(t, text, "Result:")
	assert.Contains(t, text, "users")
}

func TestAdapter_ConvertFromEnvelope_Error(t *testing.T) {
	t.Parallel()
	a := NewAdapter()

	env := body.NewEnvelope(body.EnvelopeError, "query", body.Identity{
		ID: "admin", Kind: body.ActorHuman, TenantID: "org-1",
	}, body.Tenant{ID: "org-1"})
	env.Payload = json.RawMessage(`"connection refused"`)

	out, err := a.ConvertFromEnvelope(env)
	require.NoError(t, err)

	assert.Equal(t, "ERROR: connection refused", string(out))
}
