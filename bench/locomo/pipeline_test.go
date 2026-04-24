package locomo

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/azagatti/hydra-db/internal/memory"
	"github.com/azagatti/hydra-db/internal/memory/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDataset_AssignsSampleIDs(t *testing.T) {
	raw := `[
		{
			"conversation": {
				"speaker_a": "Alice",
				"speaker_b": "Bob",
				"session_1": [
					{"speaker": "Alice", "dia_id": "D1:1", "text": "Hello"}
				],
				"session_1_date_time": "2023-01-05"
			},
			"qa": [
				{
					"question": "What did Alice say first?",
					"answer": "Hello",
					"evidence": ["D1:1"],
					"category": 1
				}
			]
		}
	]`

	path := t.TempDir() + "/locomo.json"
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	dataset, err := LoadDataset(path)

	require.NoError(t, err)
	require.Len(t, dataset, 1)
	assert.Equal(t, "sample_0", dataset[0].SampleID)
	assert.Equal(t, "Alice", dataset[0].SpeakerA)
	assert.Equal(t, "Bob", dataset[0].SpeakerB)
}

func TestIngest_StoresTurnsWithMetadata(t *testing.T) {
	sample := seededSample()

	ingested, err := Ingest(t.Context(), sample)

	require.NoError(t, err)
	assert.Equal(t, 2, ingested.TurnCount)

	memories, err := ingested.Plane.Search(t.Context(), memory.SearchQuery{
		Type:     memory.Episodic,
		TenantID: sample.SampleID,
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, memories, 2)

	assert.ElementsMatch(t, []string{"D1:1", "D1:2"}, extractDiaIDs(memories, ingested.MemToDiaID))
	for _, mem := range memories {
		assert.Equal(t, []string{"session-1"}, mem.Tags)
		assert.NotEmpty(t, mem.Metadata["dia_id"])
		assert.Equal(t, "1", mem.Metadata["session"])
		assert.False(t, mem.CreatedAt.IsZero())
	}
}

func TestExecuteQueries_UsesMetadataThenFallbacks(t *testing.T) {
	plane := memory.NewPlane(inmemory.NewProvider())
	sample := &IngestedSample{
		SampleID:   "sample-1",
		Plane:      plane,
		MemToDiaID: make(map[string]string),
	}

	storeTestMemory(t, plane, sample.SampleID, memory.Episodic, []byte(`{"dia_id":"ignored"}`), 1.0,
		map[string]string{"dia_id": "D1:1"}, nil)
	mem := storeTestMemory(t, plane, sample.SampleID, memory.Episodic, []byte(`{"dia_id":"also-ignored"}`), 1.0,
		nil, nil)
	sample.MemToDiaID[mem.ID] = "D1:2"
	storeTestMemory(t, plane, sample.SampleID, memory.Episodic, []byte(`{"dia_id":"D1:3"}`), 1.0,
		nil, nil)

	results, err := ExecuteQueries(t.Context(), sample, []QAItem{{Question: "q1"}})

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.ElementsMatch(t, []string{"D1:1", "D1:2", "D1:3"}, results[0].RetrievedIDs)
}

func TestReportTableAndJSON_IncludeLLMDetails(t *testing.T) {
	result := BenchResult{
		Provider:      "llm",
		Samples:       1,
		Questions:     2,
		Categories:    []CategoryScore{{Category: "single_hop", Count: 2, Precision: 0.5, Recall: 1.0, F1: 0.667}},
		Overall:       CategoryScore{Category: "OVERALL", Count: 2, Precision: 0.5, Recall: 1.0, F1: 0.667},
		Tokens:        &TokenUsage{InputTokens: 10, OutputTokens: 4},
		PlanFallbacks: 2,
	}

	var table bytes.Buffer
	ReportTable(&table, result)
	assert.Contains(t, table.String(), "Token Usage: 10 input, 4 output (14 total)")
	assert.Contains(t, table.String(), "Query planner fallbacks: 2")

	var jsonOut bytes.Buffer
	require.NoError(t, ReportJSON(&jsonOut, result))
	assert.Contains(t, jsonOut.String(), `"planFallbacks": 2`)
}

func seededSample() Sample {
	return Sample{
		SampleID: "sample-1",
		SpeakerA: "Alice",
		SpeakerB: "Bob",
		Sessions: []Session{
			{
				Index:    1,
				DateTime: "5 January 2023",
				Turns: []Turn{
					{Speaker: "Alice", DiaID: "D1:1", Text: "I went to Paris last summer."},
					{Speaker: "Bob", DiaID: "D1:2", Text: "That sounds amazing."},
				},
			},
		},
		QA: []QAItem{
			{
				Question: "Where did Alice travel?",
				Answer:   json.RawMessage(`"Paris"`),
				Evidence: []string{"D1:1"},
				Category: CategorySingleHop,
			},
		},
	}
}

func storeTestMemory(
	t *testing.T,
	plane *memory.Plane,
	tenantID string,
	memType memory.Type,
	content []byte,
	confidence float64,
	metadata map[string]string,
	tags []string,
) *memory.Memory {
	t.Helper()

	mem := memory.NewMemory(memType, content, "tester", tenantID)
	mem.Confidence = confidence
	mem.Metadata = metadata
	mem.Tags = tags
	require.NoError(t, plane.Store(context.Background(), mem))
	return mem
}
