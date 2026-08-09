package knowledge

import (
	"context"
	"strings"
	"testing"
)

func TestFhsLoad(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(store.GetFHSGuides()) == 0 {
		t.Skip("fhs_guides.json 未嵌入")
	}
	t.Logf("FHS pages: %d", len(store.GetFHSGuides()))
}

func TestRetrieveFHSGuide(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	if len(store.GetFHSGuides()) == 0 {
		t.Skip("fhs_guides.json 未嵌入")
	}
	cases := []struct{ q, expect string }{
		{"宝宝吃母乳要喝水吗", "水"},
		{"婴儿睡姿", "睡"},
		{"怎么加辅食", "固体食物"},
		{"宝宝发烧", "发烧"},
		{"家居安全", "安全"},
	}
	for _, c := range cases {
		res, err := r.RetrieveFHSGuide(context.Background(), c.q, 3)
		if err != nil {
			t.Fatalf("query %q: %v", c.q, err)
		}
		if len(res) == 0 {
			t.Errorf("query %q: 无结果", c.q)
			continue
		}
		found := false
		for _, rr := range res {
			if strings.Contains(rr.Guide.Title, c.expect) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("query %q: top3 无标题含 %q；top=%s", c.q, c.expect, res[0].Guide.Title)
		}
		t.Logf("query %q -> %s (%.0f)", c.q, res[0].Guide.Title, res[0].Score)
	}
}
