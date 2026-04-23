package agent

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// State tracks the lifecycle stage of an Agent from creation through
// completion or failure.
type State string

const (
	// StateCreated is the initial state before the agent begins executing.
	StateCreated State = "created"
	// StateRunning means the agent's Func is actively executing.
	StateRunning State = "running"
	// StateDone means the agent completed its Func successfully.
	StateDone State = "done"
	// StateFailed means the agent's Func returned a non-nil error.
	StateFailed State = "failed"
	// StateTimedOut means the agent exceeded its execution deadline.
	StateTimedOut State = "timed_out"
)

// Agent represents a single autonomous unit of work within the runtime, carrying
// its own identity, context, tools, and execution function.
type Agent struct {
	ID         string
	Name       string
	State      State
	Context    *Context
	Tools      []Tool
	Result     any
	Error      error
	CreatedAt  time.Time
	FinishedAt time.Time

	Func Func
}

// Func is the function signature an agent executes when spawned. It
// receives a context for cancellation and the Agent itself for state access.
type Func func(ctx context.Context, agent *Agent) (any, error)

// Option configures an Agent during construction.
type Option func(*Agent)

// WithTools assigns a set of Tools to the agent for use during execution.
func WithTools(tools ...Tool) Option {
	return func(a *Agent) {
		a.Tools = append(a.Tools, tools...)
	}
}

// WithContext provides a pre-built Context to the agent, carrying session,
// identity, and tenant information.
func WithContext(ac *Context) Option {
	return func(a *Agent) {
		a.Context = ac
	}
}

// NewAgent creates a ready-to-run Agent with a unique ID, the given name and
// function, and any additional options applied.
func NewAgent(name string, agFunc Func, opts ...Option) *Agent {
	a := &Agent{
		ID:        uuid.New().String(),
		Name:      name,
		State:     StateCreated,
		Func:      agFunc,
		CreatedAt: time.Now(),
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}
