package knowledge

import (
	"context"
	"strings"
	"testing"
)

func TestLiteratureLoad(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.GetLiteratureCount() == 0 {
		t.Fatal("literature corpus empty")
	}
	topics := store.GetLiteratureTopics()
	if len(topics) != 16 {
		t.Fatalf("expected 16 topics, got %d", len(topics))
	}
	// Every topic id must resolve to a non-empty article list.
	for _, tp := range topics {
		if n := len(store.GetLiteratureByTopic(tp.ID)); n == 0 {
			t.Errorf("topic %s has 0 articles", tp.ID)
		}
	}
	t.Logf("literature loaded: %d topics, %d articles", len(topics), store.GetLiteratureCount())
}

func TestRetrieveLiteratureChineseTopic(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	cases := []struct {
		query    string
		wantZH   string
		wantDOIs bool
	}{
		{"地中海贫血", "地中海贫血", true},
		{"地贫筛查", "地中海贫血", true},
		{"G6PD缺乏症", "G6PD缺乏症", true},
		{"我一喝牛奶就拉肚子", "乳糖不耐受", true},
		{"鼻咽癌", "鼻咽癌", true},
		{"dengue vaccine", "登革热疫苗", true},
	}
	for _, c := range cases {
		res, err := r.RetrieveLiterature(context.Background(), c.query, 3)
		if err != nil {
			t.Fatalf("query %q: %v", c.query, err)
		}
		if len(res) == 0 {
			t.Errorf("query %q: no results", c.query)
			continue
		}
		if res[0].Topic.NameZH != c.wantZH {
			t.Errorf("query %q: first topic %q, want %q", c.query, res[0].Topic.NameZH, c.wantZH)
		}
		if c.wantDOIs {
			for i, x := range res {
				if x.Entry.DOI == "" && x.Entry.PMID == "" {
					t.Errorf("query %q result %d has neither DOI nor PMID", c.query, i)
				}
				if x.Entry.Abstract == "" {
					t.Errorf("query %q result %d has empty abstract", c.query, i)
				}
			}
		}
	}
}

func TestRetrieveLiteratureEnglishGlobal(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	// An English term not tied to a topic keyword should still match via
	// title/abstract search.
	res, err := r.RetrieveLiterature(context.Background(), "thalassaemia carrier screening", 3)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("expected global English match")
	}
	for _, x := range res {
		joined := strings.ToLower(x.Entry.Title + " " + x.Entry.Abstract)
		if !strings.Contains(joined, "thalassaemia") && !strings.Contains(joined, "carrier") {
			t.Errorf("result not matching query: %s", x.Entry.Title)
		}
	}
}

func TestRetrieveLiteratureNoMatch(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)
	res, err := r.RetrieveLiterature(context.Background(), "quantum entanglement physics", 3)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("expected no results for unrelated query, got %d", len(res))
	}
}
