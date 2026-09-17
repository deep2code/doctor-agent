package knowledge

import (
	"context"
	"strings"
	"testing"
)

func TestAapLoad(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(store.GetAAPEntries()) == 0 {
		t.Skip("aap_articles.json 未嵌入")
	}
	t.Logf("AAP articles: %d", len(store.GetAAPEntries()))
}

func TestRetrieveAAP(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	if len(store.GetAAPEntries()) == 0 {
		t.Skip("aap_articles.json 未嵌入")
	}
	cases := []struct{ q, expect string }{
		{"newborn bowel movements", "Bowel Movements"},
		{"temperament", "Temperament"},
		{"cognitive development 4 months", "Cognitive Development"},
	}
	for _, c := range cases {
		res, err := r.RetrieveAAP(context.Background(), c.q, 3)
		if err != nil {
			t.Fatalf("query %q: %v", c.q, err)
		}
		if len(res) == 0 {
			t.Errorf("query %q: 无结果", c.q)
			continue
		}
		if !strings.Contains(res[0].Entry.Title, c.expect) {
			t.Errorf("query %q: 期望标题含 %q，得到 %q", c.q, c.expect, res[0].Entry.Title)
		}
		t.Logf("query %q -> %s (%.0f)", c.q, res[0].Entry.Title, res[0].Score)
	}
}
