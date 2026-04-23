package execution

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// JobState tracks the lifecycle of a Job from enqueue through completion or
// failure.
type JobState string

const (
	// JobPending means the job is waiting to be scheduled.
	JobPending JobState = "pending"
	// JobRunning means the job is actively being processed by a handler.
	JobRunning JobState = "running"
	// JobCompleted means the job finished successfully.
	JobCompleted JobState = "completed"
	// JobFailed means the job exhausted its retry attempts.
	JobFailed JobState = "failed"
	// JobRetrying means the job failed but will be retried.
	JobRetrying JobState = "retrying"
)

// Job represents a unit of work submitted to the execution plane, carrying its
// payload, retry configuration, and lifecycle timestamps.
type Job struct {
	ID          string
	Type        string
	Payload     json.RawMessage
	State       JobState
	Result      json.RawMessage
	Error       string
	Attempts    int
	MaxAttempts int
	CreatedAt   time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
	RunAfter    time.Time
}

// JobOption configures a Job during construction.
type JobOption func(*Job)

// WithMaxAttempts overrides the default retry limit for a Job.
func WithMaxAttempts(n int) JobOption {
	return func(j *Job) { j.MaxAttempts = n }
}

// WithRunAfter delays the job until the specified time, enabling scheduled
// execution.
func WithRunAfter(t time.Time) JobOption {
	return func(j *Job) { j.RunAfter = t }
}

// WithResult pre-populates the job with a result, useful for testing or
// replaying completed work.
func WithResult(r json.RawMessage) JobOption {
	return func(j *Job) { j.Result = r }
}

// NewJob creates a Job with a unique ID, pending state, and 3 default retry
// attempts.
func NewJob(jobType string, payload json.RawMessage, opts ...JobOption) *Job {
	j := &Job{
		ID:          uuid.New().String(),
		Type:        jobType,
		Payload:     payload,
		State:       JobPending,
		MaxAttempts: 3,
		Attempts:    0,
		CreatedAt:   time.Now(),
	}
	for _, opt := range opts {
		opt(j)
	}
	return j
}
