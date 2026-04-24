package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPRequestInput describes an outbound HTTP request.
type HTTPRequestInput struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    any               `json:"body,omitempty"`
	Timeout int               `json:"timeout_seconds,omitempty"`
}

// HTTPRequestOutput holds the response from an HTTP request.
type HTTPRequestOutput struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers"`
	Body       json.RawMessage     `json:"body"`
}

// HTTPRequest is a tool that performs outbound HTTP requests.
// It supports GET, POST, PUT, DELETE with optional JSON body and headers.
type HTTPRequest struct {
	client *http.Client
}

// NewHTTPRequest creates an HTTPRequest tool with a default 30s timeout.
func NewHTTPRequest() *HTTPRequest {
	return &HTTPRequest{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name returns "http_request".
func (t *HTTPRequest) Name() string { return "http_request" }

// Execute performs the HTTP request described by raw input.
func (t *HTTPRequest) Execute(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var input HTTPRequestInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("http_request: unmarshal input: %w", err)
	}

	if input.URL == "" {
		return nil, fmt.Errorf("http_request: url is required")
	}

	method := input.Method
	if method == "" {
		method = "GET"
	}

	var body io.Reader
	if input.Body != nil {
		payload, err := json.Marshal(input.Body)
		if err != nil {
			return nil, fmt.Errorf("http_request: marshal body: %w", err)
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, input.URL, body)
	if err != nil {
		return nil, fmt.Errorf("http_request: new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range input.Headers {
		req.Header.Set(k, v)
	}

	timeout := time.Duration(input.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http_request: do request: %w", err)
	}
	//nolint:errcheck
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http_request: read body: %w", err)
	}

	output := HTTPRequestOutput{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}

	return json.Marshal(output)
}
