package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/azagatti/hydra-db/internal/agent/tools"
	"github.com/azagatti/hydra-db/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolUnderTest is a test spy that records invocations and returns canned results.
type toolUnderTest struct {
	name   string
	result json.RawMessage
	err    error
	calls  int
	mu     sync.Mutex
}

func (t *toolUnderTest) Name() string { return t.name }

func (t *toolUnderTest) Execute(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	t.mu.Lock()
	t.calls++
	t.mu.Unlock()
	return t.result, t.err
}

func (t *toolUnderTest) Calls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls
}

// newMockLLMServer returns a test server that cycles through responses.
// Each response is a JSON string literal (as written in Go source: `"..."`).
// The server decodes it, then encodes it as {"text": "decoded value", "usage":{...}}.
func newMockLLMServer(responses ...string) *httptest.Server {
	index := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/complete" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if index >= len(responses) {
			http.Error(w, `{"error":"no more responses"}`, http.StatusInternalServerError)
			return
		}

		var textVal string
		// responses[i] is a Go string literal like `"42"` — parse as JSON to get the actual value.
		if err := json.Unmarshal([]byte(responses[index]), &textVal); err != nil {
			http.Error(w, `{"error":"bad response"}`, http.StatusInternalServerError)
			return
		}
		index++

		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{
			"text":  textVal,
			"usage": map[string]int{"inputTokens": 10, "outputTokens": 5},
		}
		//nolint:errcheck
		json.NewEncoder(w).Encode(body)
	}))
}

// --- Tests ---

func TestNewToolLoop(t *testing.T) {
	rt := NewRuntime()
	tl := NewToolLoop(rt, ToolLoopConfig{})
	assert.NotNil(t, tl)
	assert.Equal(t, DefaultToolLoopPrompt, tl.config.SystemPrompt)
}

func TestNewToolLoop_CustomSystemPrompt(t *testing.T) {
	rt := NewRuntime()
	customPrompt := "You are a math assistant."
	tl := NewToolLoop(rt, ToolLoopConfig{SystemPrompt: customPrompt})
	assert.Equal(t, customPrompt, tl.config.SystemPrompt)
}

func TestNewToolLoop_DefaultAliases(t *testing.T) {
	rt := NewRuntime()
	tl := NewToolLoop(rt, ToolLoopConfig{})
	assert.NotNil(t, tl.config.ToolAliases)
	assert.Len(t, tl.config.ToolAliases, 0)
}

func TestToolLoop_resolveTool(t *testing.T) {
	rt := NewRuntime()
	tool := &toolUnderTest{name: "memory_search", result: []byte(`{"ok":true}`)}
	err := rt.RegisterTool(tool)
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{
		ToolAliases: map[string]string{
			"search": "memory_search",
			"web":    "http_request",
		},
	})

	assert.Equal(t, "memory_search", tl.resolveTool("search"))
	assert.Equal(t, "memory_search", tl.resolveTool("memory_search"))
	assert.Equal(t, "", tl.resolveTool("nonexistent"))
}

