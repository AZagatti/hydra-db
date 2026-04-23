package tdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/azagatti/hydra-db/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_Write_and_Read(t *testing.T) {
	// Mock TDB server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mem/write":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			json.NewEncoder(w).Encode(map[string]any{"cell_id": 42})
		case "/cells/42":
			json.NewEncoder(w).Encode(map[string]any{
				"cell_id": 42, "owner": "alice", "layer": 0,
				"importance": 85.0, "tier": "draft", "key_dim": 768, "value_dim": 768,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := NewProvider(srv.URL)
	mem := memory.NewMemory(memory.Episodic, json.RawMessage(`{"text":"hello"}`), "alice", "default")

	err := p.Write(context.Background(), mem)
	require.NoError(t, err)

	retrieved, err := p.Read(context.Background(), "42")
	require.NoError(t, err)
	assert.Equal(t, "42", retrieved.ID)
	assert.Equal(t, "alice", retrieved.ActorID)
}

func TestProvider_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mem/read" {
			json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{"cell_id": 1, "owner": "alice", "layer": 0, "score": 0.95, "tier": "draft", "importance": 80},
					{"cell_id": 2, "owner": "alice", "layer": 1, "score": 0.88, "tier": "validated", "importance": 65},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	p := NewProvider(srv.URL)
	results, err := p.Search(context.Background(), memory.SearchQuery{
		Type:    memory.Episodic,
		ActorID: "alice",
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "1", results[0].ID)
	assert.Equal(t, memory.Episodic, results[0].Type)
	assert.Equal(t, "2", results[1].ID)
}

func TestProvider_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	p := NewProvider(srv.URL)
	// Delete is a no-op (TDB has no cell-level delete API)
	err := p.Delete(context.Background(), "nonexistent")
	assert.NoError(t, err)
}