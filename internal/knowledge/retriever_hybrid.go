package knowledge

import (
	"context"
	"fmt"
	"sort"
)

// HybridRetriever combines keyword (BM25) and vector (semantic) retrieval
// using Reciprocal Rank Fusion (RRF) to produce a unified ranking.
type HybridRetriever struct {
	keywordRetriever Retriever
	vectorRetriever  Retriever
	vectorWeight     float64 // 0.0-1.0, weight of vector search in RRF fusion
}

// NewHybridRetriever creates a hybrid retriever combining keyword and vector search.
// vectorWeight controls the fusion: 0.0 = keyword only, 1.0 = vector only, 0.7 = recommended.
func NewHybridRetriever(keyword, vector Retriever, vectorWeight float64) *HybridRetriever {
	if vectorWeight < 0 {
		vectorWeight = 0
	}
	if vectorWeight > 1 {
		vectorWeight = 1
	}
	return &HybridRetriever{
		keywordRetriever: keyword,
		vectorRetriever:  vector,
		vectorWeight:     vectorWeight,
	}
}

func (r *HybridRetriever) Name() string {
	return fmt.Sprintf("hybrid (keyword=%s, vector=%s, weight=%.1f)",
		r.keywordRetriever.Name(), r.vectorRetriever.Name(), r.vectorWeight)
}

// Retrieve performs hybrid retrieval with RRF fusion.
func (r *HybridRetriever) Retrieve(ctx context.Context, query string, topK int) ([]RetrievalResult, error) {
	if topK <= 0 {
		topK = 5
	}

	// Fetch more from each source to ensure good coverage after fusion
	fetchK := topK * 3

	// Run keyword and vector retrieval concurrently
	type fetchResult struct {
		results []RetrievalResult
		err     error
	}

	kwCh := make(chan fetchResult, 1)
	vecCh := make(chan fetchResult, 1)

	go func() {
		res, err := r.keywordRetriever.Retrieve(ctx, query, fetchK)
		kwCh <- fetchResult{results: res, err: err}
	}()

	go func() {
		res, err := r.vectorRetriever.Retrieve(ctx, query, fetchK)
		vecCh <- fetchResult{results: res, err: err}
	}()

	kwResult := <-kwCh
	vecResult := <-vecCh

	// Handle vector retrieval errors gracefully — fall back to keyword only
	if vecResult.err != nil {
		if kwResult.err != nil {
			return nil, kwResult.err
		}
		return kwResult.results[:min(topK, len(kwResult.results))], nil
	}
	if kwResult.err != nil {
		return vecResult.results[:min(topK, len(vecResult.results))], nil
	}

	// RRF fusion
	fused := r.rrfFusion(kwResult.results, vecResult.results, r.vectorWeight)

	if len(fused) > topK {
		fused = fused[:topK]
	}

	return fused, nil
}

// RetrieveDrugs delegates to the keyword retriever (drug lookup needs exact matching).
func (r *HybridRetriever) RetrieveDrugs(ctx context.Context, query string, topK int) ([]DrugRetrievalResult, error) {
	return r.keywordRetriever.RetrieveDrugs(ctx, query, topK)
}

// rrfFusion implements Reciprocal Rank Fusion between two ranked result sets.
// Reference: Cormack et al. "Reciprocal Rank Fusion outperforms Condorcet and
// individual rank learning methods" (SIGIR 2009).
func (r *HybridRetriever) rrfFusion(kwResults, vecResults []RetrievalResult, vectorWeight float64) []RetrievalResult {
	const k = 60.0 // RRF constant

	// Track best score and merged data per entry ID
	type fusedData struct {
		entry  KnowledgeEntry
		score  float64
		kwRank int
		vecRank int
	}
	entryMap := make(map[string]*fusedData)

	// Process keyword results
	for i, res := range kwResults {
		id := res.Entry.ID
		if _, ok := entryMap[id]; !ok {
			entryMap[id] = &fusedData{entry: res.Entry, kwRank: -1, vecRank: -1}
		}
		entryMap[id].kwRank = i + 1 // 1-indexed rank
		rrfScore := (1 - vectorWeight) / (k + float64(i+1))
		entryMap[id].score += rrfScore
	}

	// Process vector results
	for i, res := range vecResults {
		id := res.Entry.ID
		if _, ok := entryMap[id]; !ok {
			entryMap[id] = &fusedData{entry: res.Entry, kwRank: -1, vecRank: -1}
		}
		entryMap[id].vecRank = i + 1
		rrfScore := vectorWeight / (k + float64(i+1))
		entryMap[id].score += rrfScore
	}

	// Convert to slice and sort
	fused := make([]RetrievalResult, 0, len(entryMap))
	for _, fd := range entryMap {
		fused = append(fused, RetrievalResult{
			Entry: fd.entry,
			Score: fd.score,
		})
	}

	sort.Slice(fused, func(i, j int) bool {
		return fused[i].Score > fused[j].Score
	})

	return fused
}
