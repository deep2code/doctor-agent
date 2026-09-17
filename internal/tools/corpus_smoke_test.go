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

// TestTryICDCode covers the exact-code auto-dispatch: ICD-10/ICD-11 shaped
// queries hit the classification stores; free text never hijacks the switch.
func TestTryICDCode(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ks := NewKnowledgeSearch(store, nil)

	res, handled := ks.tryICDCode("1A00")
	if !handled {
		t.Fatalf("1A00 应被识别为 ICD 码")
	}
	b, _ := json.Marshal(res.Data)
	if !containsFold(string(b), "霍乱") {
		t.Errorf("1A00 应命中霍乱: %s", b)
	}

	res, handled = ks.tryICDCode("j45.9")
	if !handled {
		t.Fatalf("J45.9（小写）应被识别为 ICD-10 码")
	}
	b, _ = json.Marshal(res.Data)
	// 国家临床版为 6 位扩展码：J45.9 前缀命中 J45.900（哮喘）
	if !containsFold(string(b), "J45.900") || !containsFold(string(b), "哮喘") {
		t.Errorf("J45.9 应前缀命中 J45.900 哮喘: %s", b)
	}

	if _, handled := ks.tryICDCode("G6PD"); handled {
		t.Errorf("G6PD 不是 ICD 码，不应拦截")
	}
	if _, handled := ks.tryICDCode("哮喘"); handled {
		t.Errorf("中文词不应被拦截")
	}
	if _, handled := ks.tryICDCode("asthma"); handled {
		t.Errorf("英文词不应被拦截")
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
