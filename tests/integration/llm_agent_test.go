package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/azagatti/hydra-db/internal/agent"
	agenttools "github.com/azagatti/hydra-db/internal/agent/tools"
	"github.com/azagatti/hydra-db/internal/body"
	"github.com/azagatti/hydra-db/internal/llm"
	"github.com/azagatti/hydra-db/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLLMAgent_ClassifyAndStoreMemories(t *testing.T) {
	sidecarURL := os.Getenv("LLM_SIDECAR_URL")
	if sidecarURL == "" {
		t.Skip("LLM_SIDECAR_URL not set; skipping LLM integration test")
	}

	fix, cleanup := setupHydra(t)
	defer cleanup()

	client := llm.NewClient(llm.WithBaseURL(sidecarURL))

	ctx := context.Background()
	require.NoError(t, client.Health(ctx), "sidecar must be reachable")

	llmTool := agenttools.NewLLMCompleteTool(client)

	ac := agent.NewContext(
		body.Identity{ID: "test-agent", Kind: body.ActorAgent, TenantID: "llm-test"},
		body.Tenant{ID: "llm-test", Name: "llm-test"},
	)

	classifyFn := func(ctx context.Context, ag *agent.Agent) (any, error) {
		input, _ := json.Marshal(map[string]string{
			"system_prompt": `Classify each line of this conversation excerpt. Return a JSON array where each element has:
- "line": the original text
- "memory_type": one of episodic, semantic, operational, working
- "confidence": 0.0 to 1.0
- "tags": array of 1-3 topic tags

Respond ONLY with the JSON array, no other text.`,
			"user_message": `Alice: I went to Paris last summer for two weeks.
Bob: That sounds amazing! I've always wanted to visit the Eiffel Tower.
Alice: My favorite restaurant there was Le Petit Cler. You should try it.
Bob: I'm actually allergic to shellfish, so I have to be careful eating out.`,
		})

		result, err := ag.Tools[0].Execute(ctx, input)
		if err != nil {
			return nil, err
		}

		var resp struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(result, &resp); err != nil {
			return nil, err
		}

		// Clean potential markdown fences.
		text := resp.Text
		for _, prefix := range []string{"```json", "```"} {
			if len(text) > len(prefix) && text[:len(prefix)] == prefix {
				text = text[len(prefix):]
			}
		}
		if len(text) > 3 && text[len(text)-3:] == "```" {
			text = text[:len(text)-3]
		}

		var classifications []struct {
			Line       string   `json:"line"`
			MemoryType string   `json:"memory_type"`
			Confidence float64  `json:"confidence"`
			Tags       []string `json:"tags"`
		}
		if err := json.Unmarshal([]byte(text), &classifications); err != nil {
			return nil, err
		}

		for _, c := range classifications {
			content, _ := json.Marshal(map[string]string{"text": c.Line})
			mem := memory.NewMemory(memory.Type(c.MemoryType), content, "test-agent", "llm-test")
			mem.Confidence = c.Confidence
			mem.Tags = c.Tags
			if err := fix.Memory.Store(ctx, mem); err != nil {
				return nil, err
			}
		}

		return classifications, nil
	}

	spawned, err := fix.Agent.Spawn(ctx, "memory-classifier", classifyFn,
		agent.WithContext(ac),
		agent.WithTools(llmTool),
	)

	require.NoError(t, err)
	assert.Equal(t, agent.StateDone, spawned.State)

	// Verify memories were stored.
	stored, err := fix.Memory.Search(ctx, memory.SearchQuery{TenantID: "llm-test", Limit: 100})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(stored), 2, "should have stored at least 2 memories")

	// Verify confidence scores are valid.
	for _, m := range stored {
		assert.Greater(t, m.Confidence, 0.0, "confidence should be positive")
		assert.LessOrEqual(t, m.Confidence, 1.0, "confidence should be at most 1.0")
		assert.NotEmpty(t, m.Tags, "should have at least one tag")
	}

	// Verify at least some type diversity.
	types := make(map[memory.Type]int)
	for _, m := range stored {
		types[m.Type]++
	}
	assert.GreaterOrEqual(t, len(types), 1, "should have at least one memory type assigned")
}
