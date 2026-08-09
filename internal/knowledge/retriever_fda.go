package knowledge

import (
	"context"
	"sort"
	"strings"
)

// minFDALabelScore is the minimum score for an FDA-label entry.
const minFDALabelScore = 2.0

// RetrieveFDALabel looks up curated FDA drug labels by Chinese name,
// English name (INN), or keyword. Chinese queries map via name_zh; English
// via name_en substring (Latin, case-insensitive).
func (r *KeywordRetriever) RetrieveFDALabel(ctx context.Context, query string, topK int) ([]FDALabelResult, error) {
	if topK <= 0 {
		topK = 5
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	qLower := strings.ToLower(query)

	var results []FDALabelResult
	for _, d := range r.store.FDALabels {
		score, ok := 0.0, false
		nameZHLower := strings.ToLower(d.NameZH)
		nameENLower := strings.ToLower(d.NameEN)
		switch {
		case nameZHLower == qLower || nameENLower == qLower:
			score, ok = 12, true // exact (zh or en)
		case len(qLower) >= 2 && (strings.Contains(nameZHLower, qLower) || strings.Contains(qLower, nameZHLower)):
			score, ok = 8, true // Chinese substring (bidirectional)
		case len(qLower) >= 3 && strings.Contains(nameENLower, qLower):
			score, ok = 6, true // English substring
		}
		if !ok {
			for _, kw := range d.Keywords {
				kwLower := strings.ToLower(kw)
				if len(kwLower) >= 2 && (strings.Contains(qLower, kwLower) || strings.Contains(kwLower, qLower)) {
					score, ok = 5, true // keyword hit
					break
				}
			}
		}
		if ok && score >= minFDALabelScore {
			results = append(results, FDALabelResult{Drug: d, Score: score})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Drug.NameZH < results[j].Drug.NameZH
	})
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}
