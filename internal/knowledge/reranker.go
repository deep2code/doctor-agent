package knowledge

import (
	"context"
	"log/slog"
	"sort"
	"strings"
)

// Reranker rescores candidate documents against a query with a
// cross-encoder (e.g. bge-reranker). Rerank returns one score per input doc,
// positionally aligned with docs. Implemented by internal/rerank over HTTP;
// tests substitute a fake.
type Reranker interface {
	Rerank(ctx context.Context, query string, docs []string) ([]float64, error)
}

// rerankDocBudget caps the per-document text sent to the cross-encoder
// (the model window is ~512 tokens; server-side truncation is silent, so we
// do it explicitly to keep latency and payload size predictable).
const rerankDocBudget = 1000

// RerankCandidates rescores a fused candidate list with rr and returns the
// top topK in cross-encoder order. Any failure (service down, malformed
// response, ctx deadline) degrades silently to the incoming RRF order —
// rerank is a precision bonus on top of working retrieval, never a blocker.
//
// When rr is nil the input is returned untouched (NOT truncated): the caller
// keeps its existing fusion cap, so disabling rerank cannot silently shrink
// the prompt. Truncation to topK only happens on the reranked path.
func RerankCandidates(ctx context.Context, rr Reranker, query string, results []RetrievalResult, topK int) []RetrievalResult {
	truncate := func(in []RetrievalResult) []RetrievalResult {
		if topK > 0 && len(in) > topK {
			return in[:topK]
		}
		return in
	}
	if rr == nil {
		return results
	}
	if len(results) == 0 {
		return results
	}

	docs := make([]string, len(results))
	for i, r := range results {
		docs[i] = rerankDocText(r.Entry)
	}
	scores, err := rr.Rerank(ctx, query, docs)
	if err != nil {
		slog.Warn("Rerank failed; keeping RRF order", "error", err, "candidates", len(results))
		return truncate(results)
	}
	if len(scores) != len(results) {
		slog.Warn("Rerank returned mismatched score count; keeping RRF order",
			"scores", len(scores), "candidates", len(results))
		return truncate(results)
	}

	reranked := make([]RetrievalResult, len(results))
	copy(reranked, results)
	for i := range reranked {
		reranked[i].Score = scores[i]
	}
	sort.SliceStable(reranked, func(i, j int) bool {
		return reranked[i].Score > reranked[j].Score
	})
	return truncate(reranked)
}

// rerankDocText flattens an entry into the passage a cross-encoder scores.
// Title and keywords first (cheapest signal, always kept), then the richest
// prose available (Body excerpt, else structured clinical lists), truncated
// to rerankDocBudget runes.
func rerankDocText(entry KnowledgeEntry) string {
	var parts []string
	if entry.ConditionZH != "" {
		parts = append(parts, entry.ConditionZH)
	}
	if entry.ConditionEN != "" {
		parts = append(parts, entry.ConditionEN)
	}
	if len(entry.Keywords) > 0 {
		kw := entry.Keywords
		if len(kw) > 12 {
			kw = kw[:12]
		}
		parts = append(parts, strings.Join(kw, " "))
	}
	if entry.Body != "" {
		parts = append(parts, clipRunes(entry.Body, 600))
	} else {
		var prose []string
		for _, list := range [][]string{
			entry.DifferentialDiagnosis,
			entry.RiskFactors,
			entry.WhenToSeekCare,
			entry.Prevention,
			entry.Complications,
		} {
			prose = append(prose, list...)
		}
		for _, t := range entry.Treatment {
			prose = append(prose, t.Method)
		}
		if len(prose) > 0 {
			parts = append(parts, clipRunes(strings.Join(prose, "；"), 600))
		}
	}
	return clipRunes(strings.Join(parts, "\n"), rerankDocBudget)
}

func clipRunes(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}
