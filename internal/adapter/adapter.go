package adapter

import "github.com/azagatti/hydra-db/internal/body"

// Adapter translates between raw transport payloads and Hydra Envelopes so
// that external sources (HTTP, CLI, Slack, etc.) share a unified message
// format.
type Adapter interface {
	// Name identifies the transport this adapter handles.
	Name() string
	// ConvertToEnvelope parses raw bytes and metadata into an Envelope.
	ConvertToEnvelope(raw []byte, metadata map[string]string) (*body.Envelope, error)
	// ConvertFromEnvelope serializes an Envelope back into transport-specific bytes.
	ConvertFromEnvelope(env *body.Envelope) ([]byte, error)
}