func TestToolLoop_isToolCall(t *testing.T) {
	rt := NewRuntime()
	tl := NewToolLoop(rt, ToolLoopConfig{})

	tests := []struct {
		name     string
		response string
		want     bool
	}{
		{"valid tool call", `{"tool":"memory_search","input":{"type":"semantic"}}`, true},
		{"tool with nested input", `{"tool":"http_request","input":{"url":"https://example.com"}}`, true},
		{"plain text answer", `I think the answer is 42.`, false},
		{"markdown wrapped", "```json\n{\"tool\":\"memory_search\",\"input\":{}}\n```", true},
		{"empty object", `{}`, false},
		{"just tool name no input", `{"tool":"llm.complete"}`, true},
		{"array response", `[1,2,3]`, false},
		{"null", ``, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tl.isToolCall(tt.response)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToolLoop_parseToolCall(t *testing.T) {
	rt := NewRuntime()
	tl := NewToolLoop(rt, ToolLoopConfig{})

	tests := []struct {
		name      string
		response  string
		wantTool  string
		wantInput string
		wantErr   bool
	}{
		{
			name:      "basic tool call",
			response:  `{"tool":"memory_search","input":{"type":"semantic"}}`,
			wantTool:  "memory_search",
			wantInput: `{"type":"semantic"}`,
		},
		{
			name:      "tool call with truncated marker",
			response:  "```json\n{\"tool\":\"http_request\",\"input\":{\"url\":\"https://example.com\"}}\n```",
			wantTool:  "http_request",
			wantInput: `{"url":"https://example.com"}`,
		},
		{
			name:     "empty tool field",
			response: `{"tool":"","input":{}}`,
			wantErr:  true,
		},
		{
			name:     "invalid JSON",
			response: `not json`,
			wantErr:  true,
		},
		{
			name:     "missing tool field",
			response: `{"input":{}}`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, input, err := tl.parseToolCall(tt.response)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTool, tool)
			assert.JSONEq(t, tt.wantInput, string(input))
		})
	}
}

func TestToolLoop_buildPrompt(t *testing.T) {
	rt := NewRuntime()
	tl := NewToolLoop(rt, ToolLoopConfig{
		SystemPrompt: "You are helpful.",
	})

	prompt := tl.buildPrompt("Hello", nil)
	assert.Contains(t, prompt, "SYSTEM:")
	assert.Contains(t, prompt, "You are helpful.")
	assert.Contains(t, prompt, "USER: Hello")

	prompt2 := tl.buildPrompt("Hi", []string{
		"[tool=memory_search] RESULT: ok",
		"The user wants to search.",
	})
	assert.Contains(t, prompt2, "CONVERSATION HISTORY:")
	assert.Contains(t, prompt2, "[tool=memory_search] RESULT: ok")
	assert.Contains(t, prompt2, "USER: Hi")
}

func TestToolLoop_extractUserMessage(t *testing.T) {
	rt := NewRuntime()
	tl := NewToolLoop(rt, ToolLoopConfig{})

	// No context.
	agent := &Agent{}
	assert.Equal(t, "", tl.extractUserMessage(agent))

	// Context with user_message.
	agent = &Agent{Context: &Context{Memory: map[string]any{"user_message": "What is the weather?"}}}
	assert.Equal(t, "What is the weather?", tl.extractUserMessage(agent))

	// Context with wrong type.
	agent = &Agent{Context: &Context{Memory: map[string]any{"user_message": 123}}}
	assert.Equal(t, "", tl.extractUserMessage(agent))
}

// --- Integration tests with mock LLM server ---

func TestToolLoop_Do_NoToolCalls(t *testing.T) {
	server := newMockLLMServer(`"The capital of Brazil is Brasilia."`)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{
		SystemPrompt:  "You are a geography assistant.",
		MaxIterations: 10,
		ToolAliases:   map[string]string{},
	})

	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "What is the capital of Brazil?"}}}

	result, err := tl.Do(context.Background(), agent)
	require.NoError(t, err)

	m := result.(map[string]any)
	assert.Equal(t, "final", m["type"])
	assert.Equal(t, "The capital of Brazil is Brasilia.", m["answer"])
	assert.Equal(t, 0, m["iterations"])
}

func TestToolLoop_Do_SingleToolCall(t *testing.T) {
	server := newMockLLMServer(
		`{"tool":"memory_search","input":{"type":"semantic","limit":5}}`,
		`"Based on memory, the user prefers dark mode."`,
	)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	memTool := &toolUnderTest{name: "memory_search", result: json.RawMessage(`{"count":1,"results":[{"id":"mem-123"}]}`)}
	err = rt.RegisterTool(memTool)
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{
		SystemPrompt:  "You are a helpful assistant with memory.",
		MaxIterations:  20,
		ToolAliases:   map[string]string{},
	})

	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "What are my preferences?"}}}

	result, err := tl.Do(context.Background(), agent)
	require.NoError(t, err)

	m := result.(map[string]any)
	assert.Equal(t, "final", m["type"])
	assert.Equal(t, 2, m["iterations"])
	assert.Equal(t, 1, memTool.Calls())
	assert.Contains(t, m["answer"], "dark mode")
}

func TestToolLoop_Do_MaxIterations(t *testing.T) {
	server := newMockLLMServer(
		`{"tool":"memory_search","input":{"type":"episodic"}}`,
		`{"tool":"memory_search","input":{"type":"episodic"}}`,
		`{"tool":"memory_search","input":{"type":"episodic"}}`,
	)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	memTool := &toolUnderTest{name: "memory_search", result: []byte(`{"count":0,"results":[]}`)}
	err = rt.RegisterTool(memTool)
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{
		SystemPrompt:  "You are a helpful assistant.",
		MaxIterations: 3,
	})

	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "search"}}}

	result, err := tl.Do(context.Background(), agent)
	require.NoError(t, err)

	m := result.(map[string]any)
	assert.Equal(t, "max_iterations", m["type"])
	assert.Equal(t, 3, m["iterations"])
}

