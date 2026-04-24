package locomo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/azagatti/hydra-db/internal/llm"
	"github.com/azagatti/hydra-db/internal/memory"
	"github.com/azagatti/hydra-db/internal/memory/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLLMStrategy_Ingest_FallsBackToBasicClassification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "provider down"})
	}))
	defer server.Close()

	strat := NewLLMStrategy(llm.NewClient(llm.WithBaseURL(server.URL)))

	ingested, err := strat.Ingest(t.Context(), seededSample())

	require.NoError(t, err)
	assert.Equal(t, 2, ingested.TurnCount)
	assert.Equal(t, 0, strat.TotalUsage.InputTokens)
	assert.Equal(t, 0, strat.TotalUsage.OutputTokens)

	memories, err := ingested.Plane.Search(t.Context(), memory.SearchQuery{
		Type:     memory.Episodic,
		TenantID: ingested.SampleID,
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, memories, 2)
	for _, mem := range memories {
		assert.Equal(t, memory.Episodic, mem.Type)
		assert.Equal(t, 0.5, mem.Confidence)
		assert.NotEmpty(t, mem.Metadata["summary"])
	}
}

func TestLLMStrategy_Query_UsesPlannedFiltersAndTracksUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.CompleteRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, querySystemPrompt, req.SystemPrompt)
		_ = json.NewEncoder(w).Encode(llm.CompleteResponse{
			Text: "```json\n{\"memory_type\":\"semantic\",\"tags\":[\"travel\"],\"min_confidence\":0.7,\"reasoning\":\"travel memory\"}\n```",
			Usage: llm.Usage{
				InputTokens:  11,
				OutputTokens: 7,
			},
		})
	}))
	defer server.Close()

	strat := NewLLMStrategy(llm.NewClient(llm.WithBaseURL(server.URL)))
	plane := memory.NewPlane(inmemory.NewProvider())
	ingested := &IngestedSample{
		SampleID:   "sample-1",
		Plane:      plane,
		MemToDiaID: make(map[string]string),
	}

	storeTestMemory(t, plane, ingested.SampleID, memory.Semantic, []byte(`{"dia_id":"D1:1"}`), 0.9,
		map[string]string{"dia_id": "D1:1"}, []string{"travel"})
	storeTestMemory(t, plane, ingested.SampleID, memory.Semantic, []byte(`{"dia_id":"D1:2"}`), 0.6,
		map[string]string{"dia_id": "D1:2"}, []string{"travel"})
	storeTestMemory(t, plane, ingested.SampleID, memory.Episodic, []byte(`{"dia_id":"D1:3"}`), 0.95,
		map[string]string{"dia_id": "D1:3"}, []string{"travel"})
	storeTestMemory(t, plane, ingested.SampleID, memory.Semantic, []byte(`{"dia_id":"D1:4"}`), 0.95,
		map[string]string{"dia_id": "D1:4"}, []string{"food"})

	results, err := strat.Query(t.Context(), ingested, []QAItem{{
		Question: "Where did Alice travel?",
		Category: CategorySingleHop,
	}})

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, []string{"D1:1"}, results[0].RetrievedIDs)
	assert.Equal(t, 11, strat.TotalUsage.InputTokens)
	assert.Equal(t, 7, strat.TotalUsage.OutputTokens)
}

func TestLLMStrategy_Query_FallsBackToBroadSearchAndCountsFallbacks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "planner unavailable"})
	}))
	defer server.Close()

	ingested, err := Ingest(t.Context(), seededSample())
	require.NoError(t, err)

	strat := NewLLMStrategy(llm.NewClient(llm.WithBaseURL(server.URL)))
	results, err := strat.Query(t.Context(), ingested, []QAItem{{
		Question: "Where did Alice travel?",
		Category: CategorySingleHop,
	}})

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.NotEmpty(t, results[0].RetrievedIDs)
	assert.Equal(t, 1, strat.PlanFallbacks)
}
