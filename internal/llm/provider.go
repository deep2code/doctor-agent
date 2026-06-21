package llm

import "context"

// Message represents a single chat message.
type Message struct {
	Role    string // "system", "user", "assistant"
	Content string
}

// ToolDefinition describes a tool available to the LLM.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema properties
	Required    []string
}

// ToolCall represents a tool call requested by the LLM.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// ChatResponse wraps the LLM's response.
type ChatResponse struct {
	Text      string
	ToolCalls []ToolCall
}

// LLMProvider is the interface all LLM backends must implement.
// This enables switching between Anthropic Claude and DeepSeek (and
// potentially other providers) without changing the agent core.
type LLMProvider interface {
	// Chat sends a conversation to the LLM and returns the response.
	// The systemPrompt is separate from messages to allow provider-specific
	// handling (e.g., Anthropic's top-level system param vs OpenAI's system role).
	Chat(ctx context.Context, messages []Message, tools []ToolDefinition, systemPrompt string) (*ChatResponse, error)

	// Name returns a human-readable identifier for this provider (e.g., "Anthropic Claude", "DeepSeek V4").
	Name() string
}
