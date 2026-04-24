package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/azagatti/hydra-db/bench/locomo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectStrategy_Basic(t *testing.T) {
	strat, err := selectStrategy(t.Context(), "basic", "")

	require.NoError(t, err)
	assert.Equal(t, "basic", strat.Name())
}

func TestSelectStrategy_LLM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	strat, err := selectStrategy(t.Context(), "llm", server.URL)

	require.NoError(t, err)
	assert.Equal(t, "llm", strat.Name())
}

func TestSelectStrategy_Invalid(t *testing.T) {
	_, err := selectStrategy(t.Context(), "unknown", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown strategy")
}

func TestRunBenchmark_BasicStrategy(t *testing.T) {
	result, err := runBenchmark(context.Background(), locomo.Dataset{{
		SampleID: "sample-1",
		SpeakerA: "Alice",
		SpeakerB: "Bob",
		Sessions: []locomo.Session{{
			Index:    1,
			DateTime: "2023-01-05",
			Turns: []locomo.Turn{
				{Speaker: "Alice", DiaID: "D1:1", Text: "I went to Paris."},
				{Speaker: "Bob", DiaID: "D1:2", Text: "Nice trip."},
			},
		}},
		QA: []locomo.QAItem{{
			Question: "Where did Alice travel?",
			Answer:   []byte(`"Paris"`),
			Evidence: []string{"D1:1"},
			Category: locomo.CategorySingleHop,
		}},
	}}, &locomo.BasicStrategy{})

	require.NoError(t, err)
	assert.Equal(t, "basic", result.Provider)
	assert.Equal(t, 1, result.Samples)
	assert.Equal(t, 1, result.Questions)
	assert.Nil(t, result.Tokens)
	assert.Equal(t, 0, result.PlanFallbacks)
}
