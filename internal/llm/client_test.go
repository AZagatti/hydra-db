package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComplete_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/complete", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req CompleteRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "You are helpful.", req.SystemPrompt)
		assert.Equal(t, "Hello", req.UserMessage)

		json.NewEncoder(w).Encode(CompleteResponse{
			Text:  "Hi there!",
			Usage: Usage{InputTokens: 10, OutputTokens: 5},
		})
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))
	resp, err := client.Complete(t.Context(), CompleteRequest{
		SystemPrompt: "You are helpful.",
		UserMessage:  "Hello",
	})

	require.NoError(t, err)
	assert.Equal(t, "Hi there!", resp.Text)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
}

func TestComplete_SidecarError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		//nolint:errcheck
		json.NewEncoder(w).Encode(map[string]string{"error": "model overloaded"})
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))
	_, err := client.Complete(t.Context(), CompleteRequest{
		SystemPrompt: "test",
		UserMessage:  "test",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "model overloaded")
}

func TestComplete_ResponseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(CompleteResponse{
			Error: "rate limited",
		})
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))
	_, err := client.Complete(t.Context(), CompleteRequest{
		SystemPrompt: "test",
		UserMessage:  "test",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
}

func TestComplete_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately

	client := NewClient(WithBaseURL("http://localhost:1"), WithTimeout(100*time.Millisecond))

	_, err := client.Complete(ctx, CompleteRequest{
		SystemPrompt: "test",
		UserMessage:  "test",
	})

	require.Error(t, err)
}

func TestHealth_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))
	err := client.Health(t.Context())

	require.NoError(t, err)
}

func TestHealth_Unhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))
	err := client.Health(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unhealthy")
}

func TestHealth_Unreachable(t *testing.T) {
	client := NewClient(WithBaseURL("http://localhost:1"))
	err := client.Health(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unreachable")
}

func TestNewClient_Defaults(t *testing.T) {
	client := NewClient()

	assert.Equal(t, defaultBaseURL, client.baseURL)
	assert.Equal(t, defaultTimeout, client.httpClient.Timeout)
}

func TestNewClient_WithOptions(t *testing.T) {
	client := NewClient(
		WithBaseURL("http://custom:9999"),
		WithTimeout(5*time.Second),
	)

	assert.Equal(t, "http://custom:9999", client.baseURL)
	assert.Equal(t, 5*time.Second, client.httpClient.Timeout)
}
