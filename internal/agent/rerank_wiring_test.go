package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/doctor-agent/internal/knowledge"
)

// poolRetriever returns k synthetic candidates and records the requested k
// per call (to assert the rerank over-fetch contract).
type poolRetriever struct {
	mu sync.Mutex
	ks []int
}

func (p *poolRetriever) Retrieve(_ context.Context, _ string, k int) ([]knowledge.RetrievalResult, error) {
	p.mu.Lock()
	p.ks = append(p.ks, k)
	p.mu.Unlock()
	out := make([]knowledge.RetrievalResult, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, knowledge.RetrievalResult{
			Score: float64(100 - i),
			Entry: knowledge.KnowledgeEntry{ID: fmt.Sprintf("e%d", i)},
		})
	}
	return out, nil
}

func (p *poolRetriever) RetrieveDrugs(_ context.Context, _ string, _ int) ([]knowledge.DrugRetrievalResult, error) {
	return nil, nil
}

func (p *poolRetriever) Name() string { return "pool" }

func (p *poolRetriever) requested() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int(nil), p.ks...)
}

// fakeReranker scores candidate i via scoreFor (position → score).
type fakeReranker struct {
	calls    int
	scoreFor func(i int) float64
}

func (f *fakeReranker) Rerank(_ context.Context, _ string, docs []string) ([]float64, error) {
	f.calls++
	out := make([]float64, len(docs))
	for i := range out {
		out[i] = f.scoreFor(i)
	}
	return out, nil
}

// TestRetrieveWithUnderstandingReranksOverfetchedPool: with a reranker the
// retriever is asked for 2×topK candidates, the cross-encoder runs once on
// the merged pool, and the result is reordered and truncated to topK.
func TestRetrieveWithUnderstandingReranksOverfetchedPool(t *testing.T) {
	cfg := testConfig()
	cfg.KnowledgeTopK = 2
	pr := &poolRetriever{}
	rr := &fakeReranker{scoreFor: func(i int) float64 {
		switch i {
		case 1:
			return 0.9 // e1 (RRF rank 2) promoted to first
		case 2:
			return 0.5 // e2 second
		}
		return 0.1
	}}
	ag := newTestAgent(cfg, nil)
	ag.retriever = pr
	ag.reranker = rr

	out := ag.retrieveWithUnderstanding(context.Background(), "问题", func(StepEvent) {})

	if got := pr.requested(); len(got) != 1 || got[0] != 4 {
		t.Errorf("retriever asked for %v, want [4] (2×topK)", got)
	}
	if rr.calls != 1 {
		t.Errorf("rerank calls = %d, want 1", rr.calls)
	}
	if len(out) != 2 || out[0].Entry.ID != "e1" || out[1].Entry.ID != "e2" {
		t.Errorf("final list = %+v, want [e1 e2]", out)
	}
}

// TestRetrieveWithUnderstandingNoOverfetchWithoutReranker: with rerank off
// (default) fetch size and result length are exactly the pre-existing
// behaviour — no over-fetch, no truncation.
func TestRetrieveWithUnderstandingNoOverfetchWithoutReranker(t *testing.T) {
	cfg := testConfig()
	cfg.KnowledgeTopK = 3
	pr := &poolRetriever{}
	ag := newTestAgent(cfg, nil)
	ag.retriever = pr

	out := ag.retrieveWithUnderstanding(context.Background(), "问题", func(StepEvent) {})

	if got := pr.requested(); len(got) != 1 || got[0] != 3 {
		t.Errorf("retriever asked for %v, want [3]", got)
	}
	if len(out) != 3 || out[0].Entry.ID != "e0" {
		t.Errorf("final list = %+v, want untruncated RRF order", out)
	}
}
