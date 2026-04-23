package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/azagatti/hydra-db/internal/body"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdapter_Name(t *testing.T) {
	t.Parallel()
	a := NewAdapter("secret")
	assert.Equal(t, "slack", a.Name())
}

func TestAdapter_ConvertToEnvelope(t *testing.T) {
	t.Parallel()
	a := NewAdapter("secret")

	raw := []byte(`{"event":{"text":"deploy staging","user":"U123","channel":"C456"},"team_id":"T789"}`)
	env, err := a.ConvertToEnvelope(raw, nil)
	require.NoError(t, err)
	require.NotNil(t, env)

	assert.Equal(t, "chat", env.Action)
	assert.Equal(t, body.EnvelopeRequest, env.Type)
	assert.Equal(t, "U123", env.Actor.ID)
	assert.Equal(t, body.ActorHuman, env.Actor.Kind)
	assert.Equal(t, "T789", env.Actor.TenantID)
	assert.Equal(t, "T789", env.Tenant.ID)
	assert.Equal(t, "C456", env.Metadata["channel"])

	var payload map[string]any
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	assert.Equal(t, "deploy staging", payload["text"])
	assert.Equal(t, "U123", payload["user"])
}

func TestAdapter_ConvertToEnvelope_InvalidJSON(t *testing.T) {
	t.Parallel()
	a := NewAdapter("secret")

	_, err := a.ConvertToEnvelope([]byte(`{invalid`), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse json")
}

func TestAdapter_ConvertFromEnvelope(t *testing.T) {
	t.Parallel()
	a := NewAdapter("secret")

	env := body.NewEnvelope(body.EnvelopeResponse, "chat", body.Identity{
		ID: "U123", Kind: body.ActorHuman, TenantID: "T789",
	}, body.Tenant{ID: "T789"})
	payload, _ := json.Marshal(map[string]string{"text": "Deployment complete"})
	env.Payload = payload

	out, err := a.ConvertFromEnvelope(env)
	require.NoError(t, err)

	var msg map[string]string
	require.NoError(t, json.Unmarshal(out, &msg))
	assert.Equal(t, "Deployment complete", msg["text"])
}

func TestAdapter_VerifySignature_Valid(t *testing.T) {
	t.Parallel()
	a := NewAdapter("my-signing-secret")

	body := []byte(`{"event":{"text":"hello"}}`)
	timestamp := "1234567890"

	mac := hmacSHA256([]byte("my-signing-secret"), []byte("v0:"+timestamp+":"+string(body)))
	sig := "v0=" + mac

	assert.True(t, a.VerifySignature(body, timestamp, sig))
}

func TestAdapter_VerifySignature_Invalid(t *testing.T) {
	t.Parallel()
	a := NewAdapter("my-signing-secret")

	assert.False(t, a.VerifySignature([]byte(`{}`), "123", "v0=invalidsignature"))
}

func TestAdapter_VerifySignature_EmptySecret(t *testing.T) {
	t.Parallel()
	a := NewAdapter("")

	assert.True(t, a.VerifySignature([]byte(`{}`), "123", ""))
	assert.True(t, a.VerifySignature(nil, "", ""))
}

func hmacSHA256(key, data []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
