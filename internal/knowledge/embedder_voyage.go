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

const voyageBaseURL = "https://api.voyageai.com/v1"

// VoyageEmbedder uses Voyage AI's embedding API (Anthropic's recommended embedding provider).
// Model options: voyage-multilingual-2 (1024d), voyage-3-large (1024d), voyage-3 (1024d)
type VoyageEmbedder struct {
	apiKey     string
	model      string
	dimensions int
	httpClient *http.Client
}

// NewVoyageEmbedder creates a Voyage AI embedder.
func NewVoyageEmbedder(apiKey, model string) *VoyageEmbedder {
	dim := 1024
	switch model {
	case "voyage-3-large":
		dim = 1024
	case "voyage-multilingual-2":
		dim = 1024
	case "voyage-3":
		dim = 1024
	case "voyage-3-lite":
		dim = 512
	}

	return &VoyageEmbedder{
		apiKey:     apiKey,
		model:      model,
		dimensions: dim,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *VoyageEmbedder) Name() string {
	return fmt.Sprintf("Voyage AI (%s)", e.model)
}

func (e *VoyageEmbedder) Dimensions() int {
	return e.dimensions
}

func (e *VoyageEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("voyage: empty response")
	}
	return vectors[0], nil
}

func (e *VoyageEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := map[string]any{
		"model": e.model,
		"input": texts,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("voyage marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		voyageBaseURL+"/embeddings", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("voyage request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage API: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("voyage read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("voyage API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("voyage unmarshal: %w", err)
	}

	vectors := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		vectors[i] = d.Embedding
	}

	return vectors, nil
}
