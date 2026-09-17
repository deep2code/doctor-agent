package knowledge

import (
	"context"
	"testing"
)

func TestRetrieveMedins(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	if len(store.GetMedinsDrugs()) == 0 {
		t.Skip("medins_drugs.json 未嵌入")
	}
	cases := []struct{ q, expect string }{
		{"阿莫西林", "阿莫西林"},
		{"二甲双胍", "二甲双胍"},
		{"布洛芬", "布洛芬"},
	}
	for _, c := range cases {
		res, err := r.RetrieveMedinsDrug(context.Background(), c.q, 3)
		if err != nil {
			t.Fatalf("query %q: %v", c.q, err)
		}
		if len(res) == 0 {
			t.Errorf("query %q: 无结果", c.q)
			continue
		}
		t.Logf("query %q -> %s (%s, %v)", c.q, res[0].Drug.Name, res[0].Drug.Category, res[0].Drug.Forms)
	}
}
