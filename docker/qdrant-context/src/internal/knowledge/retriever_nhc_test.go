package knowledge

import (
	"context"
	"strings"
	"testing"
)

func TestNhcLoad(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(store.GetNHCGuides()) == 0 {
		t.Skip("nhc_guides.json 未嵌入")
	}
	t.Logf("NHC guides: %d", len(store.GetNHCGuides()))
}

func TestRetrieveNHCGuide(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	if len(store.GetNHCGuides()) == 0 {
		t.Skip("nhc_guides.json 未嵌入")
	}
	cases := []struct{ query, expect string }{
		{"流感", "流行性感冒诊疗方案"},
		{"脑血管病", "脑血管病防治指南"},
		{"肝癌", "原发性肝癌诊疗指南"},
		{"诺如病毒", "诺如病毒胃肠炎诊疗方案"},
		{"拉沙热", "拉沙热诊疗方案"},
		{"猴痘", "猴痘诊疗指南"},
		{"阿莫西林", "动物致伤诊疗规范"}, // 指南正文含抗菌药方案，属真实命中
		{"占星术", ""},                  // 指南库无此内容 → 无结果
	}
	for _, c := range cases {
		res, err := r.RetrieveNHCGuide(context.Background(), c.query, 3)
		if err != nil {
			t.Fatalf("query %q: %v", c.query, err)
		}
		if c.expect == "" {
			if len(res) != 0 {
				t.Errorf("query %q: 期望无结果，得到 %d 条", c.query, len(res))
			}
			continue
		}
		if len(res) == 0 {
			t.Errorf("query %q: 无结果", c.query)
			continue
		}
		if !strings.Contains(res[0].Guide.Title, c.expect) {
			t.Errorf("query %q: 期望标题含 %q，得到 %q", c.query, c.expect, res[0].Guide.Title)
		}
		t.Logf("query %q -> %s (%.0f)", c.query, res[0].Guide.Title, res[0].Score)
	}
}
