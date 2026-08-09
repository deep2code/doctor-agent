package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const deepseekEmbedBaseURL = "https://api.deepseek.com/v1"

// DeepSeekEmbedder uses DeepSeek's embedding API.
// Note: DeepSeek may or may not offer embeddings in their current API.
// This is a forward-looking implementation using the OpenAI-compatible embeddings format.
// If DeepSeek doesn't support embeddings, use VoyageEmbedder instead.
type DeepSeekEmbedder struct {
	apiKey     string
	model      string
	dimensions int
	httpClient *http.Client
}

// NewDeepSeekEmbedder creates a DeepSeek embedder.
func NewDeepSeekEmbedder(apiKey, model string) *DeepSeekEmbedder {
	return &DeepSeekEmbedder{
		apiKey:     apiKey,
		model:      model,
		dimensions: 1024,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *DeepSeekEmbedder) Name() string {
	return fmt.Sprintf("DeepSeek Embedding (%s)", e.model)
}

func (e *DeepSeekEmbedder) Dimensions() int {
	return e.dimensions
}

func (e *DeepSeekEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("deepseek embed: empty response")
	}
	return vectors[0], nil
}

func (e *DeepSeekEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := map[string]any{
		"model": e.model,
		"input": texts,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("deepseek embed marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		deepseekEmbedBaseURL+"/embeddings", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("deepseek embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek embed API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("deepseek embed read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek embed API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("deepseek embed unmarshal: %w", err)
	}

	vectors := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		vectors[i] = d.Embedding
	}

	return vectors, nil
}
