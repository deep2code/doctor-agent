package llm

import (
	"context"
	"encoding/json"
	"fmt"

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
	// Convert internal messages to Anthropic format
	anthropicMessages := make([]anthropic.MessageParam, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			anthropicMessages = append(anthropicMessages,
				anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		case "assistant":
			anthropicMessages = append(anthropicMessages,
				anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
		}
	}

	// Build system prompt
	systemBlocks := []anthropic.TextBlockParam{
		{Text: systemPrompt},
	}

	// Build tool definitions
	anthropicTools := p.convertTools(tools)

	params := anthropic.MessageNewParams{
		Model:       anthropic.Model(p.model),
		MaxTokens:   p.maxTokens,
		Messages:    anthropicMessages,
		System:      systemBlocks,
		Temperature: param.Opt[float64]{Value: p.temperature},
		Tools:       anthropicTools,
	}

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic API error: %w", err)
	}

	// Parse response
	chatResp := &ChatResponse{}
	for _, block := range resp.Content {
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

	return chatResp, nil
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
