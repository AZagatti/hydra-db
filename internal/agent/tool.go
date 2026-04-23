package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Tool is the contract for any callable capability that an agent can invoke
// during execution.
type Tool interface {
	Name() string
	Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

// ToolRegistry is a thread-safe catalog of Tools, ensuring each tool name is
// unique within the agent runtime.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewToolRegistry creates an empty registry ready for tool registration.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register adds a Tool to the registry, rejecting duplicates by name.
func (r *ToolRegistry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = tool
	return nil
}

// Get looks up a Tool by its registered name.
func (r *ToolRegistry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	return t, nil
}

// List returns the names of all registered Tools.
func (r *ToolRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	return out
}
