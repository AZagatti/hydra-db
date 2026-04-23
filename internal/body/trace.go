package body

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SpanStatus tracks the outcome of a trace span.
type SpanStatus string

const (
	// SpanOK indicates the span completed successfully.
	SpanOK SpanStatus = "ok"
	// SpanError indicates the span encountered a failure.
	SpanError SpanStatus = "error"
	// SpanPending indicates the span is still in progress.
	SpanPending SpanStatus = "pending"
)

// Span represents a unit of work within a distributed trace, allowing
// heads to correlate causality across the system.
type Span struct {
	ID        string            `json:"id"`
	TraceID   string            `json:"trace_id"`
	ParentID  string            `json:"parent_id,omitempty"`
	Head      string            `json:"head"`
	Action    string            `json:"action"`
	Status    SpanStatus        `json:"status"`
	StartedAt time.Time         `json:"started_at"`
	EndedAt   time.Time         `json:"ended_at,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// NewSpan creates a root span for a new trace, ready for tracking.
func NewSpan(traceID, head, action string) *Span {
	return &Span{
		ID:        uuid.New().String(),
		TraceID:   traceID,
		Head:      head,
		Action:    action,
		Status:    SpanPending,
		StartedAt: time.Now(),
		Metadata:  make(map[string]string),
	}
}

// Child creates a new span that is a child of parent, inheriting its
// trace ID and head for causal correlation.
func Child(parent *Span, action string) *Span {
	return &Span{
		ID:        uuid.New().String(),
		TraceID:   parent.TraceID,
		ParentID:  parent.ID,
		Head:      parent.Head,
		Action:    action,
		Status:    SpanPending,
		StartedAt: time.Now(),
		Metadata:  make(map[string]string),
	}
}

// Finish marks the span as complete with the given status and records the end time.
func (s *Span) Finish(status SpanStatus) {
	s.Status = status
	s.EndedAt = time.Now()
}

// Duration returns the elapsed time of the span, or zero if it has not finished.
func (s *Span) Duration() time.Duration {
	if s.EndedAt.IsZero() {
		return 0
	}
	return s.EndedAt.Sub(s.StartedAt)
}

// String returns a human-readable summary of the span for logging.
func (s *Span) String() string {
	return fmt.Sprintf("span=%s trace=%s head=%s action=%s status=%s", s.ID, s.TraceID, s.Head, s.Action, s.Status)
}

// WithMetadata attaches a key-value pair to the span for diagnostic context.
func (s *Span) WithMetadata(key, value string) *Span {
	cp := *s
	if cp.Metadata == nil {
		cp.Metadata = make(map[string]string)
	} else {
		cp.Metadata = make(map[string]string, len(s.Metadata))
		for k, v := range s.Metadata {
			cp.Metadata[k] = v
		}
	}
	cp.Metadata[key] = value
	return &cp
}
