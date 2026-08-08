package knowledge

import (
	"context"
	"sort"
	"strings"
)

// minMedinsScore is the minimum score for a drug to be returned.
const minMedinsScore = 1.0

// RetrieveMedinsDrug looks up drugs in the medical-insurance catalogue by
// Chinese name (substring) or pinyin-initials approximation (Latin tokens).
func (r *KeywordRetriever) RetrieveMedinsDrug(ctx context.Context, query string, topK int) ([]MedinsResult, error) {
	if topK <= 0 {
		topK = 10
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	qLower := strings.ToLower(query)

	var results []MedinsResult
	for _, d := range r.store.MedinsDrugs {
		nameLower := strings.ToLower(d.Name)
		score, ok := 0.0, false
		if nameLower == qLower {
			score, ok = 12, true // exact name match
		} else if strings.Contains(nameLower, qLower) || strings.Contains(qLower, nameLower) {
			score, ok = 8, true // bidirectional substring
		} else if len(qLower) >= 3 && len(nameLower) >= 3 && strings.Contains(qLower, nameLower[:3]) {
			score, ok = 3, true // partial
		}
		if ok && score >= minMedinsScore {
			results = append(results, MedinsResult{Drug: d, Score: score})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Drug.Name < results[j].Drug.Name
	})
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}
