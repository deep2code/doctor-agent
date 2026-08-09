package llm

import (
	"context"
	"fmt"
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
	return openAIStreamingChat(ctx, p.httpClient, p.baseURL+"/chat/completions",
		p.apiKey, p.model, p.maxTokens, p.temperature, messages, tools, systemPrompt, nil)
}

// StreamChat streams the response, forwarding text deltas to onDelta.
func (p *OpenAICompatProvider) StreamChat(ctx context.Context, messages []Message, tools []ToolDefinition, systemPrompt string, onDelta func(string)) (*ChatResponse, error) {
	return openAIStreamingChat(ctx, p.httpClient, p.baseURL+"/chat/completions",
		p.apiKey, p.model, p.maxTokens, p.temperature, messages, tools, systemPrompt, onDelta)
}
