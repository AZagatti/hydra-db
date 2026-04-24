package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/azagatti/hydra-db/internal/llm"
)

// LLMCompleteTool implements agent.Tool, providing LLM completion
// capability to agents via the sidecar service.
type LLMCompleteTool struct {
	client *llm.Client
}

// NewLLMCompleteTool creates a tool backed by the given LLM client.
func NewLLMCompleteTool(client *llm.Client) *LLMCompleteTool {
	return &LLMCompleteTool{client: client}
}

// Name returns the tool identifier.
func (t *LLMCompleteTool) Name() string { return "llm.complete" }

type llmInput struct {
	SystemPrompt string   `json:"system_prompt"`
	UserMessage  string   `json:"user_message"`
	MaxTokens    int      `json:"max_tokens,omitempty"`
	Temperature  *float64 `json:"temperature,omitempty"`
}

type llmOutput struct {
	Text  string    `json:"text"`
	Usage llm.Usage `json:"usage"`
}

// Execute takes a system prompt and user message, calls the LLM sidecar,
// and returns the completion text with usage information.
func (t *LLMCompleteTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in llmInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("unmarshal llm input: %w", err)
	}

	if in.SystemPrompt == "" || in.UserMessage == "" {
		return nil, fmt.Errorf("system_prompt and user_message are required")
	}

	resp, err := t.client.Complete(ctx, llm.CompleteRequest{
		SystemPrompt: in.SystemPrompt,
		UserMessage:  in.UserMessage,
		MaxTokens:    in.MaxTokens,
		Temperature:  in.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("llm complete: %w", err)
	}

	out := llmOutput{
		Text:  resp.Text,
		Usage: resp.Usage,
	}

	result, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshal llm output: %w", err)
	}

	return result, nil
}
