package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// openAIStreamToolCall is one incremental tool_call delta in a streaming chunk.
// OpenAI streams tool calls field-by-field: the index identifies the call,
// id/name appear only in the first chunk, and arguments arrive as fragments.
type openAIStreamToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

// openAIStreamChunk is one SSE chunk of an OpenAI-compatible streaming response.
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string                `json:"content"`
			ToolCalls []openAIStreamToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// openAIStreamingChat calls an OpenAI-compatible /chat/completions endpoint
// (shared by DeepSeek and any OpenAI-compat provider).
//
// When onDelta is non-nil the request uses stream mode and every incremental
// text chunk is forwarded to onDelta; tool-call arguments are accumulated
// across chunks. The returned ChatResponse always carries the complete text
// and tool calls regardless of mode.
func openAIStreamingChat(
	ctx context.Context,
	client *http.Client,
	endpointURL, apiKey, model string,
	maxTokens int,
	temperature float64,
	messages []Message,
	tools []ToolDefinition,
	systemPrompt string,
	onDelta func(string),
) (*ChatResponse, error) {
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
		Model:       model,
		Messages:    openAIMsgs,
		Tools:       openAITools,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Stream:      onDelta != nil,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible API request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai-compatible API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	if onDelta == nil {
		return parseOpenAIResponse(resp.Body)
	}
	return parseOpenAIStream(resp.Body, onDelta)
}

// parseOpenAIResponse parses a non-streaming chat/completions response body.
func parseOpenAIResponse(body io.Reader) (*ChatResponse, error) {
	respBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return responseFromOpenAIChoice(chatResp), nil
}

// responseFromOpenAIChoice converts a completed OpenAI response into a
// provider-agnostic ChatResponse.
func responseFromOpenAIChoice(chatResp openAIChatResponse) *ChatResponse {
	response := &ChatResponse{}
	if len(chatResp.Choices) == 0 {
		return response
	}
	choice := chatResp.Choices[0].Message
	response.Text = choice.Content
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
	return response
}

// parseOpenAIStream parses an SSE streaming body (`data:` lines), forwarding
// each text chunk via onDelta and accumulating tool-call fragments.
func parseOpenAIStream(body io.Reader, onDelta func(string)) (*ChatResponse, error) {
	response := &ChatResponse{}

	type toolAcc struct {
		id        string
		name      string
		arguments strings.Builder
	}
	accs := map[int]*toolAcc{}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // tolerate keep-alive / partial lines
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			response.Text += delta.Content
			onDelta(delta.Content)
		}
		for _, tc := range delta.ToolCalls {
			acc, ok := accs[tc.Index]
			if !ok {
				// Bound memory: cap the number of tool calls and reject
				// negative/malicious indexes.
				if tc.Index < 0 || len(accs) >= 32 {
					continue
				}
				acc = &toolAcc{}
				accs[tc.Index] = acc
			}
			if tc.ID != "" {
				acc.id = tc.ID
			}
			if tc.Function.Name != "" {
				acc.name = tc.Function.Name
			}
			// Bound the accumulated arguments JSON per call (64 KiB).
			if acc.arguments.Len() < 64<<10 {
				acc.arguments.WriteString(tc.Function.Arguments)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stream: %w", err)
	}

	// Assemble tool calls in index order for deterministic output.
	indexes := make([]int, 0, len(accs))
	for i := range accs {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	for _, i := range indexes {
		acc := accs[i]
		var args map[string]any
		if err := json.Unmarshal([]byte(acc.arguments.String()), &args); err != nil {
			args = map[string]any{"raw": acc.arguments.String()}
		}
		response.ToolCalls = append(response.ToolCalls, ToolCall{
			ID:        acc.id,
			Name:      acc.name,
			Arguments: args,
		})
	}
	return response, nil
}
