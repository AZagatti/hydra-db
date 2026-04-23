package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/azagatti/hydra-db/internal/body"
)

// Runtime is the top-level agent plane that manages agent lifecycles, tool
// registration, and execution via a shared Executor.
type Runtime struct {
	registry  *ToolRegistry
	executor  *Executor
	agents    map[string]*Agent
	mu        sync.RWMutex
	cancelFns map[string]context.CancelFunc
}

// NewRuntime creates a Runtime with a default ToolRegistry and Executor.
func NewRuntime() *Runtime {
	return &Runtime{
		registry:  NewToolRegistry(),
		executor:  NewExecutor(),
		agents:    make(map[string]*Agent),
		cancelFns: make(map[string]context.CancelFunc),
	}
}

// Name implements body.Head, identifying this plane as "agent".
func (r *Runtime) Name() string {
	return "agent"
}

// Start is a no-op; the runtime is ready immediately after construction.
func (r *Runtime) Start(_ context.Context) error {
	return nil
}

// Stop cancels all running agent contexts so in-flight executions terminate.
func (r *Runtime) Stop(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, cancel := range r.cancelFns {
		cancel()
		delete(r.cancelFns, id)
	}
	return nil
}

// Health reports the runtime as healthy.
func (r *Runtime) Health() body.HealthReport {
	return body.HealthReport{
		Head:   r.Name(),
		Status: body.HealthHealthy,
	}
}

// Spawn creates a new Agent from the given function and options, then executes
// it through the shared Executor. Returns the Agent with its final state.
func (r *Runtime) Spawn(ctx context.Context, name string, fn Func, opts ...Option) (*Agent, error) {
	agent := NewAgent(name, fn, opts...)

	execCtx, cancel := context.WithCancel(ctx)

	r.mu.Lock()
	r.agents[agent.ID] = agent
	r.cancelFns[agent.ID] = cancel
	r.mu.Unlock()

	err := r.executor.Execute(execCtx, agent)

	r.mu.Lock()
	delete(r.cancelFns, agent.ID)
	r.mu.Unlock()

	if err != nil {
		return agent, fmt.Errorf("agent %q execution failed: %w", name, err)
	}

	return agent, nil
}

// GetAgent retrieves a previously spawned Agent by its ID.
func (r *Runtime) GetAgent(id string) (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent %q not found", id)
	}
	return a, nil
}

// ListAgents returns all agents managed by this runtime.
func (r *Runtime) ListAgents() []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*Agent, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, a)
	}
	return out
}

// RegisterTool adds a Tool to the runtime's shared registry so agents can
// discover and invoke it.
func (r *Runtime) RegisterTool(tool Tool) error {
	return r.registry.Register(tool)
}