func TestToolLoop_Do_UnknownTool(t *testing.T) {
	server := newMockLLMServer(
		`{"tool":"nonexistent_tool","input":{}}`,
		`"I tried but couldn't."`,
	)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{
		SystemPrompt:  "You are a helpful assistant.",
		MaxIterations: 10,
	})

	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "test"}}}

	result, err := tl.Do(context.Background(), agent)
	require.NoError(t, err)

	m := result.(map[string]any)
	assert.Equal(t, "final", m["type"])
	assert.Equal(t, 2, m["iterations"])
}

func TestToolLoop_Do_ToolExecutionError(t *testing.T) {
	server := newMockLLMServer(
		`{"tool":"http_request","input":{"url":"https://example.com"}}`,
		`"The HTTP request failed."`,
	)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	httpTool := &toolUnderTest{name: "http_request", err: errors.New("connection refused")}
	err = rt.RegisterTool(httpTool)
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{
		SystemPrompt:  "You are a helpful assistant.",
		MaxIterations: 10,
	})

	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "fetch something"}}}

	result, err := tl.Do(context.Background(), agent)
	require.NoError(t, err)

	m := result.(map[string]any)
	assert.Equal(t, "final", m["type"])
	assert.Equal(t, 2, m["iterations"])
}

func TestToolLoop_Do_LLMCallError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
	}))
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{SystemPrompt: "You are a helpful assistant."})
	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "test"}}}

	_, err = tl.Do(context.Background(), agent)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "llm.complete")
}

func TestToolLoop_Do_ToolAliases(t *testing.T) {
	server := newMockLLMServer(
		`{"tool":"search","input":{"type":"semantic"}}`,
		`"Found your preference."`,
	)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	memTool := &toolUnderTest{name: "memory_search", result: []byte(`{"count":1,"results":[{"id":"mem-1"}]}`)}
	err = rt.RegisterTool(memTool)
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{
		SystemPrompt: "You are a helpful assistant.",
		ToolAliases: map[string]string{
			"search": "memory_search",
		},
	})

	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "search memories"}}}

	result, err := tl.Do(context.Background(), agent)
	require.NoError(t, err)

	m := result.(map[string]any)
	assert.Equal(t, "final", m["type"])
	assert.Equal(t, 2, m["iterations"])
}

func TestToolLoop_Do_EmptySystemPrompt(t *testing.T) {
	server := newMockLLMServer(`"final answer."`)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{SystemPrompt: "", MaxIterations: 5})
	assert.Equal(t, DefaultToolLoopPrompt, tl.config.SystemPrompt)

	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "hello"}}}

	result, err := tl.Do(context.Background(), agent)
	require.NoError(t, err)
	m := result.(map[string]any)
	assert.Equal(t, "final", m["type"])
}

func TestToolLoop_Do_ParseError(t *testing.T) {
	server := newMockLLMServer(`{"tool":,}`, `"gave up"`)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{MaxIterations: 10})
	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "test"}}}

	result, err := tl.Do(context.Background(), agent)
	require.NoError(t, err)

	m := result.(map[string]any)
	assert.Contains(t, []string{"final", "parse_error"}, m["type"])
}

func TestToolLoop_Do_MultipleToolCalls(t *testing.T) {
	server := newMockLLMServer(
		`{"tool":"memory_search","input":{"type":"semantic","limit":5}}`,
		`{"tool":"http_request","input":{"url":"https://api.example.com/data"}}`,
		`"Based on memory and API data, the answer is 42."`,
	)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	memTool := &toolUnderTest{name: "memory_search", result: []byte(`{"count":1,"results":[{"id":"mem-1"}]}`)}
	httpTool := &toolUnderTest{name: "http_request", result: []byte(`{"status_code":200,"body":{"value":42}}`)}
	err = rt.RegisterTool(memTool)
	require.NoError(t, err)
	err = rt.RegisterTool(httpTool)
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{MaxIterations: 20})
	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "search and fetch"}}}

	result, err := tl.Do(context.Background(), agent)
	require.NoError(t, err)

	m := result.(map[string]any)
	assert.Equal(t, "final", m["type"])
	assert.Equal(t, 3, m["iterations"])
	assert.Equal(t, 1, memTool.Calls())
	assert.Equal(t, 1, httpTool.Calls())
}

