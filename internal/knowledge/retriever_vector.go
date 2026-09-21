package knowledge

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// queryVectorCache memoises query-text → embedding. One user turn can send
// the same string through several retrieval legs (base + understanding
// branches + follow-up entity queries, each embedding its verbatim and
// expanded form), and every miss is a round-trip to the embedding service.
// Keyed by text only — the query-side model is fixed per process, so cached
// vectors can never straddle a model change.
type queryVectorCache struct {
	mu    sync.Mutex
	limit int
	order *list.List
	items map[string]*list.Element
}

type queryVectorItem struct {
	key    string
	vector []float32
}

const queryVectorCacheLimit = 256

func newQueryVectorCache(limit int) *queryVectorCache {
	return &queryVectorCache{limit: limit, order: list.New(), items: make(map[string]*list.Element)}
}

func (c *queryVectorCache) get(key string) ([]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*queryVectorItem).vector, true
	}
	return nil, false
}

func (c *queryVectorCache) put(key string, vector []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*queryVectorItem).vector = vector
		c.order.MoveToFront(el)
		return
	}
	c.items[key] = c.order.PushFront(&queryVectorItem{key: key, vector: vector})
	if c.order.Len() > c.limit {
		if oldest := c.order.Back(); oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*queryVectorItem).key)
		}
	}
}

// VectorRetriever performs semantic search using embeddings.
type VectorRetriever struct {
	store      *VectorStore
	embedder   Embedder
	storeData  *Store
	queryCache *queryVectorCache
}

// Embedder is the interface for text embedding.
type Embedder interface {
	Embed(text string) ([]float32, error)
	Dimensions() int
}

// BatchEmbedder is the optional batch form an embedding.Provider implements
// (embedding.OpenAICompatProvider always does). PrewarmQueries needs it;
// without it prewarming is a no-op and legs embed one query at a time.
type BatchEmbedder interface {
	EmbedBatch(texts []string) ([][]float32, error)
}

// NewVectorRetriever creates a new vector retriever.
func NewVectorRetriever(store *VectorStore, embedder Embedder, storeData *Store) *VectorRetriever {
	return &VectorRetriever{
		store:      store,
		embedder:   embedder,
		storeData:  storeData,
		queryCache: newQueryVectorCache(queryVectorCacheLimit),
	}
}

// embedQuery returns the vector for a retrieval query, memoised. Indexing
// paths (IndexKnowledgeEntry/IndexDrugEntry) embed entry text once each and
// must NOT go through here — only query strings belong in the cache.
func (r *VectorRetriever) embedQuery(query string) ([]float32, error) {
	if v, ok := r.queryCache.get(query); ok {
		return v, nil
	}
	v, err := r.embedder.Embed(query)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}
	r.queryCache.put(query, v)
	return v, nil
}

// PrewarmQueries resolves the vectors for a batch of upcoming query strings
// with a single EmbedBatch call, filling the query cache the legs will later
// read. Duplicate/already-cached queries are dropped; on provider failure
// this logs and returns — each leg then embeds its own query lazily, exactly
// as without prewarming.
func (r *VectorRetriever) PrewarmQueries(queries []string) {
	batch, ok := r.embedder.(BatchEmbedder)
	if !ok {
		return
	}
	var missing []string
	seen := make(map[string]bool, len(queries))
	for _, q := range queries {
		if q == "" || seen[q] {
			continue
		}
		seen[q] = true
		if _, cached := r.queryCache.get(q); cached {
			continue
		}
		missing = append(missing, q)
	}
	if len(missing) == 0 {
		return
	}
	vectors, err := batch.EmbedBatch(missing)
	if err != nil {
		slog.Warn("Query prewarm batch embedding failed; legs will embed individually",
			"count", len(missing), "error", err)
		return
	}
	for i, q := range missing {
		if i < len(vectors) && len(vectors[i]) > 0 {
			r.queryCache.put(q, vectors[i])
		}
	}
}

// Retrieve performs semantic search and returns matching knowledge entries.
func (r *VectorRetriever) Retrieve(ctx context.Context, query string, topK int) ([]RetrievalResult, error) {
	if topK <= 0 {
		topK = 5
	}

	queryVector, err := r.embedQuery(query)
	if err != nil {
		return nil, err
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
		// Self-contained mode (baked data image): the full entry JSON lives in
		// the payload, so retrieval works even when MariaDB only holds business
		// data. Fall back to the in-memory store for runtime-synced indexes.
		if raw, ok := result.Payload["data"]; ok && raw != "" {
			var e KnowledgeEntry
			if err := json.Unmarshal([]byte(raw), &e); err == nil && e.ID != "" {
				retrievalResults = append(retrievalResults, RetrievalResult{Entry: e, Score: result.Score})
				if len(retrievalResults) >= topK {
					break
				}
				continue
			}
		}

		// Get entry ID from payload
		entryID, ok := result.Payload["entry_id"]
		if !ok {
			continue
		}

		// Look up the actual knowledge entry (accessor holds the store lock
		// and triggers the lazy MariaDB load; raw map reads here would race
		// with a concurrent cold ingest).
		entry, exists := r.storeData.MedicalEntryByID(entryID)
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

	// Search query vector (memoised, shared with the knowledge path)
	queryVector, err := r.embedQuery(query)
	if err != nil {
		return nil, err
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
		// Self-contained mode: runtime-synced points carry the full drug JSON
		// in the payload, mirroring the knowledge path above (their entry_id
		// is a content-hash UUID that is not present in the in-memory store).
		if raw, ok := result.Payload["data"]; ok && raw != "" {
			var d DrugEntry
			if err := json.Unmarshal([]byte(raw), &d); err == nil && d.ID != "" {
				retrievalResults = append(retrievalResults, DrugRetrievalResult{Entry: d, Score: result.Score})
				if len(retrievalResults) >= topK {
					break
				}
				continue
			}
		}

		entryID, ok := result.Payload["entry_id"]
		if !ok {
			continue
		}

		entry, exists := r.storeData.DrugEntryByID(entryID)
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
	r.storeData.ensureMedical()
	r.storeData.ensureDrug()
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
