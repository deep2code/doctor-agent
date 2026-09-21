// Package rerank provides an HTTP cross-encoder rerank client (TEI-style
// /rerank endpoint, e.g. HuggingFace text-embeddings-inference or any
// server speaking the same JSON shape such as vLLM/Xinference with a
// results wrapper). It satisfies knowledge.Reranker.
package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Provider calls an external rerank service.
type Provider struct {
	baseURL string // service root; "/rerank" is appended
	apiKey  string // optional (TEI needs none)
	model   string
	client  *http.Client
}

// New creates a rerank provider. baseURL is required; model is informational
// for servers that multiplex models (TEI ignores it). The HTTP timeout caps
// the whole rerank hop: it sits on the first-token path, and the caller
// degrades to the pre-rerank order on any error.
func New(baseURL, apiKey, model string) (*Provider, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("rerank: base_url is required")
	}
	if model == "" {
		model = "bge-reranker-v2-m3"
	}
	return &Provider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 2 * time.Second},
	}, nil
}

// Name returns a human-readable identifier for logging.
func (p *Provider) Name() string {
	return fmt.Sprintf("rerank:%s", p.model)
}

type rerankRequest struct {
	Model  string   `json:"model"`
	Query  string   `json:"query"`
	Texts  []string `json:"texts"`
	Return bool     `json:"return_text"`
}

type rerankItem struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// rerankResponse absorbs both wire shapes seen in the wild: a bare TEI
// array, and Cohere/vLLM-style {"results":[...]}.
type rerankResponse struct {
	Results []rerankItem
}

func (r *rerankResponse) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return json.Unmarshal(trimmed, &r.Results)
	}
	var wrapper struct {
		Results []rerankItem `json:"results"`
	}
	if err := json.Unmarshal(trimmed, &wrapper); err != nil {
		return err
	}
	r.Results = wrapper.Results
	return nil
}

// Rerank scores docs against query and returns scores aligned by index.
func (p *Provider) Rerank(ctx context.Context, query string, docs []string) ([]float64, error) {
	if len(docs) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(rerankRequest{Model: p.model, Query: query, Texts: docs})
	if err != nil {
		return nil, fmt.Errorf("rerank: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/rerank", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("rerank: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank: request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("failed to close rerank response body", "error", err)
		}
	}()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("rerank: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rerank: API error (status %d): %s", resp.StatusCode, string(raw))
	}

	var parsed rerankResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("rerank: parse response: %w", err)
	}
	scores := make([]float64, len(docs))
	seen := 0
	for _, item := range parsed.Results {
		if item.Index >= 0 && item.Index < len(docs) {
			scores[item.Index] = item.RelevanceScore
			seen++
		}
	}
	if seen == 0 {
		return nil, fmt.Errorf("rerank: response carried no usable index for %d docs", len(docs))
	}
	return scores, nil
}
