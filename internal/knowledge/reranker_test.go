package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// scriptedReranker returns preset scores per call and records the docs it saw.
type scriptedReranker struct {
	scores []float64
	err    error
	calls  int
	lastQ  string
	lastN  int
}

func (r *scriptedReranker) Rerank(_ context.Context, query string, docs []string) ([]float64, error) {
	r.calls++
	r.lastQ = query
	r.lastN = len(docs)
	if r.err != nil {
		return nil, r.err
	}
	return r.scores, nil
}

func resultsWith(ids ...string) []RetrievalResult {
	out := make([]RetrievalResult, 0, len(ids))
	for i, id := range ids {
		out = append(out, RetrievalResult{
			Score: float64(100 - i), // descending RRF order
			Entry: KnowledgeEntry{ID: id, ConditionZH: id},
		})
	}
	return out
}

// TestRerankCandidatesReordersAndTruncates: scores promote the last candidate
// to first, and the output is capped at topK.
func TestRerankCandidatesReordersAndTruncates(t *testing.T) {
	in := resultsWith("a", "b", "c")
	// scores: a=0.1 b=0.2 c=0.9 → order c,b,a
	rr := &scriptedReranker{scores: []float64{0.1, 0.2, 0.9}}
	out := RerankCandidates(context.Background(), rr, "乳糖不耐受", in, 2)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2 (topK)", len(out))
	}
	if out[0].Entry.ID != "c" || out[1].Entry.ID != "b" {
		t.Errorf("order = %s,%s, want c,b", out[0].Entry.ID, out[1].Entry.ID)
	}
	if rr.calls != 1 || rr.lastQ != "乳糖不耐受" || rr.lastN != 3 {
		t.Errorf("rerank call = %d queries=%q docs=%d", rr.calls, rr.lastQ, rr.lastN)
	}
}

// TestRerankCandidatesNilKeepsPoolUntouched: with no reranker the input is
// returned verbatim (NOT truncated) — disabling rerank must not shrink the
// prompt's existing 2×topK fusion cap.
func TestRerankCandidatesNilKeepsPoolUntouched(t *testing.T) {
	in := resultsWith("a", "b", "c", "d", "e")
	out := RerankCandidates(context.Background(), nil, "q", in, 2)
	if len(out) != 5 {
		t.Errorf("nil reranker must not truncate: len = %d, want 5", len(out))
	}
}

// TestRerankCandidatesDegradesOnError: any reranker failure keeps the
// original RRF order (still truncated to topK, which is the caller's target).
func TestRerankCandidatesDegradesOnError(t *testing.T) {
	in := resultsWith("a", "b", "c")
	rr := &scriptedReranker{err: errors.New("boom")}
	out := RerankCandidates(context.Background(), rr, "q", in, 2)
	if len(out) != 2 || out[0].Entry.ID != "a" || out[1].Entry.ID != "b" {
		t.Errorf("expected RRF order a,b on error, got %+v", out)
	}
}

// TestRerankCandidatesMismatchedScoresFallBack: a score slice whose length
// does not match the docs is rejected (keeps RRF order rather than a partial,
// possibly misaligned ranking).
func TestRerankCandidatesMismatchedScoresFallBack(t *testing.T) {
	in := resultsWith("a", "b", "c")
	rr := &scriptedReranker{scores: []float64{0.5}} // wrong length
	out := RerankCandidates(context.Background(), rr, "q", in, 3)
	if out[0].Entry.ID != "a" || out[2].Entry.ID != "c" {
		t.Errorf("mismatched scores should fall back to RRF order, got %+v", out)
	}
}

// TestRerankDocTextPrioritizesTitleKeywords: the passage fed to the
// cross-encoder starts with the condition name and keywords, and includes the
// Body excerpt when present.
func TestRerankDocTextPrioritizesTitleKeywords(t *testing.T) {
	entry := KnowledgeEntry{
		ConditionZH: "乳糖不耐受",
		Keywords:    []string{"腹泻", "胀气"},
		Body:        strings.Repeat("肠道缺乏乳糖酶。", 200),
	}
	doc := rerankDocText(entry)
	if !strings.HasPrefix(doc, "乳糖不耐受") {
		t.Errorf("doc should begin with condition name, got %q", doc[:min(20, len(doc))])
	}
	if !strings.Contains(doc, "腹泻") || !strings.Contains(doc, "胀气") {
		t.Error("doc should carry keywords")
	}
	if len([]rune(doc)) > rerankDocBudget {
		t.Errorf("doc exceeds budget: %d runes", len([]rune(doc)))
	}
}
