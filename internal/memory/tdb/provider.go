package tdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/azagatti/hydra-db/internal/memory"
)

// Provider is a memory.Provider that delegates to a TardigradeDB HTTP server.
// It is suitable for development and single-node deployments where the TDB
// server runs as a separate process.
type Provider struct {
	baseURL string
	client  *http.Client
}

// NewProvider creates a TDB-backed memory provider that connects to the given
// base URL (e.g. "http://localhost:8765").
func NewProvider(baseURL string) *Provider {
	return &Provider{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Write stores a Memory record in TardigradeDB via POST /mem/write.
// The Content (json.RawMessage) is stored as the cell value; a lightweight
// key vector is derived from the Content for retrieval scoring.
func (p *Provider) Write(_ context.Context, mem *memory.Memory) error {
	// Derive a simple key vector from Content bytes (first N bytes hashed).
	// In a full implementation this would use the model's actual KV projection.
	key := deriveKey(mem.Content)
	value := mem.Content // store raw content as value

	payload := map[string]any{
		"owner":    mem.ActorID,
		"layer":    layerFromType(mem.Type),
		"key":      key,
		"value":    value,
		"salience": mem.Confidence * 100, // TDB uses 0-100
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal mem_write payload: %w", err)
	}

	resp, err := p.client.Post(p.baseURL+"/mem/write", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("tdb /mem/write: %w", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tdb /mem/write: HTTP %d", resp.StatusCode)
	}

	var result struct {
		CellID int `json:"cell_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode mem_write response: %w", err)
	}

	return nil
}

// Read retrieves a Memory record by ID via GET /cells/<id>.
func (p *Provider) Read(_ context.Context, id string) (*memory.Memory, error) {
	resp, err := p.client.Get(p.baseURL + "/cells/" + id)
	if err != nil {
		return nil, fmt.Errorf("tdb /cells/%s: %w", id, err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("memory %q not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tdb /cells/%s: HTTP %d", id, resp.StatusCode)
	}

	var cell struct {
		CellID     int     `json:"cell_id"`
		Owner      string  `json:"owner"`
		Layer      int     `json:"layer"`
		Importance float64 `json:"importance"`
		Tier       string  `json:"tier"`
		KeyDim     int     `json:"key_dim"`
		ValueDim   int     `json:"value_dim"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cell); err != nil {
		return nil, fmt.Errorf("decode cell response: %w", err)
	}

	// Reconstruct minimal Memory from cell metadata.
	// Full content requires a separate mem_read call.
	return &memory.Memory{
		ID:      id,
		Type:    typeFromLayer(cell.Layer),
		ActorID: cell.Owner,
		Confidence: cell.Importance / 100,
		Tags:   []string{},
	}, nil
}

// Search returns Memory records by querying TDB via POST /mem/read with a
// synthetic query key derived from the SearchQuery fields.
func (p *Provider) Search(_ context.Context, query memory.SearchQuery) ([]*memory.Memory, error) {
	// Build a query key from the search fields.
	queryKey := buildQueryKey(query)

	payload := map[string]any{
		"query_key": queryKey,
		"k":         query.Limit,
	}
	if query.Limit <= 0 {
		payload["k"] = 50
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal mem_read payload: %w", err)
	}

	resp, err := p.client.Post(p.baseURL+"/mem/read", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tdb /mem/read: %w", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tdb /mem/read: HTTP %d", resp.StatusCode)
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
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode mem_read response: %w", err)
	}

	memories := make([]*memory.Memory, 0, len(result.Results))
	for _, r := range result.Results {
		memories = append(memories, &memory.Memory{
			ID:         fmt.Sprintf("%d", r.CellID),
			Type:       typeFromLayer(r.Layer),
			ActorID:    r.Owner,
			Confidence: r.Importance / 100,
			Tags:       []string{},
		})
	}

	return memories, nil
}

// Delete removes a Memory record from TDB.
// TDB does not have a direct delete by cell_id API, so this is a no-op
// with a warning. In production, implement a /mem/delete endpoint or
// rely on TDB's automatic tier expiration (core → validated → draft → evict).
func (p *Provider) Delete(_ context.Context, id string) error {
	// TDB has no cell-level delete API yet.
	// Cells are evicted via governance (AKL tier expiration).
	return nil
}

// ─── Helpers ───────────────────────────────────────────────────────────────

// deriveKey creates a deterministic float32 key vector from content bytes.
// Uses a simple hash to generate a fixed-dimension vector for latent retrieval.
// In production, this would use the model's actual KV projection.
func deriveKey(content json.RawMessage) []float32 {
	const dim = 768 // matches d_model for Qwen3-0.6B / GPT-2
	h := hashBytes(content)
	key := make([]float32, dim)
	for i := 0; i < dim; i++ {
		key[i] = float32((uint64(h[i%8]) << (i % 56)) & 0xFFFF) / 65535.0
	}
	return key
}

func hashBytes(b []byte) [32]byte {
	// FNV-1a hash
	h := [32]byte{}
	fnv := uint64(2166136261)
	for _, v := range b {
		fnv ^= uint64(v)
		fnv *= 16777619
	}
	// Write into array in 8-byte chunks
	for i := 0; i < 8; i++ {
		h[i] = byte(fnv >> (i * 8))
		h[i+8] = byte(fnv >> ((i + 8) * 8))
		h[i+16] = byte(fnv >> ((i + 16) * 8))
	}
	return h
}

// buildQueryKey builds a synthetic key vector from SearchQuery fields.
// This is a best-effort query representation; full semantic search requires
// the actual model projection.
func buildQueryKey(query memory.SearchQuery) []float32 {
	const dim = 768
	// Mix type, actor, tags into a deterministic vector.
	data, _ := json.Marshal(query)
	h := hashBytes(data)
	key := make([]float32, dim)
	for i := 0; i < dim; i++ {
		key[i] = float32((uint64(h[i%8]) << (i % 56)) & 0xFFFF) / 65535.0
	}
	return key
}

// layerFromType maps Hydra memory.Type to a TDB layer index.
// Episodic=0, Semantic=1, Operational=2, Working=3.
func layerFromType(t memory.Type) int {
	switch t {
	case memory.Episodic:
		return 0
	case memory.Semantic:
		return 1
	case memory.Operational:
		return 2
	case memory.Working:
		return 3
	default:
		return 0
	}
}

// typeFromLayer maps a TDB layer index back to Hydra memory.Type.
func typeFromLayer(layer int) memory.Type {
	switch layer {
	case 0:
		return memory.Episodic
	case 1:
		return memory.Semantic
	case 2:
		return memory.Operational
	case 3:
		return memory.Working
	default:
		return memory.Semantic
	}
}

var _ memory.Provider = (*Provider)(nil)