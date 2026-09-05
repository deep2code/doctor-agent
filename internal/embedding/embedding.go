package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Provider is the interface for embedding models.
type Provider interface {
	// Embed converts text to a vector.
	Embed(text string) ([]float32, error)
	// EmbedBatch converts multiple texts to vectors.
	EmbedBatch(texts []string) ([][]float32, error)
	// Dimensions returns the vector dimensions.
	Dimensions() int
	// Name returns the provider name.
	Name() string
}

// Config holds embedding provider configuration.
type Config struct {
	Provider   string // "openai-compat"
	BaseURL    string
	APIKey     string
	Model      string
	Dimensions int // 0 = use API default; 1024 forces 1024 for embedding-3-pro
}

// OpenAICompatProvider implements embedding using OpenAI-compatible API.
type OpenAICompatProvider struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
}

// NewOpenAICompat creates a new OpenAI-compatible embedding provider.
// apiKey is optional (e.g. Ollama local endpoint needs no auth).
func NewOpenAICompat(cfg Config) (*OpenAICompatProvider, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base_url is required")
	}
	if cfg.Model == "" {
		cfg.Model = "text-embedding-v3"
	}

	return &OpenAICompatProvider{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:     cfg.APIKey, // may be empty for local Ollama
		model:      cfg.Model,
		dimensions: cfg.Dimensions, // 0 = API default; >0 = request specific dims
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

// embeddingRequest represents the API request.
type embeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"` // only sent when >0
}

// embeddingResponse represents the API response.
type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// Embed converts text to a vector.
func (p *OpenAICompatProvider) Embed(text string) ([]float32, error) {
	results, err := p.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return results[0], nil
}

// EmbedBatch converts multiple texts to vectors.
func (p *OpenAICompatProvider) EmbedBatch(texts []string) ([][]float32, error) {
	reqBody := embeddingRequest{
		Model:      p.model,
		Input:      texts,
		Dimensions: p.dimensions,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", p.baseURL+"/embeddings", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log but don't fail - we already have the body
			fmt.Printf("Warning: failed to close response body: %v\n", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var embeddingResp embeddingResponse
	if err := json.Unmarshal(body, &embeddingResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	// Sort by index and extract embeddings
	results := make([][]float32, len(texts))
	for _, item := range embeddingResp.Data {
		if item.Index < len(results) {
			results[item.Index] = item.Embedding
		}
	}

	// Dimensions are set at construction time; do not auto-update from
	// responses to avoid data races when EmbedBatch is called concurrently
	// by a worker pool (e.g. parallel bake). The value returned by the API
	// is deterministic for a given model.

	return results, nil
}

// Dimensions returns the vector dimensions.
func (p *OpenAICompatProvider) Dimensions() int {
	return p.dimensions
}

// Name returns the provider name.
func (p *OpenAICompatProvider) Name() string {
	return fmt.Sprintf("openai-compat:%s", p.model)
}
