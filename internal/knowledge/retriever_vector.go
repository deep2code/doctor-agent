package knowledge

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// VectorRetriever performs semantic search using embeddings.
type VectorRetriever struct {
	store     *VectorStore
	embedder  Embedder
	storeData *Store
}

// Embedder is the interface for text embedding.
type Embedder interface {
	Embed(text string) ([]float32, error)
	Dimensions() int
}

// NewVectorRetriever creates a new vector retriever.
func NewVectorRetriever(store *VectorStore, embedder Embedder, storeData *Store) *VectorRetriever {
	return &VectorRetriever{
		store:     store,
		embedder:  embedder,
		storeData: storeData,
	}
}

// Retrieve performs semantic search and returns matching knowledge entries.
func (r *VectorRetriever) Retrieve(ctx context.Context, query string, topK int) ([]RetrievalResult, error) {
	if topK <= 0 {
		topK = 5
	}

	// Embed the query
	queryVector, err := r.embedder.Embed(query)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	// Search vector store
	results, err := r.store.Search(ctx, SearchQuery{
		Vector:    queryVector,
		TopK:      topK * 2, // Fetch more to filter
		Threshold: 0.4,     // Minimum similarity
	})
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	// Convert to RetrievalResult
	var retrievalResults []RetrievalResult
	for _, result := range results {
		// Get entry ID from payload
		entryID, ok := result.Payload["entry_id"]
		if !ok {
			continue
		}

		// Look up the actual knowledge entry
		entry, exists := r.storeData.MedicalByID[entryID]
		if !exists {
			continue
		}

		retrievalResults = append(retrievalResults, RetrievalResult{
			Entry: *entry,
			Score: result.Score,
		})

		if len(retrievalResults) >= topK {
			break
		}
	}

	return retrievalResults, nil
}

// RetrieveDrugs performs semantic search on drug entries.
func (r *VectorRetriever) RetrieveDrugs(ctx context.Context, query string, topK int) ([]DrugRetrievalResult, error) {
	if topK <= 0 {
		topK = 5
	}

	// Embed the query
	queryVector, err := r.embedder.Embed(query)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	// Search vector store with drug type filter
	results, err := r.store.Search(ctx, SearchQuery{
		Vector:    queryVector,
		TopK:      topK * 2,
		Threshold: 0.4,
		Filter:    map[string]string{"type": "drug"},
	})
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	// Convert to DrugRetrievalResult
	var retrievalResults []DrugRetrievalResult
	for _, result := range results {
		entryID, ok := result.Payload["entry_id"]
		if !ok {
			continue
		}

		entry, exists := r.storeData.DrugByID[entryID]
		if !exists {
			continue
		}

		retrievalResults = append(retrievalResults, DrugRetrievalResult{
			Entry: *entry,
			Score: result.Score,
		})

		if len(retrievalResults) >= topK {
			break
		}
	}

	return retrievalResults, nil
}

// Name returns the retriever name.
func (r *VectorRetriever) Name() string {
	return "vector"
}

// IndexKnowledgeEntry indexes a knowledge entry into the vector store.
func (r *VectorRetriever) IndexKnowledgeEntry(ctx context.Context, entry KnowledgeEntry) error {
	// Build text for embedding
	text := r.buildEntryText(entry)

	// Embed
	vector, err := r.embedder.Embed(text)
	if err != nil {
		return fmt.Errorf("embedding entry: %w", err)
	}

	// Build payload
	payload := map[string]string{
		"entry_id":   entry.ID,
		"type":       "knowledge",
		"condition":  entry.ConditionZH,
		"category":   entry.Category,
		"icd10":      entry.ICD10,
	}

	// Index
	return r.store.Upsert(ctx, []VectorPoint{
		{
			ID:      fmt.Sprintf("k-%s", entry.ID),
			Vector:  vector,
			Payload: payload,
		},
	})
}

// IndexDrugEntry indexes a drug entry into the vector store.
func (r *VectorRetriever) IndexDrugEntry(ctx context.Context, entry DrugEntry) error {
	// Build text for embedding
	text := r.buildDrugText(entry)

	// Embed
	vector, err := r.embedder.Embed(text)
	if err != nil {
		return fmt.Errorf("embedding drug: %w", err)
	}

	// Build payload
	payload := map[string]string{
		"entry_id": entry.ID,
		"type":     "drug",
		"name_en":  entry.GenericNameEN,
		"name_zh":  entry.GenericNameZH,
		"class":    entry.DrugClass,
	}

	// Index
	return r.store.Upsert(ctx, []VectorPoint{
		{
			ID:      fmt.Sprintf("d-%s", entry.ID),
			Vector:  vector,
			Payload: payload,
		},
	})
}

// IndexAllKnowledge indexes all knowledge entries.
func (r *VectorRetriever) IndexAllKnowledge(ctx context.Context) error {
	slog.Info("Starting knowledge indexing", "entries", len(r.storeData.MedicalEntries), "drugs", len(r.storeData.DrugEntries))

	// Index knowledge entries
	for i, entry := range r.storeData.MedicalEntries {
		if err := r.IndexKnowledgeEntry(ctx, entry); err != nil {
			slog.Warn("Failed to index entry", "id", entry.ID, "error", err)
			continue
		}
		if (i+1)%100 == 0 {
			slog.Info("Indexed knowledge entries", "count", i+1)
		}
	}

	// Index drug entries
	for i, entry := range r.storeData.DrugEntries {
		if err := r.IndexDrugEntry(ctx, entry); err != nil {
			slog.Warn("Failed to index drug", "id", entry.ID, "error", err)
			continue
		}
		if (i+1)%100 == 0 {
			slog.Info("Indexed drug entries", "count", i+1)
		}
	}

	slog.Info("Knowledge indexing completed")
	return nil
}

// buildEntryText builds the text to embed for a knowledge entry.
func (r *VectorRetriever) buildEntryText(entry KnowledgeEntry) string {
	var sb strings.Builder
	sb.WriteString(entry.ConditionZH)
	if entry.ConditionEN != "" {
		sb.WriteString(" ")
		sb.WriteString(entry.ConditionEN)
	}
	if entry.ICD10 != "" {
		sb.WriteString(" ICD-10:")
		sb.WriteString(entry.ICD10)
	}
	if len(entry.Keywords) > 0 {
		sb.WriteString(" ")
		sb.WriteString(strings.Join(entry.Keywords, " "))
	}
	if len(entry.Treatment) > 0 {
		sb.WriteString(" 治疗:")
		for _, t := range entry.Treatment {
			sb.WriteString(t.Method)
			sb.WriteString(" ")
		}
	}
	return sb.String()
}

// buildDrugText builds the text to embed for a drug entry.
func (r *VectorRetriever) buildDrugText(entry DrugEntry) string {
	var sb strings.Builder
	sb.WriteString(entry.GenericNameZH)
	sb.WriteString(" ")
	sb.WriteString(entry.GenericNameEN)
	if len(entry.TradeNames) > 0 {
		sb.WriteString(" ")
		sb.WriteString(strings.Join(entry.TradeNames, " "))
	}
	if entry.DrugClass != "" {
		sb.WriteString(" ")
		sb.WriteString(entry.DrugClass)
	}
	return sb.String()
}