func TestToolLoop_isToolCall_Variations(t *testing.T) {
	rt := NewRuntime()
	tl := NewToolLoop(rt, ToolLoopConfig{})

	cases := []struct {
		desc  string
		input string
		want  bool
	}{
		{"newline after tool", "{\n\"tool\": \"search\",\n\"input\": {}\n}", true},
		{"spaces before json", `   {"tool":"x","input":{}}`, true},
		{"trailing whitespace", `{"tool":"x","input":{}}   \n`, true},
		{"just text no JSON", "I think therefore I am", false},
		{"markdown with extra text", "```json\n{\"tool\":\"search\",\"input\":{}}\n```\n\nThat was the tool call.", true},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			got := tl.isToolCall(c.input)
			assert.Equal(t, c.want, got, "input: %q", c.input)
		})
	}
}

func TestToolLoop_Do_LLMReturnsNonJSON(t *testing.T) {
	server := newMockLLMServer(`"I think we should use the search tool."`)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{MaxIterations: 10})
	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "test"}}}

	result, err := tl.Do(context.Background(), agent)
	require.NoError(t, err)

	m := result.(map[string]any)
	assert.Equal(t, "final", m["type"])
	assert.Equal(t, "I think we should use the search tool.", m["answer"])
}

func TestToolLoop_UsesRuntimeRegistry(t *testing.T) {
	rt := NewRuntime()
	t1 := &toolUnderTest{name: "tool_a", result: []byte(`"a"`)}
	t2 := &toolUnderTest{name: "tool_b", result: []byte(`"b"`)}
	//nolint:errcheck
	rt.RegisterTool(t1)
	//nolint:errcheck
	rt.RegisterTool(t2)

	tl := NewToolLoop(rt, ToolLoopConfig{})
	assert.Equal(t, "tool_a", tl.resolveTool("tool_a"))
	assert.Equal(t, "tool_b", tl.resolveTool("tool_b"))
	assert.Equal(t, "", tl.resolveTool("tool_c"))
}

func TestToolLoop_Do_ToolNotRegistered(t *testing.T) {
	server := newMockLLMServer(
		`{"tool":"memory_write","input":{"type":"semantic","content":{}}}`,
		`"cannot write"`,
	)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	// memory_write NOT registered.
	tl := NewToolLoop(rt, ToolLoopConfig{MaxIterations: 10})
	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "store a memory"}}}

	result, err := tl.Do(context.Background(), agent)
	require.NoError(t, err)

	m := result.(map[string]any)
	assert.Equal(t, "final", m["type"])
	assert.Equal(t, 2, m["iterations"])
}

func TestToolLoop_Do_ToolResultTruncation(t *testing.T) {
	server := newMockLLMServer(
		`{"tool":"http_request","input":{"url":"https://example.com"}}`,
		`"done"`,
	)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	err := rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))
	require.NoError(t, err)

	largeBody := strings.Repeat("x", 1000)
	httpTool := &toolUnderTest{name: "http_request", result: json.RawMessage(`{"status_code":200,"body":"` + largeBody + `"}`)}
	err = rt.RegisterTool(httpTool)
	require.NoError(t, err)

	tl := NewToolLoop(rt, ToolLoopConfig{MaxIterations: 10})
	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "fetch large page"}}}

	result, err := tl.Do(context.Background(), agent)
	require.NoError(t, err)

	m := result.(map[string]any)
	assert.Equal(t, "final", m["type"])
}

func BenchmarkToolLoop_SingleIteration(b *testing.B) {
	server := newMockLLMServer(`"final answer."`)
	defer server.Close()

	rt := NewRuntime()
	llmClient := llm.NewClient(llm.WithBaseURL(server.URL))
	//nolint:errcheck
	rt.RegisterTool(tools.NewLLMCompleteTool(llmClient))

	tl := NewToolLoop(rt, ToolLoopConfig{MaxIterations: 10})
	agent := &Agent{Context: &Context{Memory: map[string]any{"user_message": "hello"}}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		//nolint:errcheck
		tl.Do(context.Background(), agent)
	}
}