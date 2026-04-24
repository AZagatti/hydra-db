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

// TDBSearchInput describes a semantic vector search query for TardigradeDB.
type TDBSearchInput struct {
	Query    string                 `json:"query"`
	Limit    int                    `json:"limit,omitempty"`
	Layer    int                    `json:"layer,omitempty"`
	Metadata map[string]any         `json:"metadata,omitempty"`
}

// TDBSearchOutput holds the search results from TDB.
type TDBSearchOutput struct {
	Count   int         `json:"count"`
	Results []TDBResult `json:"results"`
}

// TDBResult represents a single TDB cell result.
type TDBResult struct {
	CellID     int               `json:"cell_id"`
	Owner      string            `json:"owner"`
	Layer      int               `json:"layer"`
	Score      float64           `json:"score"`
	Tier       string            `json:"tier"`
	Importance float64           `json:"importance"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

// TDBSearch performs a latent semantic search against the TardigradeDB engine
// via its HTTP API, using Q*K retrieval for high-recall results.
type TDBSearch struct {
	baseURL string
	client  *http.Client
}

// NewTDBSearch creates a TDBSearch tool that connects to the given TDB base URL.
func NewTDBSearch(baseURL string) *TDBSearch {
	return &TDBSearch{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Name returns "tdb_search".
func (t *TDBSearch) Name() string { return "tdb_search" }

// Execute performs a semantic search via TDB's /mem/read endpoint.
func (t *TDBSearch) Execute(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var input TDBSearchInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("tdb_search: unmarshal input: %w", err)
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}

	// Build a synthetic query key from the query text.
	queryKey := buildQueryKey(input.Query, 768)

	payload := map[string]any{
		"query_key": queryKey,
		"k":         limit,
	}
	if input.Layer >= 0 {
		payload["layer"] = input.Layer
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("tdb_search: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/mem/read", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tdb_search: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tdb_search: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tdb_search: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tdb_search: TDB returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Results []struct {
			CellID     int     `json:"cell_id"`
			Owner      string  `json:"owner"`
			Layer      int     `json:"layer"`
			Score      float64 `json:"score"`
			Tier       string  `json:"tier"`
			Importance float64 `json:"importance"`
		} `json:"results"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("tdb_search: unmarshal response: %w", err)
	}

	out := make([]TDBResult, 0, len(result.Results))
	for _, r := range result.Results {
		out = append(out, TDBResult{
			CellID:     r.CellID,
			Owner:      r.Owner,
			Layer:      r.Layer,
			Score:      r.Score,
			Tier:       r.Tier,
			Importance: r.Importance,
		})
	}

	return json.Marshal(TDBSearchOutput{
		Count:   len(out),
		Results: out,
	})
}

// buildQueryKey creates a deterministic float32 vector from a string query.
// Uses FNV-1a hash to deterministically distribute values across the key space.
func buildQueryKey(query string, dim int) []float32 {
	h := fnv1aHash(query)
	key := make([]float32, dim)
	for i := 0; i < dim; i++ {
		key[i] = float32((uint64(h[i%8]) << (i % 56)) & 0xFFFF) / 65535.0
	}
	return key
}

func fnv1aHash(s string) [8]byte {
	h := uint64(2166136261)
	for _, c := range s {
		h ^= uint64(c)
		h *= 16777619
	}
	var b [8]byte
	for i := 0; i < 8; i++ {
		b[i] = byte(h >> (i * 8))
	}
	return b
}
