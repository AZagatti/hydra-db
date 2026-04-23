package agent

import (
	"github.com/azagatti/hydra-db/internal/body"
	"github.com/google/uuid"
)

// Context carries the session, identity, and tenant metadata for a single
// agent invocation, along with arbitrary key-value storage for inter-tool
// communication.
type Context struct {
	SessionID string
	Actor     body.Identity
	Tenant    body.Tenant
	Memory    map[string]any
	Variables map[string]string
}

// NewContext creates a context scoped to a new session for the given actor
// and tenant.
func NewContext(actor body.Identity, tenant body.Tenant) *Context {
	return &Context{
		SessionID: uuid.New().String(),
		Actor:     actor,
		Tenant:    tenant,
		Memory:    make(map[string]any),
		Variables: make(map[string]string),
	}
}

// Set stores an arbitrary value in the agent's memory, useful for passing data
// between tools within a single agent run.
func (ac *Context) Set(key string, value any) {
	ac.Memory[key] = value
}

// Get retrieves a value from the agent's memory by key.
func (ac *Context) Get(key string) (any, bool) {
	v, ok := ac.Memory[key]
	return v, ok
}

// SetVar stores a string variable in the agent's variable map, intended for
// lightweight string-based state.
func (ac *Context) SetVar(key, value string) {
	ac.Variables[key] = value
}

// GetVar retrieves a string variable from the agent's variable map.
func (ac *Context) GetVar(key string) (string, bool) {
	v, ok := ac.Variables[key]
	return v, ok
}
