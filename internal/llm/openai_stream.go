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
			Content          string                `json:"content"`
			ReasoningContent string                `json:"reasoning_content"`
			ToolCalls        []openAIStreamToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage,omitempty"`
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
	openAIMsgs := make([]any, 0, len(messages)+1)
	if systemPrompt != "" {
		openAIMsgs = append(openAIMsgs, openAIMessage{Role: "system", Content: systemPrompt})
	}
	for _, msg := range messages {
		// Handle multimodal content
		if msg.HasImages() {
			var parts []openAIContentPart
			// Add text content if present
			if msg.Content != "" {
				parts = append(parts, openAIContentPart{Type: "text", Text: msg.Content})
			}
			// Add image parts
			for _, part := range msg.Parts {
				if part.Type == "image" && part.Image != nil {
					var imgURL string
					if part.Image.Base64Data != "" {
						imgURL = fmt.Sprintf("data:%s;base64,%s", part.Image.MediaType, part.Image.Base64Data)
					} else if part.Image.URL != "" {
						imgURL = part.Image.URL
					}
					if imgURL != "" {
						parts = append(parts, openAIContentPart{
							Type:     "image_url",
							ImageURL: &openAIImageURL{URL: imgURL},
						})
					}
				}
			}
			// Add any text parts from msg.Parts
			for _, part := range msg.Parts {
				if part.Type == "text" && part.Text != "" {
					parts = append(parts, openAIContentPart{Type: "text", Text: part.Text})
				}
			}
			openAIMsgs = append(openAIMsgs, openAIMessageWithParts{
				Role:    msg.Role,
				Content: parts,
			})
		} else {
			m := openAIMessage{
				Role:             msg.Role,
				Content:          msg.Content,
				ReasoningContent: msg.ReasoningContent,
				ToolCallID:       msg.ToolCallID,
			}
			// Assistant tool-call messages must carry tool_calls (OpenAI-compatible
			// endpoints reject assistant messages with neither content nor
			// tool_calls; empty content is omitted by the omitempty tag).
			if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
				tcs := make([]openAIToolCall, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					args, err := json.Marshal(tc.Arguments)
					if err != nil {
						args = []byte("{}")
					}
					tcs = append(tcs, openAIToolCall{
						ID:   tc.ID,
						Type: "function",
						Function: openAIFunctionCall{
							Name:      tc.Name,
							Arguments: string(args),
						},
					})
				}
				m.ToolCalls = tcs
			}
			openAIMsgs = append(openAIMsgs, m)
		}
	}

	openAITools := make([]openAITool, 0, len(tools))
	for _, t := range tools {
		required := t.Required
		if required == nil {
			required = []string{}
		}
		openAITools = append(openAITools, openAITool{
			Type: "function",
			Function: openAIFunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters: map[string]any{
					"type":                 "object",
					"properties":           t.Parameters,
					"required":             required,
					"additionalProperties": false,
				},
			},
		})
	}

	reqBody := openAIChatRequest{
		Model:               model,
		Messages:            openAIMsgs,
		Tools:               openAITools,
		Temperature:         temperature,
		MaxTokens:           maxTokens,
		MaxCompletionTokens: maxTokens,
		Stream:              onDelta != nil,
	}
	if onDelta != nil {
		reqBody.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
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
	response.ReasoningContent = choice.ReasoningContent
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
		// DeepSeek V4 (thinking mode) sends reasoning_content as a
		// separate field. We accumulate it and pass it back in the
		// assistant message so the API can validate multi-turn tool
		// conversations (otherwise it returns HTTP 400).
		if delta.ReasoningContent != "" {
			response.ReasoningContent += delta.ReasoningContent
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
