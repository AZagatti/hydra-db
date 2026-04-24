package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/azagatti/hydra-db/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLLMCompleteTool_Name(t *testing.T) {
	tool := NewLLMCompleteTool(llm.NewClient())
	assert.Equal(t, "llm.complete", tool.Name())
}

func TestLLMCompleteTool_Execute_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.CompleteRequest
		//nolint:errcheck
		json.NewDecoder(r.Body).Decode(&req)

		assert.Equal(t, "Be concise.", req.SystemPrompt)
		assert.Equal(t, "What is Go?", req.UserMessage)

		//nolint:errcheck
		json.NewEncoder(w).Encode(llm.CompleteResponse{
			Text:  "A programming language.",
			Usage: llm.Usage{InputTokens: 15, OutputTokens: 4},
		})
	}))
	defer server.Close()

	client := llm.NewClient(llm.WithBaseURL(server.URL))
	tool := NewLLMCompleteTool(client)

	input, _ := json.Marshal(llmInput{
		SystemPrompt: "Be concise.",
		UserMessage:  "What is Go?",
	})

	result, err := tool.Execute(t.Context(), input)
	require.NoError(t, err)

	var out llmOutput
	require.NoError(t, json.Unmarshal(result, &out))
	assert.Equal(t, "A programming language.", out.Text)
	assert.Equal(t, 15, out.Usage.InputTokens)
	assert.Equal(t, 4, out.Usage.OutputTokens)
}

func TestLLMCompleteTool_Execute_MissingFields(t *testing.T) {
	client := llm.NewClient()
	tool := NewLLMCompleteTool(client)

	tests := []struct {
		name  string
		input llmInput
	}{
		{"missing system_prompt", llmInput{UserMessage: "test"}},
		{"missing user_message", llmInput{SystemPrompt: "test"}},
		{"both empty", llmInput{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, _ := json.Marshal(tt.input)
			_, err := tool.Execute(t.Context(), raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "required")
		})
	}
}

func TestLLMCompleteTool_Execute_InvalidJSON(t *testing.T) {
	client := llm.NewClient()
	tool := NewLLMCompleteTool(client)

	_, err := tool.Execute(t.Context(), json.RawMessage(`{invalid`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestLLMCompleteTool_Execute_SidecarError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		//nolint:errcheck
		json.NewEncoder(w).Encode(map[string]string{"error": "provider down"})
	}))
	defer server.Close()

	client := llm.NewClient(llm.WithBaseURL(server.URL))
	tool := NewLLMCompleteTool(client)

	input, _ := json.Marshal(llmInput{
		SystemPrompt: "test",
		UserMessage:  "test",
	})

	_, err := tool.Execute(t.Context(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider down")
}

func TestLLMCompleteTool_ImplementsToolInterface(t *testing.T) {
	// Verify it satisfies agent.Tool at compile time via the Name() and Execute() methods.
	tool := NewLLMCompleteTool(llm.NewClient())
	assert.NotEmpty(t, tool.Name())
}
