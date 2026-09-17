package knowledge

import (
	"context"
	"testing"
)

func TestMSDLoad(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.GetMSDCount() == 0 {
		t.Skip("msd_manual.json 未嵌入")
	}
	t.Logf("MSD pages: %d", store.GetMSDCount())
}

func TestRetrieveMSD(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	if store.GetMSDCount() == 0 {
		t.Skip("msd_manual.json 未嵌入")
	}
	cases := []struct {
		query  string
		expect string // expected title substring in top result
	}{
		{"地中海贫血", "地中海贫血"},
		{"荨麻疹", "荨麻疹"},
		{"G6PD", "G6PD"},
		{"低血糖", "低血糖"},
	}
	for _, c := range cases {
		res, err := r.RetrieveMSD(context.Background(), c.query, 3)
		if err != nil {
			t.Fatalf("query %q: %v", c.query, err)
		}
		if len(res) == 0 {
			t.Errorf("query %q: 无结果", c.query)
			continue
		}
		if res[0].Entry.Title == "" {
			t.Errorf("query %q: 结果缺标题", c.query)
		}
		t.Logf("query %q -> %s (score=%.1f)", c.query, res[0].Entry.Title, res[0].Score)
	}
}

func TestRetrieveMSDEveryday(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.GetMSDCount() == 0 {
		t.Skip("msd_manual.json 未嵌入")
	}
	// 调试"便秘怎么办"
	q := "便秘怎么办"
	windows := cjkWindows(q, 2, 6)
	t.Logf("query %q windows=%v", q, windows)
	for _, e := range store.GetMSDEntries() {
		if e.Title == "便秘" || e.Title == "欺凌" || e.Title == "成人便秘" {
			s, ok := scoreMSD(&e, q, windows, nil)
			t.Logf("「%s」(src=%s) score=%.1f ok=%v", e.Title, e.Source, s, ok)
		}
	}
}
