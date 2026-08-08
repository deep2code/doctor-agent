package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatProvider implements LLMProvider against any OpenAI-compatible
// chat-completions endpoint (e.g. Zhipu BigModel, Qwen DashScope, SiliconFlow).
// It reuses the same request/response types as DeepSeekProvider (same package).
type OpenAICompatProvider struct {
	baseURL     string
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
}

// NewOpenAICompatProvider creates a provider for an OpenAI-compatible endpoint.
// baseURL is the endpoint root, e.g. "https://open.bigmodel.cn/api/paas/v4".
func NewOpenAICompatProvider(baseURL, apiKey, model string, maxTokens int, temperature float64) *OpenAICompatProvider {
	return &OpenAICompatProvider{
		baseURL:     strings.TrimRight(baseURL, "/"),
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

func (p *OpenAICompatProvider) Name() string {
	return fmt.Sprintf("OpenAI-compatible (%s)", p.model)
}

func (p *OpenAICompatProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, systemPrompt string) (*ChatResponse, error) {
	openAIMsgs := make([]openAIMessage, 0, len(messages)+1)
	if systemPrompt != "" {
		openAIMsgs = append(openAIMsgs, openAIMessage{Role: "system", Content: systemPrompt})
	}
	for _, msg := range messages {
		openAIMsgs = append(openAIMsgs, openAIMessage{Role: msg.Role, Content: msg.Content})
	}

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
		p.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai-compat API request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai-compat API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return &ChatResponse{Text: ""}, nil
	}

	choice := chatResp.Choices[0].Message
	response := &ChatResponse{Text: choice.Content}
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
