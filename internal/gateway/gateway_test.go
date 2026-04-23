package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/azagatti/hydra-db/internal/body"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGateway_Name(t *testing.T) {
	gw := NewGateway(body.GatewayConfig{})
	assert.Equal(t, "gateway", gw.Name())
}

func TestGateway_Health_NotStarted(t *testing.T) {
	gw := NewGateway(body.GatewayConfig{})
	report := gw.Health()
	assert.Equal(t, "gateway", report.Head)
	assert.Equal(t, body.HealthUnhealthy, report.Status)
	assert.Equal(t, "not running", report.Detail)
}

func TestGateway_RegisterHandler(t *testing.T) {
	gw := NewGateway(body.GatewayConfig{})

	var called bool
	gw.RegisterHandler("ping", func(_ context.Context, _ *body.Envelope) (*body.Envelope, error) {
		called = true
		return nil, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ping", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	assert.True(t, called, "registered handler should be invoked")
}

func TestGateway_ServeHTTP_ValidRequest(t *testing.T) {
	gw := NewGateway(body.GatewayConfig{})

	var captured *body.Envelope
	gw.RegisterHandler("chat", func(_ context.Context, env *body.Envelope) (*body.Envelope, error) {
		captured = env
		resp := *env
		resp.Type = body.EnvelopeResponse
		return &resp, nil
	})

	payload := `{"message": "hello hydra"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-ID", "trace-abc-123")
	req.Header.Set("Authorization", "Bearer s3cret")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, captured)
	assert.Equal(t, "chat", captured.Action)
	assert.Equal(t, "trace-abc-123", captured.TraceID)
	assert.Equal(t, body.EnvelopeRequest, captured.Type)
	assert.Equal(t, "Bearer s3cret", captured.Metadata["authorization"])

	var resp body.Envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, body.EnvelopeResponse, resp.Type)
	assert.Equal(t, "chat", resp.Action)
	assert.Equal(t, "trace-abc-123", resp.TraceID)
}

func TestGateway_ServeHTTP_NotFound(t *testing.T) {
	gw := NewGateway(body.GatewayConfig{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Contains(t, errResp["error"], "no handler for action")
}

func TestGateway_ServeHTTP_MethodNotAllowed(t *testing.T) {
	gw := NewGateway(body.GatewayConfig{})
	gw.RegisterHandler("chat", func(_ context.Context, env *body.Envelope) (*body.Envelope, error) {
		return env, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "method not allowed", errResp["error"])
}

func TestGateway_ServeHTTP_InvalidPath(t *testing.T) {
	gw := NewGateway(body.GatewayConfig{})

	req := httptest.NewRequest(http.MethodPost, "/invalid", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGateway_ServeHTTP_TraceID(t *testing.T) {
	t.Run("preserves provided trace ID", func(t *testing.T) {
		gw := NewGateway(body.GatewayConfig{})
		var captured *body.Envelope
		gw.RegisterHandler("echo", func(_ context.Context, env *body.Envelope) (*body.Envelope, error) {
			captured = env
			return env, nil
		})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/echo", strings.NewReader(`{}`))
		req.Header.Set("X-Trace-ID", "my-custom-trace")
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)

		require.NotNil(t, captured)
		assert.Equal(t, "my-custom-trace", captured.TraceID)
	})

	t.Run("generates trace ID when missing", func(t *testing.T) {
		gw := NewGateway(body.GatewayConfig{})
		var captured *body.Envelope
		gw.RegisterHandler("echo", func(_ context.Context, env *body.Envelope) (*body.Envelope, error) {
			captured = env
			return env, nil
		})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/echo", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)

		require.NotNil(t, captured)
		assert.NotEmpty(t, captured.TraceID)
		_, err := uuid.Parse(captured.TraceID)
		assert.NoError(t, err, "generated trace ID should be a valid UUID")
	})
}

func TestGateway_ServeHTTP_HandlerError(t *testing.T) {
	gw := NewGateway(body.GatewayConfig{})
	gw.RegisterHandler("fail", func(_ context.Context, _ *body.Envelope) (*body.Envelope, error) {
		return nil, fmt.Errorf("something went wrong")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/fail", strings.NewReader(`{}`))
	req.Header.Set("X-Trace-ID", "err-trace-1")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "something went wrong", errResp["error"])
	assert.Equal(t, "err-trace-1", errResp["trace_id"])
}

func TestGateway_ServeHTTP_PanicRecovery(t *testing.T) {
	gw := NewGateway(body.GatewayConfig{})
	gw.RegisterHandler("panic", func(_ context.Context, _ *body.Envelope) (*body.Envelope, error) {
		panic("boom!")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/panic", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		gw.ServeHTTP(rec, req)
	})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "internal server error", errResp["error"])
}

func TestGateway_StartStop(t *testing.T) {
	gw := NewGateway(body.GatewayConfig{
		Host:         "127.0.0.1",
		Port:         0,
		ReadTimeout:  5,
		WriteTimeout: 5,
	})

	ctx := context.Background()

	require.NoError(t, gw.Start(ctx))

	report := gw.Health()
	assert.Equal(t, "gateway", report.Head)
	assert.Equal(t, body.HealthHealthy, report.Status)

	require.NoError(t, gw.Stop(ctx))

	report = gw.Health()
	assert.Equal(t, body.HealthUnhealthy, report.Status)
}
