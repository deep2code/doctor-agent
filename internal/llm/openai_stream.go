package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	disableThinking bool,
) (*ChatResponse, error) {
	openAIMsgs := make([]any, 0, len(messages)+1)
	if systemPrompt != "" {
		openAIMsgs = append(openAIMsgs, openAIMessage{Role: "system", Content: systemPrompt})
	}
	// When an empty assistant message is dropped, any tool messages that
	// follow it (up to the next user/assistant message) must also be dropped:
	// a tool message without its preceding assistant tool_call is invalid and
	// will be rejected by the API with "messages with role 'tool' must be a
	// response to a tool call". This tracks that state across iterations.
	droppingToolMsgs := false
	for _, msg := range messages {
		// Defence-in-depth: drop assistant messages that carry neither text
		// nor tool calls. OpenAI-compatible endpoints reject them with
		// "Invalid assistant message: content or tool_calls must be set"
		// (HTTP 400). They can enter the history when a previous turn's LLM
		// returned an empty body (e.g. truncated output, thinking-only
		// response) and was stored verbatim into the session.
		// Whitespace-only content also counts as empty — some endpoints
		// reject it the same way.
		if msg.Role == "assistant" && strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) == 0 {
			slog.Warn("Dropping empty assistant message from request history",
				"has_reasoning", msg.ReasoningContent != "",
				"reasoning_len", len(msg.ReasoningContent),
				"content_preview", truncateForLog(msg.Content, 80))
			droppingToolMsgs = true
			continue
		}
		// A non-empty assistant or user message resets the drop state.
		if msg.Role == "assistant" || msg.Role == "user" {
			droppingToolMsgs = false
		}
		// Drop orphaned tool messages that followed a dropped empty assistant.
		if droppingToolMsgs && msg.Role == "tool" {
			slog.Warn("Dropping orphaned tool message after empty assistant",
				"tool_call_id", truncateForLog(msg.ToolCallID, 40))
			continue
		}
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
			// Non-multimodal path. If Content is empty but the message carries
			// text parts (e.g. a historical message that was built with Parts
			// but no image), fold the text parts into Content so nothing is
			// silently dropped.
			content := msg.Content
			if content == "" && len(msg.Parts) > 0 {
				var texts []string
				for _, p := range msg.Parts {
					if p.Type == "text" && p.Text != "" {
						texts = append(texts, p.Text)
					}
				}
				content = strings.Join(texts, "\n")
			}
			m := openAIMessage{
				Role:             msg.Role,
				Content:          content,
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
		Model:       model,
		Messages:    openAIMsgs,
		Tools:       openAITools,
		Temperature: temperature,
		// Only max_tokens is set: max_completion_tokens is the newer OpenAI
		// field and is NOT supported by DeepSeek / Zhipu / most compatible
		// endpoints. Sending both can cause "Cannot specify both" errors or
		// silent parameter rejection on stricter endpoints.
		MaxTokens: maxTokens,
		Stream:    onDelta != nil,
	}
	// DeepSeek V4 defaults to thinking mode, which adds a long reasoning
	// chain before the final answer. For fast deterministic sub-tasks
	// (query understanding, judge verification) we disable it to cut
	// latency and avoid max_tokens exhaustion on reasoning alone.
	if disableThinking {
		reqBody.Thinking = &openAIThinking{Type: "disabled"}
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
		// On bad requests (400), dump the outgoing message roles and content
		// lengths so we can identify which message the provider is rejecting.
		if resp.StatusCode == http.StatusBadRequest {
			slog.Error("OpenAI-compatible API returned 400; dumping request messages for diagnosis",
				"model", model,
				"endpoint", endpointURL,
				"message_count", len(openAIMsgs),
				"request_body_bytes", len(bodyBytes),
				"response", string(respBytes))
			for i, m := range openAIMsgs {
				switch mm := m.(type) {
				case openAIMessage:
					slog.Error("  request message",
						"index", i,
						"role", mm.Role,
						"content_len", len(mm.Content),
						"content_preview", truncateForLog(mm.Content, 120),
						"tool_calls", len(mm.ToolCalls),
						"has_reasoning", mm.ReasoningContent != "",
						"tool_call_id", mm.ToolCallID)
				case openAIMessageWithParts:
					slog.Error("  request message (multimodal)",
						"index", i,
						"role", mm.Role,
						"parts", len(mm.Content),
						"tool_calls", len(mm.ToolCalls))
				}
			}
		}
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
	if chatResp.Usage != nil {
		response.Usage = TokenUsage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
		}
	}
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
		// Usage arrives on a dedicated final chunk (stream_options.include_usage)
		// with an empty choices array — capture before the choices check.
		if chunk.Usage != nil {
			response.Usage = TokenUsage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
			}
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

// truncateForLog returns a preview of s suitable for log output, capped at
// maxRunes runes. Multi-line strings are collapsed to a single line.
func truncateForLog(s string, maxRunes int) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", "\\n")
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}
