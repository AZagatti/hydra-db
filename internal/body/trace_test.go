package body

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSpan(t *testing.T) {
	before := time.Now()
	s := NewSpan("trace-123", "controller", "handle_request")
	after := time.Now()

	assert.NotEmpty(t, s.ID, "ID should be populated with a UUID")
	assert.Equal(t, "trace-123", s.TraceID)
	assert.Empty(t, s.ParentID, "ParentID should be empty for root spans")
	assert.Equal(t, "controller", s.Head)
	assert.Equal(t, "handle_request", s.Action)
	assert.Equal(t, SpanPending, s.Status)
	assert.True(t, !s.StartedAt.Before(before) && !s.StartedAt.After(after), "StartedAt should be around time.Now()")
	assert.True(t, s.EndedAt.IsZero(), "EndedAt should be zero for a new span")
	assert.NotNil(t, s.Metadata)
	assert.Empty(t, s.Metadata)
}

func TestSpan_Child(t *testing.T) {
	parent := NewSpan("trace-456", "service", "process")
	child := Child(parent, "sub_task")

	require.NotNil(t, child)

	assert.NotEmpty(t, child.ID, "child should have its own UUID")
	assert.NotEqual(t, parent.ID, child.ID, "child ID must differ from parent ID")
	assert.Equal(t, parent.TraceID, child.TraceID, "child inherits trace ID")
	assert.Equal(t, parent.ID, child.ParentID, "child's ParentID is parent's ID")
	assert.Equal(t, parent.Head, child.Head, "child inherits head")
	assert.Equal(t, "sub_task", child.Action)
	assert.Equal(t, SpanPending, child.Status)
	assert.False(t, child.StartedAt.IsZero(), "StartedAt should be set")
	assert.True(t, child.EndedAt.IsZero(), "EndedAt should be zero")
}

func TestSpan_Finish(t *testing.T) {
	s := NewSpan("trace-789", "gateway", "forward")
	before := time.Now()
	s.Finish(SpanOK)
	after := time.Now()

	assert.Equal(t, SpanOK, s.Status)
	assert.False(t, s.EndedAt.IsZero(), "EndedAt should be set after Finish")
	assert.True(t, !s.EndedAt.Before(before) && !s.EndedAt.After(after), "EndedAt should be around time.Now()")
}

func TestSpan_Finish_WithStatus(t *testing.T) {
	s := NewSpan("t", "h", "a")
	s.Finish(SpanError)
	assert.Equal(t, SpanError, s.Status)
}

func TestSpan_Duration(t *testing.T) {
	t.Run("returns zero before Finish", func(t *testing.T) {
		s := NewSpan("trace-dur", "head", "act")
		assert.Equal(t, time.Duration(0), s.Duration())
	})

	t.Run("returns positive duration after Finish", func(t *testing.T) {
		s := NewSpan("trace-dur", "head", "act")
		time.Sleep(10 * time.Millisecond)
		s.Finish(SpanOK)
		d := s.Duration()
		assert.GreaterOrEqual(t, d, 10*time.Millisecond, "duration should be at least the sleep time")
	})
}

func TestSpan_String(t *testing.T) {
	s := NewSpan("trace-str", "svc", "do_thing")
	str := s.String()

	assert.Contains(t, str, "span="+s.ID)
	assert.Contains(t, str, "trace=trace-str")
	assert.Contains(t, str, "head=svc")
	assert.Contains(t, str, "action=do_thing")
	assert.Contains(t, str, "status=pending")
	assert.True(t, strings.HasPrefix(str, "span="))
}

func TestSpan_WithMetadata(t *testing.T) {
	s := NewSpan("trace-meta", "head", "act")

	result := s.WithMetadata("region", "us-east-1")

	assert.NotSame(t, s, result, "WithMetadata should return a new span")
	assert.Equal(t, "us-east-1", result.Metadata["region"])
	assert.Empty(t, s.Metadata, "original span should not be mutated")

	chained := result.WithMetadata("env", "prod").WithMetadata("team", "platform")
	assert.Equal(t, "us-east-1", chained.Metadata["region"])
	assert.Equal(t, "prod", chained.Metadata["env"])
	assert.Equal(t, "platform", chained.Metadata["team"])
	assert.Len(t, chained.Metadata, 3)
}

func TestSpan_WithMetadata_NilMap(t *testing.T) {
	s := &Span{ID: "x", TraceID: "t", Head: "h", Action: "a", Status: SpanPending, StartedAt: time.Now()}
	assert.Nil(t, s.Metadata)

	result := s.WithMetadata("key", "val")
	assert.Equal(t, "val", result.Metadata["key"])
	assert.Nil(t, s.Metadata, "original span should not be mutated")
}
