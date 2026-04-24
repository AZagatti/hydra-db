package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ToolLoopConfig controls the behavior of a ToolLoop agent.
type ToolLoopConfig struct {
	// SystemPrompt is injected before each user message to guide tool selection.
	SystemPrompt string
	// MaxIterations limits how many tool-call cycles the agent can perform.
	// A value of 0 means no limit.
	MaxIterations int
	// ToolAliases maps friendly shortcut names to actual registered tool names.
	// Example: {"search": "memory_search", "web": "http_request"}
	ToolAliases map[string]string
}

// DefaultToolLoopPrompt is the default system prompt for tool-loop agents.
const DefaultToolLoopPrompt = `You are an agent with access to tools. Each tool is identified by name.

Available tools:
- memory_write: Store a structured memory record (episodic/semantic/operational/working)
- memory_search: Query the memory plane for relevant past context
- tdb_search: Perform latent semantic search against TardigradeDB
- http_request: Make outbound HTTP requests
- llm.complete: Call the LLM for reasoning or complex extraction

When you need to perform an action, respond with a JSON tool-call request:
{"tool": "tool_name", "input": {...tool-specific input...}}

After each tool result, you will receive the result and can decide the next action.

Stop when you have a final answer. Format final answers clearly.`

// ToolLoop wraps a Func so that instead of executing a static function, the
// agent cycles through tool calls until it produces a final result or hits
// MaxIterations.
//
// The agent uses an LLM (via llm.complete tool) to decide which tool to call
// and when to stop. This is the "option C" approach: the LLM sidecar drives
// the tool loop, giving us a self-contained agent that can use memory, HTTP,
// and TDB tools with proper tool-call granularity.
//
// # Usage
//
// Instead of:
//
//	rt.Spawn(ctx, "my-agent", func(ctx, agent) (any, error) { return "hello", nil })
//
// Use:
//
//	toolLoop := agent.NewToolLoop(rt, llmClient, agent.ToolLoopConfig{
//	    SystemPrompt: "You are a helpful assistant with memory.",
//	    MaxIterations: 20,
//	})
//	rt.Spawn(ctx, "my-agent", toolLoop.Do)
type ToolLoop struct {
	runtime  *Runtime
	config   ToolLoopConfig
	registry *ToolRegistry // separate ref to avoid lock on hot path
}

// NewToolLoop creates a ToolLoop that drives agent execution via tools.
// The runtime is used to look up registered tools by name.
func NewToolLoop(runtime *Runtime, config ToolLoopConfig) *ToolLoop {
	if config.SystemPrompt == "" {
		config.SystemPrompt = DefaultToolLoopPrompt
	}
	if config.ToolAliases == nil {
		config.ToolAliases = make(map[string]string)
	}
	return &ToolLoop{
		runtime:  runtime,
		config:   config,
		registry: runtime.registry,
	}
}

// Do is the Func that a Spawn call can pass to execute via tool loop.
// It cycles: LLM decides tool → execute tool → inject result → repeat
// until the LLM returns a final answer or MaxIterations is hit.
func (tl *ToolLoop) Do(ctx context.Context, agent *Agent) (any, error) {
	// Retrieve the original user message from agent context or result.
	// The /chat handler embeds the user message in the agent's context metadata.
	userMsg := tl.extractUserMessage(agent)

	iteration := 0
	maxIterations := tl.config.MaxIterations

	// Build conversation history for the LLM.
	// We use a simple text protocol: human messages, tool responses, assistant reasoning.
	var conversation []string

	for {
		if maxIterations > 0 && iteration >= maxIterations {
			return map[string]any{
				"type":    "max_iterations",
				"iterations": iteration,
				"answer":  strings.Join(conversation, "\n"),
			}, nil
		}

		// Build the prompt for the LLM.
		// Include: system prompt, conversation history, user message.
		prompt := tl.buildPrompt(userMsg, conversation)

		// Ask the LLM what to do next.
		llmResp, err := tl.callLLM(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("llm call: %w", err)
		}

		// Check if the LLM returned a final answer (no tool call).
		if !tl.isToolCall(llmResp) {
			// Final answer — we're done.
			return map[string]any{
				"type":       "final",
				"answer":     llmResp,
				"iterations": iteration,
			}, nil
		}

		// Parse and execute the tool call.
		toolName, toolInput, parseErr := tl.parseToolCall(llmResp)
		if parseErr != nil {
			// Malformed tool call — treat as final answer.
			return map[string]any{
				"type":       "parse_error",
				"raw":        llmResp,
				"iterations": iteration,
			}, nil
		}

		// Resolve aliases.
		resolved := tl.resolveTool(toolName)
		if resolved == "" {
			conversation = append(conversation,
				fmt.Sprintf("[tool=%s] ERROR: unknown tool", toolName))
			iteration++
			continue
		}

		// Check tool access.
		allowed, err := tl.registry.Get(resolved)
		if err != nil {
			conversation = append(conversation,
				fmt.Sprintf("[tool=%s] ERROR: %v", resolved, err))
			iteration++
			continue
		}

		// Execute the tool.
		result, execErr := allowed.Execute(ctx, toolInput)
		if execErr != nil {
			conversation = append(conversation,
				fmt.Sprintf("[tool=%s] ERROR: %v", resolved, execErr))
			iteration++
			continue
		}

		// Append tool result to conversation.
		var resultStr string
		if len(result) > 500 {
			resultStr = string(result[:500]) + "... (truncated)"
		} else {
			resultStr = string(result)
		}
		conversation = append(conversation,
			fmt.Sprintf("[tool=%s] RESULT: %s", resolved, resultStr))

		iteration++
	}
}

