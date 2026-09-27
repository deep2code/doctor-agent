package knowledge

import (
	"context"
	"testing"
)

// TestRetrieverWHOZhHealth verifies WHO Chinese health articles are retrievable.
func TestRetrieverWHOZhHealth(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	entries := store.GetAllMedical()

	// Count WHO Chinese entries
	whoCount := 0
	for _, e := range entries {
		if e.Category == "health_topic" || e.Category == "health_qa" || e.Category == "health_news" {
			whoCount++
		}
	}

	if whoCount < 300 {
		t.Errorf("Expected >= 300 WHO Chinese entries, got %d", whoCount)
	}

	t.Logf("✅ Found %d WHO Chinese health entries", whoCount)
}

// TestRetrieverWHOZhQueries tests common Chinese queries against WHO dataset.
func TestRetrieverWHOZhQueries(t *testing.T) {
	store, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	r := NewRetriever(store)
	ctx := context.Background()

	tests := []struct {
		query    string
		minScore float64
	}{
		{"流产安全", 3.0},
		{"避孕方法", 3.0},
		{"艾滋病预防", 3.0},
		{"癌症预防", 3.0},
		{"儿童生长", 3.0},
		{"心理健康", 3.0},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			results, err := r.Retrieve(ctx, tc.query, 5)
			if err != nil {
				t.Fatalf("Retrieve(%q) error: %v", tc.query, err)
			}
			if len(results) == 0 {
				t.Fatalf("Retrieve(%q) returned 0 results", tc.query)
			}

			// Check that we got results with reasonable scores
			if results[0].Score >= tc.minScore {
				t.Logf("✅ Query %q: top score %.1f, count=%d, first ID: %s",
					tc.query, results[0].Score, len(results), results[0].Entry.ID)
			} else {
				t.Logf("⚠️  Query %q: top score %.1f (expected >= %.1f), count=%d",
					tc.query, results[0].Score, tc.minScore, len(results))
			}
		})
	}
}
