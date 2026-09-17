package knowledge

import (
	"context"
	"testing"
)

func TestChinaVaccinesLoad(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.GetMedicalByID("cn-vaccine-overview") == nil {
		t.Fatal("china_vaccines.json 未嵌入（cn-vaccine-overview 缺失）")
	}
	t.Logf("china_vaccines entries embedded")
}

func TestRetrieveChinaVaccines(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.GetMedicalByID("cn-vaccine-overview") == nil {
		t.Skip("china_vaccines.json 未嵌入")
	}
	r := NewRetriever(store)
	cases := []struct{ q, expectID string }{
		{"孩子什么时候打预防针", "cn-vaccine-overview"},
		{"卡介苗", "cn-vaccine-bcg"},
		{"麻腮风疫苗", "cn-vaccine-mmr"},
		{"乙肝疫苗新生儿", "cn-vaccine-hepb"},
		{"甲肝疫苗", "cn-vaccine-hepa"},
	}
	for _, c := range cases {
		res, err := r.Retrieve(context.Background(), c.q, 5)
		if err != nil {
			t.Fatalf("query %q: %v", c.q, err)
		}
		found := false
		for _, e := range res {
			if e.Entry.ID == c.expectID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("query %q: 期望命中 %s，实际结果: %v", c.q, c.expectID, resIDs(res))
		}
		t.Logf("query %q -> top: %s", c.q, res[0].Entry.ID)
	}
}

func resIDs(res []RetrievalResult) []string {
	out := make([]string, 0, len(res))
	for _, e := range res {
		out = append(out, e.Entry.ID)
	}
	return out
}
