// Package adapter defines the interface contract for Hydra's inbound adapters.
//
// Each adapter translates between an external channel (HTTP, CLI, Slack, etc.)
// and the internal Envelope-based message system. This decouples transport
// concerns from domain logic and allows new channels to be added without
// modifying core Hydra code.
package adapter
