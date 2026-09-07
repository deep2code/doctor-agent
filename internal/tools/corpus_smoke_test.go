package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/doctor-agent/internal/knowledge"
)

// TestCorpusToolSmoke exercises knowledge_search (medgen/lactmed datasets) and
// exact_lookup (icd11) end-to-end against the seeded database. Skips when the
// new datasets have not been seeded yet.
func TestCorpusToolSmoke(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.GetCorpusDocCount() == 0 {
		t.Skip("corpus 数据未 seed")
	}

	ks := NewKnowledgeSearch(store, nil)
	for _, tc := range []struct{ dataset, query, want string }{
		{"medgen", "allergic asthma", "Allergic asthma"},
		{"lactmed", "哺乳期布洛芬", "Ibuprofen"},
	} {
		res, err := ks.Execute(context.Background(), map[string]any{
			"query": tc.query, "dataset": tc.dataset, "top_k": float64(3),
		})
		if err != nil || !res.Success {
			t.Fatalf("%s/%s: err=%v res=%+v", tc.dataset, tc.query, err, res)
		}
		b, _ := json.Marshal(res.Data)
		if !containsFold(string(b), tc.want) {
			t.Errorf("%s/%s: 结果不含 %q: %s", tc.dataset, tc.query, tc.want, b)
		}
	}

	el := NewExactLookup(store)
	res, err := el.Execute(context.Background(), map[string]any{"query": "霍乱", "type": "icd11"})
	if err != nil || !res.Success {
		t.Fatalf("icd11 霍乱: err=%v res=%+v", err, res)
	}
	b, _ := json.Marshal(res.Data)
	if !containsFold(string(b), "1A00") {
		t.Errorf("icd11 霍乱应返回 1A00: %s", b)
	}
}

func containsFold(s, sub string) bool {
	n := len(sub)
	if n == 0 {
		return true
	}
	for i := 0; i+n <= len(s); i++ {
		if eqFold(s[i:i+n], sub) {
			return true
		}
	}
	return false
}

func eqFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
