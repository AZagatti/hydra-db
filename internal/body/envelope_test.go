package body

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	validActor  = Identity{ID: "a1", Kind: ActorHuman, TenantID: "t1"}
	validTenant = Tenant{ID: "t1", Name: "acme"}
)

func TestNewEnvelope(t *testing.T) {
	before := time.Now()
	env := NewEnvelope(EnvelopeRequest, "user.create", validActor, validTenant)
	after := time.Now()

	assert.NotEmpty(t, env.ID)
	assert.NotEmpty(t, env.TraceID)
	assert.NotEqual(t, env.ID, env.TraceID)
	assert.Equal(t, EnvelopeRequest, env.Type)
	assert.Equal(t, "user.create", env.Action)
	assert.Equal(t, validActor, env.Actor)
	assert.Equal(t, validTenant, env.Tenant)
	assert.True(t, !env.Timestamp.Before(before) && !env.Timestamp.After(after))
}

func TestNewEnvelope_Validation(t *testing.T) {
	t.Run("valid envelope passes", func(t *testing.T) {
		env := NewEnvelope(EnvelopeRequest, "user.create", validActor, validTenant)
		assert.NoError(t, env.Validate())
	})

	t.Run("invalid type fails", func(t *testing.T) {
		env := NewEnvelope(EnvelopeType("bogus"), "user.create", validActor, validTenant)
		assert.Error(t, env.Validate())
	})

	t.Run("empty action fails", func(t *testing.T) {
		env := NewEnvelope(EnvelopeRequest, "", validActor, validTenant)
		assert.Error(t, env.Validate())
	})
}

func TestEnvelope_WithPayload(t *testing.T) {
	type widget struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}

	original := NewEnvelope(EnvelopeRequest, "widget.create", validActor, validTenant)
	w := widget{Name: "foo", N: 42}

	got, err := original.WithPayload(w)
	require.NoError(t, err)

	var decoded widget
	require.NoError(t, got.DecodePayload(&decoded))
	assert.Equal(t, w, decoded)

	assert.Nil(t, original.Payload, "original should not be mutated")
}

func TestEnvelope_WithPayload_InvalidJSON(t *testing.T) {
	env := NewEnvelope(EnvelopeRequest, "bad", validActor, validTenant)
	_, err := env.WithPayload(make(chan int))
	assert.Error(t, err)
}

func TestEnvelope_DecodePayload(t *testing.T) {
	type item struct {
		SKU string `json:"sku"`
	}

	env := NewEnvelope(EnvelopeResponse, "item.get", validActor, validTenant)
	raw, _ := json.Marshal(item{SKU: "ABC-123"})
	env.Payload = raw

	var target item
	require.NoError(t, env.DecodePayload(&target))
	assert.Equal(t, "ABC-123", target.SKU)
}

func TestEnvelope_DecodePayload_EmptyPayload(t *testing.T) {
	env := NewEnvelope(EnvelopeResponse, "item.get", validActor, validTenant)
	assert.Error(t, env.DecodePayload(&struct{}{}))
}

func TestEnvelope_WithMetadata(t *testing.T) {
	original := NewEnvelope(EnvelopeEvent, "tick", validActor, validTenant)

	WithFoo := original.WithMetadata("foo", "bar")
	WithBoth := WithFoo.WithMetadata("baz", "qux")

	assert.Equal(t, "bar", WithFoo.Metadata["foo"])
	assert.Equal(t, "bar", WithBoth.Metadata["foo"])
	assert.Equal(t, "qux", WithBoth.Metadata["baz"])
	assert.Nil(t, original.Metadata, "original should not be mutated")
}

func TestEnvelope_Validate_MissingFields(t *testing.T) {
	base := NewEnvelope(EnvelopeRequest, "user.create", validActor, validTenant)

	t.Run("missing ID", func(t *testing.T) {
		env := *base
		env.ID = ""
		assert.Error(t, env.Validate())
	})

	t.Run("missing TraceID", func(t *testing.T) {
		env := *base
		env.TraceID = ""
		assert.Error(t, env.Validate())
	})

	t.Run("missing Type", func(t *testing.T) {
		env := *base
		env.Type = ""
		assert.Error(t, env.Validate())
	})

	t.Run("missing Action", func(t *testing.T) {
		env := *base
		env.Action = ""
		assert.Error(t, env.Validate())
	})
}
