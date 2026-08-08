package knowledge

import (
	"context"
	"sort"
	"strings"
)

// minMedlineScore is the minimum score for a MedlinePlus page to be returned.
const minMedlineScore = 2.0

// RetrieveMedlinePlus searches the embedded MedlinePlus corpus (English) by
// title (weighted) and body token/substring matches. Query words >= 3 chars
// match case-insensitively; the full query phrase in the title dominates.
func (r *KeywordRetriever) RetrieveMedlinePlus(ctx context.Context, query string, topK int) ([]MedlinePlusResult, error) {
	if topK <= 0 {
		topK = 5
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	qLower := strings.ToLower(query)
	tokens := tokenize(query)
	var words []string
	for _, t := range tokens {
		if len([]rune(t)) >= 3 {
			words = append(words, t)
		}
	}
	if len(words) == 0 {
		return nil, nil
	}

	var results []MedlinePlusResult
	for _, e := range r.store.MedlinePlusEntries {
		score, ok := scoreMedline(&e, qLower, words)
		if !ok || score < minMedlineScore {
			continue
		}
		results = append(results, MedlinePlusResult{Entry: e, Score: score})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func scoreMedline(e *MedlinePlusEntry, qLower string, words []string) (float64, bool) {
	titleLower := strings.ToLower(e.Title)
	bodyLower := strings.ToLower(e.Content)
	var score float64
	matched := false

	// Full phrase in title: strong signal.
	if len([]rune(qLower)) >= 5 && strings.Contains(titleLower, qLower) {
		score += 15
		matched = true
	}
	// Full phrase in body.
	if len([]rune(qLower)) >= 5 && strings.Contains(bodyLower, qLower) {
		score += 5
		matched = true
	}
	// Word hits.
	for _, w := range words {
		if strings.Contains(titleLower, w) {
			score += 4
			matched = true
		} else if strings.Contains(bodyLower, w) {
			score += 1
			matched = true
		}
	}
	return score, matched
}
