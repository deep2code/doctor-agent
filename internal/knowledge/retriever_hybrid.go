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

// fetchResult carries one retriever call's outcome.
type fetchResult struct {
	results []RetrievalResult
	err     error
}

// retrieveAsync runs one retriever in a goroutine.
func (r *HybridRetriever) retrieveAsync(ctx context.Context, retriever Retriever, query string, topK int) <-chan fetchResult {
	ch := make(chan fetchResult, 1)
	go func() {
		res, err := retriever.Retrieve(ctx, query, topK)
		ch <- fetchResult{results: res, err: err}
	}()
	return ch
}

// Retrieve performs hybrid retrieval with RRF fusion.
//
// Four-way recall for colloquial queries: the query runs through keyword and
// vector retrieval verbatim, and — when ExpandQuery maps it to a different
// string — also through both retrievers with the synonym-expanded query.
// Colloquial input ("突然大哭") often shares no surface form with indexed
// clinical terms ("哭闹"), so the expanded lists recall what the verbatim
// lists miss. Expanded lists enter the fusion at half weight: verbatim
// matches keep priority over synonym-only matches.
func (r *HybridRetriever) Retrieve(ctx context.Context, query string, topK int) ([]RetrievalResult, error) {
	if topK <= 0 {
		topK = 5
	}

	// Fetch more from each source to ensure good coverage after fusion
	fetchK := topK * 3

	kwCh := r.retrieveAsync(ctx, r.keywordRetriever, query, fetchK)
	vecCh := r.retrieveAsync(ctx, r.vectorRetriever, query, fetchK)

	expanded := ExpandQuery(query)
	hasExpanded := expanded != query
	var expKwCh, expVecCh <-chan fetchResult
	if hasExpanded {
		expKwCh = r.retrieveAsync(ctx, r.keywordRetriever, expanded, fetchK)
		expVecCh = r.retrieveAsync(ctx, r.vectorRetriever, expanded, fetchK)
	}

	kwResult := <-kwCh
	vecResult := <-vecCh

	// Handle vector retrieval errors gracefully — fall back to keyword only
	if vecResult.err != nil {
		if kwResult.err != nil {
			return nil, kwResult.err
		}
		out := kwResult.results
		if len(out) > topK {
			out = out[:topK]
		}
		return out, nil
	}
	if kwResult.err != nil {
		out := vecResult.results
		if len(out) > topK {
			out = out[:topK]
		}
		return out, nil
	}

	// RRF fusion. Expanded-query errors are non-fatal: those lists only add
	// recall, so on failure they are simply omitted.
	const expandDecay = 0.5
	sources := []rankSource{
		{results: kwResult.results, weight: 1 - r.vectorWeight},
		{results: vecResult.results, weight: r.vectorWeight},
	}
	if hasExpanded {
		expKw := <-expKwCh
		expVec := <-expVecCh
		if expKw.err == nil && len(expKw.results) > 0 {
			sources = append(sources, rankSource{results: expKw.results, weight: (1 - r.vectorWeight) * expandDecay})
		}
		if expVec.err == nil && len(expVec.results) > 0 {
			sources = append(sources, rankSource{results: expVec.results, weight: r.vectorWeight * expandDecay})
		}
	}

	fused := rrfFuse(sources)

	if len(fused) > topK {
		fused = fused[:topK]
	}

	return fused, nil
}

// RetrieveDrugs delegates to the keyword retriever (drug lookup needs exact matching).
func (r *HybridRetriever) RetrieveDrugs(ctx context.Context, query string, topK int) ([]DrugRetrievalResult, error) {
	return r.keywordRetriever.RetrieveDrugs(ctx, query, topK)
}

// rankSource is one ranked list entering the RRF fusion, with the total
// fusion weight of that list.
type rankSource struct {
	results []RetrievalResult
	weight  float64
}

// rrfFuse implements Reciprocal Rank Fusion across N ranked lists.
// Reference: Cormack et al. "Reciprocal Rank Fusion outperforms Condorcet and
// individual rank learning methods" (SIGIR 2009).
func rrfFuse(sources []rankSource) []RetrievalResult {
	const k = 60.0 // RRF constant

	entryMap := make(map[string]*fusedData)

	for _, src := range sources {
		for i, res := range src.results {
			id := res.Entry.ID
			fd, ok := entryMap[id]
			if !ok {
				fd = &fusedData{entry: res.Entry}
				entryMap[id] = fd
			}
			fd.score += src.weight / (k + float64(i+1)) // 1-indexed rank
		}
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

// fusedData accumulates the RRF score for one entry across lists.
type fusedData struct {
	entry KnowledgeEntry
	score float64
}
