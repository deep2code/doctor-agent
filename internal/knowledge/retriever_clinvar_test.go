package knowledge

import (
	"context"
	"testing"
)

func TestClinVarLoad(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.GetClinVarCount() == 0 {
		t.Skip("clinvar.json 未嵌入")
	}
	t.Logf("ClinVar variants: %d", store.GetClinVarCount())
}

func TestRetrieveClinVar(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	if store.GetClinVarCount() == 0 {
		t.Skip("clinvar.json 未嵌入")
	}
	cases := []struct {
		query  string
		expect string // expected gene in top results
	}{
		{"c.79G>A", "HBB"},
		{"HBB", "HBB"},
		{"β地中海贫血", "HBB"},
		{"G6PD", "G6PD"},
		{"c.1376G>A", "G6PD"},
	}
	for _, c := range cases {
		res, err := r.RetrieveClinVar(context.Background(), c.query, 5)
		if err != nil {
			t.Fatalf("query %q: %v", c.query, err)
		}
		if len(res) == 0 {
			t.Errorf("query %q: 无结果", c.query)
			continue
		}
		t.Logf("query %q -> %s %s (%.0f)", c.query, res[0].Variant.Gene, res[0].Variant.Cdna, res[0].Score)
	}
}
