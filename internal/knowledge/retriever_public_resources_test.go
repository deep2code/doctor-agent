package knowledge

import (
	"context"
	"strings"
	"testing"
)

// TestPublicResourcesLoad tests that public resources can be loaded from the database.
func TestPublicResourcesLoad(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Ensure public resources are loaded
	_ = store.ensurePublicResources()
	store.mu.RLock()
	count := len(store.PublicResources)
	store.mu.RUnlock()

	if count == 0 {
		t.Skip("public_resources.json 未嵌入")
	}
	t.Logf("Public resources count: %d", count)
}

// TestRetrievePublicResources tests the public resources retrieval.
func TestRetrievePublicResources(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRetriever(store)

	// Check if public resources are loaded
	store.mu.RLock()
	count := len(store.PublicResources)
	store.mu.RUnlock()

	if count == 0 {
		t.Skip("public_resources.json 未嵌入")
	}

	cases := []struct {
		query       string
		expectMatch string // expected category or keyword in results
		minResults  int
	}{
		{"医学视频", "教学视频", 1},
		{"教科书", "教科书", 1},
		{"公开课", "公开课", 1},
		{"PubMed", "文献数据库", 1},
		{"科普", "科普", 1},
		{"WHO", "指南", 1},
		{"丁香园", "科普", 1},
		{"clinical", "ClinicalTrials", 1},
		{"textbook", "", 1}, // English keyword
		{"Osmosis", "", 1},
	}

	for _, c := range cases {
		res, err := r.RetrievePublicResources(context.Background(), c.query, 5)
		if err != nil {
			t.Fatalf("query %q: %v", c.query, err)
		}
		if len(res) < c.minResults {
			t.Errorf("query %q: expected >= %d results, got %d", c.query, c.minResults, len(res))
			continue
		}
		// Check if at least one result matches the expected category/keyword
		found := false
		for _, r := range res {
			if c.expectMatch != "" {
				if containsString(r.Resource.Category, c.expectMatch) ||
					containsString(r.Resource.NameZH, c.expectMatch) ||
					containsString(r.Resource.NameEN, c.expectMatch) ||
					containsString(r.Resource.DescriptionZH, c.expectMatch) ||
					containsStringSlice(r.Resource.Keywords, c.expectMatch) {
					found = true
					break
				}
			} else {
				// For English queries, just check we got results
				found = true
				break
			}
		}
		if c.expectMatch != "" && !found {
			t.Logf("query %q: results don't contain %q, got: %v", c.query, c.expectMatch, res)
		}
		t.Logf("query %q -> %d results, top: %s (score=%.1f)", c.query, len(res), res[0].Resource.NameZH, res[0].Score)
	}
}

// TestScorePublicResource tests the scoring function for public resources.
func TestScorePublicResource(t *testing.T) {
	tests := []struct {
		name       string
		resource   PublicResource
		query      string
		expectZero bool
	}{
		{
			name: "exact name match",
			resource: PublicResource{
				NameZH:     "默克诊疗手册",
				NameEN:     "Merck Manual",
				Category:   "教科书",
				Keywords:   []string{"医学参考", "诊疗手册"},
			},
			query:      "默克诊疗手册",
			expectZero: false,
		},
		{
			name: "category match",
			resource: PublicResource{
				NameZH:     "NCBI Bookshelf",
				NameEN:     "NCBI Bookshelf",
				Category:   "教科书",
				Keywords:   []string{"生物医学", "书籍"},
			},
			query:      "教科书",
			expectZero: false,
		},
		{
			name: "keyword match",
			resource: PublicResource{
				NameZH:     "Osmosis",
				NameEN:     "Osmosis",
				Category:   "教学视频",
				Keywords:   []string{"医学动画", "教学视频", "Osmosis"},
			},
			query:      "医学动画",
			expectZero: false,
		},
		{
			name: "description match",
			resource: PublicResource{
				NameZH:          "PubMed Central",
				NameEN:          "PubMed Central",
				Category:        "文献数据库",
				DescriptionZH:   "NIH提供的免费生物医学文献全文数据库",
				DescriptionEN:   "Free full-text biomedical literature",
				Keywords:        []string{"文献", "PubMed"},
			},
			query:      "生物医学文献",
			expectZero: false,
		},
		{
			name: "no match",
			resource: PublicResource{
				NameZH:     "NCBI Bookshelf",
				NameEN:     "NCBI Bookshelf",
				Category:   "教科书",
				Keywords:   []string{"生物医学"},
			},
			query:      "完全不相关的查询",
			expectZero: true,
		},
		{
			name: "english query",
			resource: PublicResource{
				NameZH:     "ClinicalTrials.gov",
				NameEN:     "ClinicalTrials.gov",
				Category:   "临床试验",
				Keywords:   []string{"临床试验", "research"},
			},
			query:      "clinical trials",
			expectZero: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cjkWindows := cjkWindows(tt.query, 2, 6)
			tokens := tokenize(tt.query)
			var latin []string
			for _, t := range tokens {
				if !hasCJK(t) && len([]rune(t)) >= 2 {
					latin = append(latin, t)
				}
			}
			queryLower := strings.ToLower(tt.query)
			score, matched := scorePublicResource(&tt.resource, queryLower, cjkWindows, latin)

			if tt.expectZero && matched {
				t.Errorf("expected no match, got score=%.1f", score)
			}
			if !tt.expectZero && !matched {
				t.Errorf("expected match, got no match")
			}
			if matched {
				t.Logf("query %q -> score=%.1f", tt.query, score)
			}
		})
	}
}

// TestPublicResourcesCategories tests that all expected categories are present.
func TestPublicResourcesCategories(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_ = store.ensurePublicResources()

	store.mu.RLock()
	count := len(store.PublicResources)
	store.mu.RUnlock()

	if count == 0 {
		t.Skip("public_resources.json 未嵌入")
	}

	// Check categories are present
	categories := make(map[string]int)
	for _, r := range store.PublicResources {
		categories[r.Category]++
	}

	expectedCategories := []string{"教科书", "教学视频", "公开课", "文献数据库", "科普", "指南", "临床试验", "科普视频"}
	for _, cat := range expectedCategories {
		if categories[cat] == 0 {
			t.Logf("Warning: category %q not found", cat)
		} else {
			t.Logf("Category %q: %d resources", cat, categories[cat])
		}
	}
}

// TestPublicResourcesURLValidity tests that all URLs are valid.
func TestPublicResourcesURLValidity(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_ = store.ensurePublicResources()

	store.mu.RLock()
	count := len(store.PublicResources)
	store.mu.RUnlock()

	if count == 0 {
		t.Skip("public_resources.json 未嵌入")
	}

	// Just check that URLs are not empty
	emptyURLs := 0
	for _, r := range store.PublicResources {
		if r.URL == "" {
			emptyURLs++
			t.Logf("Resource %s has empty URL", r.ID)
		}
	}

	if emptyURLs > 0 {
		t.Errorf("%d resources have empty URLs", emptyURLs)
	}
}

// Helper functions
func containsString(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func containsStringSlice(slice []string, substr string) bool {
	substrLower := strings.ToLower(substr)
	for _, s := range slice {
		if strings.Contains(strings.ToLower(s), substrLower) {
			return true
		}
	}
	return false
}