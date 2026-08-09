package llm

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const deepseekBaseURL = "https://api.deepseek.com/v1"

// DeepSeekProvider implements LLMProvider using the DeepSeek API.
// DeepSeek follows an OpenAI-compatible format.
type DeepSeekProvider struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
}

// NewDeepSeekProvider creates a DeepSeek-backed LLM provider.
func NewDeepSeekProvider(apiKey, model string, maxTokens int, temperature float64) *DeepSeekProvider {
	return &DeepSeekProvider{
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (p *DeepSeekProvider) Name() string {
	return fmt.Sprintf("DeepSeek (%s)", p.model)
}

// --- OpenAI-compatible request/response types ---

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Tools       []openAITool    `json:"tools,omitempty"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens"`
	Stream      bool            `json:"stream,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string              `json:"type"`
	Function openAIFunctionDef   `json:"function"`
}

type openAIFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatResponse struct {
	Choices []openAIChoice `json:"choices"`
}

type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

func (p *DeepSeekProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, systemPrompt string) (*ChatResponse, error) {
	return openAIStreamingChat(ctx, p.httpClient, deepseekBaseURL+"/chat/completions",
		p.apiKey, p.model, p.maxTokens, p.temperature, messages, tools, systemPrompt, nil)
}

// StreamChat streams the response, forwarding text deltas to onDelta.
func (p *DeepSeekProvider) StreamChat(ctx context.Context, messages []Message, tools []ToolDefinition, systemPrompt string, onDelta func(string)) (*ChatResponse, error) {
	return openAIStreamingChat(ctx, p.httpClient, deepseekBaseURL+"/chat/completions",
		p.apiKey, p.model, p.maxTokens, p.temperature, messages, tools, systemPrompt, onDelta)
}