// resolveTool maps an alias to a registered tool name.
func (tl *ToolLoop) resolveTool(name string) string {
	if actual, ok := tl.config.ToolAliases[name]; ok {
		return actual
	}
	// Check if it's a direct tool name.
	_, err := tl.registry.Get(name)
	if err == nil {
		return name
	}
	return ""
}

// buildPrompt assembles the full prompt sent to the LLM each iteration.
func (tl *ToolLoop) buildPrompt(userMsg string, history []string) string {
	var sb strings.Builder
	sb.WriteString("SYSTEM: ")
	sb.WriteString(tl.config.SystemPrompt)
	sb.WriteString("\n\n")

	if len(history) > 0 {
		sb.WriteString("CONVERSATION HISTORY:\n")
		for _, h := range history {
			sb.WriteString(h)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("USER: ")
	sb.WriteString(userMsg)
	return sb.String()
}

// callLLM invokes the llm.complete tool with the given prompt.
// Returns the text field of the completion.
func (tl *ToolLoop) callLLM(ctx context.Context, prompt string) (string, error) {
	llmTool, err := tl.registry.Get("llm.complete")
	if err != nil {
		return "", fmt.Errorf("llm.complete tool not registered: %w", err)
	}

	input := map[string]any{
		"system_prompt": tl.config.SystemPrompt,
		"user_message":  prompt,
		"max_tokens":    2048,
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("marshal llm input: %w", err)
	}

	result, err := llmTool.Execute(ctx, payload)
	if err != nil {
		return "", fmt.Errorf("llm.complete execute: %w", err)
	}

	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		return "", fmt.Errorf("unmarshal llm output: %w", err)
	}

	return out.Text, nil
}

// isToolCall detects whether an LLM response contains a JSON tool-call request.
// A tool call response is a JSON object with "tool" and "input" fields.
func (tl *ToolLoop) isToolCall(response string) bool {
	// Fast path: check for JSON object with tool field.
	var candidate struct {
		Tool string `json:"tool"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(response)), &candidate); err != nil {
		return false
	}
	return candidate.Tool != ""
}

// parseToolCall extracts the tool name and input JSON from an LLM response.
func (tl *ToolLoop) parseToolCall(response string) (tool string, input json.RawMessage, err error) {
	// Try to extract the JSON object from the response.
	// The LLM might return markdown-wrapped JSON.
	trimmed := strings.TrimSpace(response)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	var call struct {
		Tool  string          `json:"tool"`
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal([]byte(trimmed), &call); err != nil {
		return "", nil, fmt.Errorf("parse tool call JSON: %w", err)
	}
	if call.Tool == "" {
		return "", nil, fmt.Errorf("tool field is empty")
	}
	return call.Tool, call.Input, nil
}

// extractUserMessage pulls the original user message from the agent.
// In the /chat handler, we store the message in agent.Context.Memory["user_message"].
func (tl *ToolLoop) extractUserMessage(agent *Agent) string {
	if agent.Context != nil {
		if msg, ok := agent.Context.Memory["user_message"]; ok {
			if s, ok := msg.(string); ok {
				return s
			}
		}
	}
	return ""
}