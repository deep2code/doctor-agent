package knowledge

import (
	"context"
	"sort"
	"strings"
)

// minAAPScore is the minimum score for an AAP article to be returned.
const minAAPScore = 2.0

// RetrieveAAP searches the embedded healthychildren.org (American Academy of
// Pediatrics) corpus (English) by title (weighted) and body token/substring
// matches — same scoring as MedlinePlus.
func (r *KeywordRetriever) RetrieveAAP(ctx context.Context, query string, topK int) ([]AAPResult, error) {
	if topK <= 0 {
		topK = 3
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	qLower := strings.ToLower(query)
	var words []string
	for _, t := range tokenize(query) {
		if len([]rune(t)) >= 3 {
			words = append(words, t)
		}
	}
	if len(words) == 0 {
		return nil, nil
	}

	var results []AAPResult
	for _, e := range r.store.AAPEntries {
		score, ok := scoreAAP(&e, qLower, words)
		if !ok || score < minAAPScore {
			continue
		}
		results = append(results, AAPResult{Entry: e, Score: score})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func scoreAAP(e *AAPEntry, qLower string, words []string) (float64, bool) {
	titleLower := strings.ToLower(e.Title)
	bodyLower := strings.ToLower(e.Content)
	var score float64
	matched := false

	if len([]rune(qLower)) >= 5 && strings.Contains(titleLower, qLower) {
		score += 15
		matched = true
	}
	if len([]rune(qLower)) >= 5 && strings.Contains(bodyLower, qLower) {
		score += 5
		matched = true
	}
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
