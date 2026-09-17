package knowledge

import (
	"context"
	"testing"
)

func TestRetrieveEMLDrug(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)

	cases := []struct {
		query string
		want  string // expected top-result INN
	}{
		{"amoxicillin", "amoxicillin"},
		{"阿莫西林", "amoxicillin"}, // Chinese alias
		{"二甲双胍", "metformin"},
		{"ceftriaxone", "ceftriaxone"},
		{"头孢曲松", "ceftriaxone"},
		{"阿司匹林", "acetylsalicylic acid"},
		{"no-such-drug-xyz", ""},
	}
	for _, c := range cases {
		res, err := r.RetrieveEMLDrug(context.Background(), c.query, 5)
		if err != nil {
			t.Fatalf("%q: %v", c.query, err)
		}
		if c.want == "" {
			if len(res) != 0 {
				t.Errorf("%q: expected no results, got %d (top %q)", c.query, len(res), res[0].Entry.Name)
			}
			continue
		}
		if len(res) == 0 || res[0].Entry.Name != c.want {
			got := ""
			if len(res) > 0 {
				got = res[0].Entry.Name
			}
			t.Errorf("%q: expected top %q, got %q", c.query, c.want, got)
		}
	}
}

func TestEMLEntriesLoaded(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	entries := store.GetEMLEntries()
	if len(entries) < 500 {
		t.Fatalf("expected >=500 EML entries, got %d", len(entries))
	}
	// every entry has a name and belongs to core or complementary list
	for _, e := range entries {
		if e.Name == "" {
			t.Fatal("found EML entry with empty name")
		}
		if e.List != "core" && e.List != "complementary" {
			t.Fatalf("entry %q has invalid list %q", e.Name, e.List)
		}
	}
	// well-known entries must exist
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Name] = true
	}
	for _, want := range []string{"amoxicillin", "cefotaxime", "metformin", "paracetamol (acetaminophen)"} {
		if !seen[want] {
			t.Errorf("EML missing expected entry %q", want)
		}
	}
}
