package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/azagatti/hydra-db/internal/body"
)

// Adapter converts between Slack event payloads and Hydra Envelopes, with
// built-in signature verification.
type Adapter struct {
	SigningSecret string
}

// NewAdapter creates an Adapter using the given signing secret for
// request verification.
func NewAdapter(signingSecret string) *Adapter {
	return &Adapter{SigningSecret: signingSecret}
}

// Name returns "slack", identifying this adapter's transport.
func (a *Adapter) Name() string {
	return "slack"
}

// ConvertToEnvelope parses a Slack event payload into an Envelope, mapping user
// and team info to actor identity and tenant.
func (a *Adapter) ConvertToEnvelope(raw []byte, metadata map[string]string) (*body.Envelope, error) {
	var event struct {
		Event struct {
			Text    string `json:"text"`
			User    string `json:"user"`
			Channel string `json:"channel"`
		} `json:"event"`
		TeamID string `json:"team_id"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, fmt.Errorf("slack adapter: parse json: %w", err)
	}

	identity := body.Identity{
		ID:       event.Event.User,
		Kind:     body.ActorHuman,
		TenantID: event.TeamID,
	}
	tenant := body.Tenant{
		ID: event.TeamID,
	}

	env := body.NewEnvelope(body.EnvelopeRequest, "chat", identity, tenant)

	payload, _ := json.Marshal(map[string]any{
		"text":    event.Event.Text,
		"user":    event.Event.User,
		"channel": event.Event.Channel,
		"team_id": event.TeamID,
	})
	env.Payload = payload

	env.Metadata = make(map[string]string)
	for k, v := range metadata {
		env.Metadata[k] = v
	}
	env.Metadata["channel"] = event.Event.Channel

	return env, nil
}

// ConvertFromEnvelope serializes an Envelope into a Slack-compatible JSON
// message with a "text" field.
func (a *Adapter) ConvertFromEnvelope(env *body.Envelope) ([]byte, error) {
	var text string
	if len(env.Payload) > 0 {
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(env.Payload, &payload); err == nil && payload.Text != "" {
			text = payload.Text
		} else {
			text = string(env.Payload)
		}
	}

	msg := map[string]string{"text": text}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("slack adapter: marshal message: %w", err)
	}
	return data, nil
}

// VerifySignature validates that a request body was signed by Slack using
// HMAC-SHA256, returning true when the signing secret is empty (disabled).
func (a *Adapter) VerifySignature(body []byte, timestamp, signature string) bool {
	if a.SigningSecret == "" {
		return true
	}

	baseString := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(a.SigningSecret))
	mac.Write([]byte(baseString))
	expectedSig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedSig), []byte(signature))
}
