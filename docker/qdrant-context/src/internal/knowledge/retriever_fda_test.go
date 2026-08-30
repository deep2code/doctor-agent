package knowledge

import (
	"context"
	"testing"
)

func TestRetrieveFDALabel(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(store.GetFDALabels()) < 200 {
		t.Fatalf("expected >=200 FDA labels, got %d", len(store.GetFDALabels()))
	}
	r := NewRetriever(store)

	cases := []struct {
		query string
		want  string // expected name_zh of the top result
	}{
		{"阿巴卡韦", "阿巴卡韦"},
		{"abacavir", "阿巴卡韦"},
		{"二甲双胍", "二甲双胍"},
		{"metformin", "二甲双胍"},
		{"阿莫西林", "阿莫西林"},
		{"amoxicillin", "阿莫西林"},
		{"no-such-drug-xyz", ""},
	}
	for _, c := range cases {
		res, err := r.RetrieveFDALabel(context.Background(), c.query, 3)
		if err != nil {
			t.Fatalf("%q: %v", c.query, err)
		}
		if c.want == "" {
			if len(res) != 0 {
				t.Errorf("%q: expected no results, got %d (top %q)", c.query, len(res), res[0].Drug.NameZH)
			}
			continue
		}
		if len(res) == 0 || res[0].Drug.NameZH != c.want {
			got := ""
			if len(res) > 0 {
				got = res[0].Drug.NameZH
			}
			t.Errorf("%q: expected top %q, got %q", c.query, c.want, got)
		}
	}
}

func TestFDALabelFieldCompleteness(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	labels := store.GetFDALabels()
	for _, d := range labels {
		if d.NameZH == "" || d.NameEN == "" {
			t.Fatalf("label with empty name: %+v", d)
		}
		if len(d.Keywords) == 0 {
			t.Errorf("label %q has no keywords", d.NameZH)
		}
		if d.SourceURL == "" || d.SourceURL == "https://dailymed.nlm.nih.gov/dailymed/drugInfo.cfm?setid=" {
			t.Errorf("label %q has dead source_url: %q", d.NameZH, d.SourceURL)
		}
	}
}
