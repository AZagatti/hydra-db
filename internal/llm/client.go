package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultBaseURL    = "http://localhost:3100"
	defaultTimeout    = 120 * time.Second
	defaultMaxTokens  = 1024
	defaultTemperature = 0.7
)

// CompleteRequest is sent to the sidecar's /complete endpoint.
type CompleteRequest struct {
	SystemPrompt string   `json:"systemPrompt"`
	UserMessage  string   `json:"userMessage"`
	MaxTokens    int      `json:"maxTokens,omitempty"`
	Temperature  *float64 `json:"temperature,omitempty"`
	Model        string   `json:"model,omitempty"`
}

// CompleteResponse is returned by the sidecar's /complete endpoint.
type CompleteResponse struct {
	Text  string `json:"text"`
	Usage Usage  `json:"usage"`
	Error string `json:"error,omitempty"`
}

// Usage tracks token consumption for cost monitoring.
type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

// Client calls the LLM sidecar HTTP service.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	model       string
	maxTokens   int
	temperature float64
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the sidecar URL (default http://localhost:3100).
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithTimeout sets the HTTP client timeout (default 120s).
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// WithModel sets the default model for all requests.
func WithModel(model string) Option {
	return func(c *Client) {
		c.model = model
	}
}

// WithMaxTokens sets the default max tokens for all requests.
func WithMaxTokens(maxTokens int) Option {
	return func(c *Client) {
		c.maxTokens = maxTokens
	}
}

// WithTemperature sets the default temperature for all requests.
func WithTemperature(temp float64) Option {
	return func(c *Client) {
		c.temperature = temp
	}
}

// NewClient creates a configured LLM client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:     defaultBaseURL,
		httpClient:  &http.Client{Timeout: defaultTimeout},
		maxTokens:   defaultMaxTokens,
		temperature: defaultTemperature,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Complete sends a prompt to the LLM sidecar and returns the response.
// Default values for model, maxTokens, and temperature are taken from the Client
// if not explicitly set in the request.
func (c *Client) Complete(ctx context.Context, req CompleteRequest) (*CompleteResponse, error) {
	// Apply client defaults for fields not set in the request.
	if req.MaxTokens == 0 {
		req.MaxTokens = c.maxTokens
	}
	if req.Model == "" {
		req.Model = c.model
	}
	if req.Temperature == nil && c.temperature > 0 {
		temp := c.temperature
		req.Temperature = &temp
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/complete", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sidecar request: %w", err)
	}
	//nolint:errcheck
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("sidecar error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("sidecar HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var result CompleteResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("sidecar error: %s", result.Error)
	}

	return &result, nil
}

// Health checks if the sidecar is reachable and responding on /health.
func (c *Client) Health(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("create health request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sidecar unreachable: %w", err)
	}
	//nolint:errcheck
	defer resp.Body.Close()

	// Drain body to allow connection reuse.
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sidecar unhealthy: HTTP %d", resp.StatusCode)
	}

	return nil
}
