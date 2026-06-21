package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	Model       string              `json:"model"`
	Messages    []openAIMessage     `json:"messages"`
	Tools       []openAITool        `json:"tools,omitempty"`
	Temperature float64             `json:"temperature"`
	MaxTokens   int                 `json:"max_tokens"`
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
	// Build OpenAI-compatible messages array
	openAIMsgs := make([]openAIMessage, 0, len(messages)+1)

	// System prompt as first message
	if systemPrompt != "" {
		openAIMsgs = append(openAIMsgs, openAIMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	for _, msg := range messages {
		openAIMsgs = append(openAIMsgs, openAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Build tools
	openAITools := make([]openAITool, 0, len(tools))
	for _, t := range tools {
		openAITools = append(openAITools, openAITool{
			Type: "function",
			Function: openAIFunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters: map[string]any{
					"type":       "object",
					"properties": t.Parameters,
					"required":   t.Required,
				},
			},
		})
	}

	reqBody := openAIChatRequest{
		Model:       p.model,
		Messages:    openAIMsgs,
		Tools:       openAITools,
		Temperature: p.temperature,
		MaxTokens:   p.maxTokens,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		deepseekBaseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("deepseek API request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return &ChatResponse{Text: ""}, nil
	}

	choice := chatResp.Choices[0].Message
	response := &ChatResponse{
		Text: choice.Content,
	}

	// Parse tool calls
	for _, tc := range choice.ToolCalls {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = map[string]any{"raw": tc.Function.Arguments}
		}
		response.ToolCalls = append(response.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return response, nil
}
