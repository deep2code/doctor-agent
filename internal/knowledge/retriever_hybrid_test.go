package knowledge

import (
	"context"
	"sync"
	"testing"
)

// mapRetriever returns preset entry IDs for exact query strings; queries not
// in the map return nothing. It stands in for both keyword and vector
// retrievers in hybrid-fusion tests.
type mapRetriever struct {
	exact map[string][]string
}

func (m *mapRetriever) Retrieve(_ context.Context, query string, _ int) ([]RetrievalResult, error) {
	ids, ok := m.exact[query]
	if !ok {
		return nil, nil
	}
	out := make([]RetrievalResult, 0, len(ids))
	for _, id := range ids {
		out = append(out, RetrievalResult{Entry: KnowledgeEntry{ID: id}})
	}
	return out, nil
}

func (m *mapRetriever) RetrieveDrugs(_ context.Context, _ string, _ int) ([]DrugRetrievalResult, error) {
	return nil, nil
}

func (m *mapRetriever) Name() string { return "map" }

// TestHybridFusesExpandedQueryLists verifies the colloquial-recall path: a
// query whose verbatim form hits one entry and whose synonym-expanded form
// hits another must recall BOTH, with verbatim-only entries ranked higher
// (full weight beats decayed expanded weight).
func TestHybridFusesExpandedQueryLists(t *testing.T) {
	query := "孩子拉肚子"
	expanded := ExpandQuery(query)
	if expanded == query {
		t.Fatal("内置同义词组应命中，否则测试前提不成立")
	}

	mr := &mapRetriever{exact: map[string][]string{
		query:    {"verbatim-only", "both"},
		expanded: {"both", "expanded-only"},
	}}
	h := NewHybridRetriever(mr, mr, 0.5)
	res, err := h.Retrieve(context.Background(), query, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}

	ids := map[string]int{}
	for i, r := range res {
		ids[r.Entry.ID] = i
	}
	for _, want := range []string{"verbatim-only", "expanded-only", "both"} {
		if _, ok := ids[want]; !ok {
			t.Errorf("融合结果缺少条目 %q，实际: %v", want, ids)
		}
	}
	// "both" appears in all three lists (verbatim kw rank 2, verbatim vec
	// rank 2, expanded kw rank 1) so its accumulated RRF score tops any
	// single-list entry — expected RRF behaviour: entries matching more
	// query variants rank higher.
	if len(res) > 1 && res[0].Entry.ID != "both" {
		t.Errorf("命中最多查询变体的条目应排第一，实际第一: %q", res[0].Entry.ID)
	}
}

// spyRetriever records every query text it receives.
type spyRetriever struct {
	inner Retriever
	mu    sync.Mutex
	seen  []string
}

func (s *spyRetriever) Retrieve(ctx context.Context, query string, topK int) ([]RetrievalResult, error) {
	s.mu.Lock()
	s.seen = append(s.seen, query)
	s.mu.Unlock()
	return s.inner.Retrieve(ctx, query, topK)
}

func (s *spyRetriever) RetrieveDrugs(ctx context.Context, query string, topK int) ([]DrugRetrievalResult, error) {
	return s.inner.RetrieveDrugs(ctx, query, topK)
}

func (s *spyRetriever) Name() string { return "spy" }

func (s *spyRetriever) seenQueries() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.seen...)
}

// TestHybridExpandedQueryUsesKeywordLegOnly: the synonym-expanded string is
// a keyword-leg recall aid only — sending it through the vector leg too
// would pay a second embedding round-trip per Retrieve call.
func TestHybridExpandedQueryUsesKeywordLegOnly(t *testing.T) {
	query := "孩子拉肚子"
	expanded := ExpandQuery(query)
	if expanded == query {
		t.Fatal("内置同义词组应命中，否则测试前提不成立")
	}
	kw := &spyRetriever{inner: &mapRetriever{exact: map[string][]string{
		query:    {"verbatim-hit"},
		expanded: {"expanded-hit"},
	}}}
	vec := &spyRetriever{inner: &mapRetriever{exact: map[string][]string{
		query: {"verbatim-hit"},
	}}}
	h := NewHybridRetriever(kw, vec, 0.4)
	if _, err := h.Retrieve(context.Background(), query, 5); err != nil {
		t.Fatalf("Retrieve: %v", err)
	}

	kwSeen := kw.seenQueries()
	if len(kwSeen) != 2 || kwSeen[0] != query || kwSeen[1] != expanded {
		t.Errorf("关键词腿应收到原词+扩词两条查询，实际: %v", kwSeen)
	}
	vecSeen := vec.seenQueries()
	if len(vecSeen) != 1 || vecSeen[0] != query {
		t.Errorf("向量腿只应收到原词查询，实际: %v", vecSeen)
	}
}

// TestHybridUnchangedQuerySkipsExpandedLists verifies no extra retriever
// calls happen when the query has no synonym hits (expanded == query).
func TestHybridUnchangedQuerySkipsExpandedLists(t *testing.T) {
	query := "no synonym match"
	mr := &mapRetriever{exact: map[string][]string{
		query: {"a", "b"},
	}}
	h := NewHybridRetriever(mr, mr, 0.5)
	res, err := h.Retrieve(context.Background(), query, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(res) != 2 {
		t.Errorf("expected 2 results, got %d", len(res))
	}
}
