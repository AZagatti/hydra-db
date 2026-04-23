package agent

import (
	"context"
	"time"
)

// Executor runs an Agent's Func with configurable retry and timeout policies,
// updating the Agent's state throughout the lifecycle.
type Executor struct {
	MaxRetries int
	Timeout    time.Duration
}

// ExecutorOption configures an Executor during construction.
type ExecutorOption func(*Executor)

// WithMaxRetries sets the number of times the Executor will retry a failed
// agent function before giving up.
func WithMaxRetries(n int) ExecutorOption {
	return func(e *Executor) {
		e.MaxRetries = n
	}
}

// WithTimeout sets the per-attempt execution deadline for the agent function.
func WithTimeout(d time.Duration) ExecutorOption {
	return func(e *Executor) {
		e.Timeout = d
	}
}

// NewExecutor creates an Executor with sensible defaults (0 retries, 60s
// timeout) and applies any given options.
func NewExecutor(opts ...ExecutorOption) *Executor {
	e := &Executor{
		MaxRetries: 0,
		Timeout:    60 * time.Second,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Execute runs the agent's Func with exponential backoff retries, respecting
// the configured timeout and propagating cancellation through the context.
func (e *Executor) Execute(ctx context.Context, agent *Agent) error {
	var lastErr error

	for attempt := 0; attempt <= e.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := 100 * time.Millisecond * time.Duration(1<<(attempt-1))
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				agent.State = StateTimedOut
				agent.FinishedAt = time.Now()
				return ctx.Err()
			}
		}

		agent.State = StateRunning

		execCtx, cancel := context.WithTimeout(ctx, e.Timeout)
		result, err := agent.Func(execCtx, agent)
		cancel()

		if err == nil {
			agent.State = StateDone
			agent.Result = result
			agent.FinishedAt = time.Now()
			return nil
		}

		if execCtx.Err() == context.DeadlineExceeded {
			agent.State = StateTimedOut
			agent.FinishedAt = time.Now()
			return execCtx.Err()
		}

		lastErr = err
	}

	agent.State = StateFailed
	agent.Error = lastErr
	agent.FinishedAt = time.Now()
	return lastErr
}
