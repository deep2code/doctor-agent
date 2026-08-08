package knowledge

import (
	"context"
	"testing"
)

func TestMedlineLoad(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(store.GetMedlinePlusEntries()) == 0 {
		t.Skip("medlineplus.json 未嵌入")
	}
	t.Logf("MedlinePlus pages: %d", len(store.GetMedlinePlusEntries()))
}

func TestRetrieveMedlinePlus(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	if len(store.GetMedlinePlusEntries()) == 0 {
		t.Skip("medlineplus.json 未嵌入")
	}
	cases := []struct {
		query  string
		expect string
	}{
		{"diabetes", "Diabetes"},
		{"sickle cell", "Sickle"},
		{"food poisoning", "Food"},
	}
	for _, c := range cases {
		res, err := r.RetrieveMedlinePlus(context.Background(), c.query, 3)
		if err != nil {
			t.Fatalf("query %q: %v", c.query, err)
		}
		if len(res) == 0 {
			t.Errorf("query %q: 无结果", c.query)
			continue
		}
		t.Logf("query %q -> %s (%.0f)", c.query, res[0].Entry.Title, res[0].Score)
	}
}
