package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
)

// AnthropicProvider implements LLMProvider using the Anthropic Claude API.
type AnthropicProvider struct {
	client    anthropic.Client
	model     string
	maxTokens int64
	temperature float64
}

// NewAnthropicProvider creates an Anthropic-backed LLM provider.
func NewAnthropicProvider(apiKey, model string, maxTokens int, temperature float64) *AnthropicProvider {
	return &AnthropicProvider{
		client: anthropic.NewClient(
			option.WithAPIKey(apiKey),
		),
		model:       model,
		maxTokens:   int64(maxTokens),
		temperature: temperature,
	}
}

func (p *AnthropicProvider) Name() string {
	return fmt.Sprintf("Anthropic Claude (%s)", p.model)
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, systemPrompt string) (*ChatResponse, error) {
	resp, err := p.client.Messages.New(ctx, p.buildParams(messages, tools, systemPrompt))
	if err != nil {
		return nil, fmt.Errorf("anthropic API error: %w", err)
	}

	return responseFromAnthropicMessage(resp.Content), nil
}

// StreamChat streams the response via the Anthropic streaming API: visible
// text deltas are forwarded to onDelta, tool_use blocks are accumulated from
// their incremental JSON and returned in the final ChatResponse.
func (p *AnthropicProvider) StreamChat(ctx context.Context, messages []Message, tools []ToolDefinition, systemPrompt string, onDelta func(string)) (*ChatResponse, error) {
	stream := p.client.Messages.NewStreaming(ctx, p.buildParams(messages, tools, systemPrompt))
	defer func() { _ = stream.Close() }()

	chatResp := &ChatResponse{}

	// Accumulate tool_use blocks: content-block index -> {id, name, args}
	type toolAcc struct {
		id        string
		name      string
		args      strings.Builder
	}
	accs := map[int64]*toolAcc{}

	for stream.Next() {
		switch v := stream.Current().AsAny().(type) {
		case anthropic.ContentBlockStartEvent:
			// Bound the accumulated tool_use map: only accept sane block
			// indexes and cap the number of concurrent tool calls.
			if v.ContentBlock.Type == "tool_use" && v.Index >= 0 && v.Index < 64 && len(accs) < 32 {
				accs[v.Index] = &toolAcc{id: v.ContentBlock.ID, name: v.ContentBlock.Name}
			}
		case anthropic.ContentBlockDeltaEvent:
			switch v.Delta.Type {
			case "text_delta":
				if v.Delta.Text != "" {
					chatResp.Text += v.Delta.Text
					if onDelta != nil {
						onDelta(v.Delta.Text)
					}
				}
			case "input_json_delta":
				if acc, ok := accs[v.Index]; ok && acc.args.Len() < 64<<10 {
					acc.args.WriteString(v.Delta.PartialJSON)
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("anthropic stream error: %w", err)
	}

	// Assemble tool calls in block-index order for deterministic output.
	indexes := make([]int, 0, len(accs))
	for i := range accs {
		indexes = append(indexes, int(i))
	}
	sort.Ints(indexes)
	for _, i := range indexes {
		acc := accs[int64(i)]
		chatResp.ToolCalls = append(chatResp.ToolCalls, ToolCall{
			ID:        acc.id,
			Name:      acc.name,
			Arguments: parseJSONObject(json.RawMessage(acc.args.String())),
		})
	}
	return chatResp, nil
}

// buildParams converts provider-agnostic messages/tools/system into an
// Anthropic MessageNewParams (shared by Chat and StreamChat).
func (p *AnthropicProvider) buildParams(messages []Message, tools []ToolDefinition, systemPrompt string) anthropic.MessageNewParams {
	// Convert internal messages to Anthropic format
	anthropicMessages := make([]anthropic.MessageParam, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			// Handle multimodal content (text + images)
			if msg.HasImages() {
				var blocks []anthropic.ContentBlockParamUnion
				for _, part := range msg.Parts {
					switch part.Type {
					case "text":
						if part.Text != "" {
							blocks = append(blocks, anthropic.NewTextBlock(part.Text))
						}
					case "image":
						if part.Image != nil {
							// Anthropic image block
							blocks = append(blocks, anthropic.ContentBlockParamUnion{
								OfImage: &anthropic.ImageBlockParam{
									Source: anthropic.ImageBlockParamSourceUnion{
										OfBase64: &anthropic.Base64ImageSourceParam{
											Data:      part.Image.Base64Data,
											MediaType: anthropic.Base64ImageSourceMediaType(part.Image.MediaType),
										},
									},
								},
							})
						}
					}
				}
				// Fallback to text content if no parts
				if len(blocks) == 0 && msg.Content != "" {
					blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
				}
				anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(blocks...))
			} else {
				anthropicMessages = append(anthropicMessages,
					anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
			}
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Assistant tool-use turn: text (if any) + tool_use blocks.
				// Anthropic rejects assistant messages with neither content
				// nor tool_use, so empty text is omitted.
				var blocks []anthropic.ContentBlockParamUnion
				if msg.Content != "" {
					blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
				}
				for _, tc := range msg.ToolCalls {
					blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, tc.Arguments, tc.Name))
				}
				anthropicMessages = append(anthropicMessages, anthropic.NewAssistantMessage(blocks...))
			} else {
				anthropicMessages = append(anthropicMessages,
					anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
			}
		}
	}

	// Build system prompt
	systemBlocks := []anthropic.TextBlockParam{
		{Text: systemPrompt},
	}

	return anthropic.MessageNewParams{
		Model:       anthropic.Model(p.model),
		MaxTokens:   p.maxTokens,
		Messages:    anthropicMessages,
		System:      systemBlocks,
		Temperature: param.Opt[float64]{Value: p.temperature},
		Tools:       p.convertTools(tools),
	}
}

// responseFromAnthropicMessage parses a completed Anthropic message content
// into a provider-agnostic ChatResponse.
func responseFromAnthropicMessage(content []anthropic.ContentBlockUnion) *ChatResponse {
	chatResp := &ChatResponse{}
	for _, block := range content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				chatResp.Text += block.Text
			}
		case "tool_use":
			args := parseJSONObject(block.Input)
			chatResp.ToolCalls = append(chatResp.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}
	return chatResp
}

func (p *AnthropicProvider) convertTools(tools []ToolDefinition) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		required := t.Required
		if required == nil {
			required = []string{}
		}

		toolParam := anthropic.ToolParam{
			Name:        t.Name,
			Description: param.Opt[string]{Value: t.Description},
			InputSchema: anthropic.ToolInputSchemaParam{
				Type:       constant.Object("object"),
				Properties: t.Parameters,
				Required:   required,
			},
		}

		result = append(result, anthropic.ToolUnionParam{
			OfTool: &toolParam,
		})
	}
	return result
}

// parseJSONObject extracts a map from a json.RawMessage.
func parseJSONObject(raw json.RawMessage) map[string]any {
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return map[string]any{"raw": string(raw)}
	}
	return result
}
