package knowledge

import (
	"context"
	"sort"
	"strings"
)

// minMSDScore is the minimum score for an MSD page to count as a match.
// One CJK substring hit on the title scores 3; one on the body scores 1.
const minMSDScore = 1.0

// RetrieveMSD searches the embedded MSD Manual (Chinese consumer edition)
// corpus. Chinese queries match by CJK substring/bigram overlap against the
// page title (weighted) and body; Latin queries (e.g. "G6PD", "HPV") match
// case-insensitively.
func (r *KeywordRetriever) RetrieveMSD(ctx context.Context, query string, topK int) ([]MSDResult, error) {
	if topK <= 0 {
		topK = 5
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	queryLower := strings.ToLower(query)
	// CJK substrings to search for: 2-6 rune windows covering the query.
	cjkWindows := cjkWindows(query, 2, 6)
	// Latin tokens for exact-ish matching.
	tokens := tokenize(query)
	var latin []string
	for _, t := range tokens {
		if !hasCJK(t) && len([]rune(t)) >= 2 {
			latin = append(latin, t)
		}
	}
	if len(cjkWindows) == 0 && len(latin) == 0 {
		return nil, nil
	}

	var results []MSDResult
	for _, e := range r.store.GetMSDEntries() {
		score, ok := scoreMSD(&e, queryLower, cjkWindows, latin)
		if !ok || score < minMSDScore {
			continue
		}
		results = append(results, MSDResult{Entry: e, Score: score})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// scoreMSD scores one MSD page. The full CJK query matched against the title
// dominates; longer windows outrank short 2-rune windows; Latin tokens match
// case-insensitively. This keeps e.g. "荨麻疹" on the urticaria page instead
// of any page merely containing "麻疹".
func scoreMSD(e *MSDEntry, queryLower string, cjkWindows []string, latin []string) (float64, bool) {
	titleLower := strings.ToLower(e.Title)
	bodyLower := strings.ToLower(e.Content)
	var score float64
	matchedAny := false

	// Full query in title/body (specificity: exact phrase beats fragments).
	if q := strings.TrimSpace(queryLower); len([]rune(q)) >= 2 && hasCJK(q) {
		if strings.Contains(titleLower, q) {
			score += 20
			matchedAny = true
		} else if strings.Contains(bodyLower, q) {
			score += 8
			matchedAny = true
		}
	}

	// CJK window hits: longer windows are more specific.
	for _, w := range cjkWindows {
		w := strings.ToLower(w)
		r := len([]rune(w))
		var weight float64
		switch {
		case r >= 4:
			weight = 8
		case r == 3:
			weight = 5
		default:
			weight = 2 // 2-rune window: weak signal
		}
		if strings.Contains(titleLower, w) {
			score += weight * 2
			matchedAny = true
		} else if strings.Contains(bodyLower, w) {
			score += weight
			matchedAny = true
		}
	}

	// Latin token hits.
	for _, t := range latin {
		if strings.Contains(titleLower, t) {
			score += 4
			matchedAny = true
		} else if strings.Contains(bodyLower, t) {
			score += 2
			matchedAny = true
		}
	}
	return score, matchedAny
}

// cjkWindows returns sliding CJK substrings of the query between minR and
// maxR runes (windows containing any Han character).
func cjkWindows(s string, minR, maxR int) []string {
	runes := []rune(s)
	var out []string
	seen := make(map[string]bool)
	for size := minR; size <= maxR; size++ {
		for i := 0; i+size <= len(runes); i++ {
			win := string(runes[i : i+size])
			if !hasCJK(win) {
				continue
			}
			if !seen[win] {
				seen[win] = true
				out = append(out, win)
			}
		}
	}
	return out
}
