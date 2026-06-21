package knowledge

import (
	"context"
	"fmt"
)

// VectorRetriever performs semantic search using embeddings + Qdrant vector store.
type VectorRetriever struct {
	embedder Embedder
	store    *QdrantStore
}

// NewVectorRetriever creates a vector-based retriever.
func NewVectorRetriever(embedder Embedder, store *QdrantStore) *VectorRetriever {
	return &VectorRetriever{
		embedder: embedder,
		store:    store,
	}
}

func (r *VectorRetriever) Name() string {
	return fmt.Sprintf("vector (%s)", r.embedder.Name())
}

// Retrieve performs semantic search over the vector store.
func (r *VectorRetriever) Retrieve(ctx context.Context, query string, topK int) ([]RetrievalResult, error) {
	if topK <= 0 {
		topK = 5
	}

	// Embed the query
	queryVector, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("vector retrieve embed: %w", err)
	}

	// Search Qdrant
	results, err := r.store.Search(ctx, queryVector, topK*2) // Get more for filtering
	if err != nil {
		return nil, fmt.Errorf("vector retrieve search: %w", err)
	}

	// Convert to RetrievalResult
	retrievalResults := make([]RetrievalResult, 0, len(results))
	for _, sr := range results {
		entry := searchResultToEntry(sr)
		retrievalResults = append(retrievalResults, RetrievalResult{
			Entry:  entry,
			Score:  float64(sr.Score),
		})
	}

	if len(retrievalResults) > topK {
		retrievalResults = retrievalResults[:topK]
	}

	return retrievalResults, nil
}

// RetrieveDrugs is not currently supported by vector retriever.
// Falls back to an empty result set.
func (r *VectorRetriever) RetrieveDrugs(ctx context.Context, query string, topK int) ([]DrugRetrievalResult, error) {
	// Drug lookup requires exact matching; vector search is less suitable.
	// The hybrid retriever handles this by delegating to keyword retriever.
	return nil, nil
}

// searchResultToEntry converts a Qdrant search result payload to a KnowledgeEntry.
func searchResultToEntry(sr SearchResult) KnowledgeEntry {
	entry := KnowledgeEntry{
		ID: sr.ID,
	}

	if title, ok := sr.Payload["condition_zh"].(string); ok {
		entry.ConditionZH = title
	}
	if titleEN, ok := sr.Payload["condition_en"].(string); ok {
		entry.ConditionEN = titleEN
	}
	if cat, ok := sr.Payload["category"].(string); ok {
		entry.Category = cat
	}
	if icd10, ok := sr.Payload["icd10"].(string); ok {
		entry.ICD10 = icd10
	}

	// Extract keywords
	if kw, ok := sr.Payload["keywords"].([]any); ok {
		for _, k := range kw {
			if s, ok := k.(string); ok {
				entry.Keywords = append(entry.Keywords, s)
			}
		}
	}

	// Extract citation
	if doi, ok := sr.Payload["doi"].(string); ok && doi != "" {
		entry.Citations = append(entry.Citations, Citation{
			DOI:   doi,
			Title: sr.Payload["title"].(string),
			Year:  int(sr.Payload["year"].(float64)),
			Level: sr.Payload["level"].(string),
		})
	}

	return entry
}
