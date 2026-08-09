package knowledge

import (
	"context"
	"sort"
	"strings"
)

// minFHSScore is the minimum score for an FHS page to count as a match.
const minFHSScore = 1.0

// RetrieveFHSGuide searches the 香港卫生署家庭健康服务 parenting corpus
// (Simplified Chinese) by full text. Same scoring as NHC/MSD: full CJK query
// in title dominates, longer CJK windows outrank 2-rune windows, Latin tokens
// match case-insensitively.
func (r *KeywordRetriever) RetrieveFHSGuide(ctx context.Context, query string, topK int) ([]FHSGuideResult, error) {
	if topK <= 0 {
		topK = 3
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	queryLower := strings.ToLower(query)
	windows := cjkWindows(query, 2, 6)
	// FHS uses Hong Kong wording (固体食物 for 辅食, 仰睡 for 睡姿 etc.);
	// map mainland colloquial terms so they still hit the right pages.
	for _, syns := range [][]string{nhcSynonymList(query), fhsSynonymList(query)} {
		for _, s := range syns {
			if !containsStr(windows, s) {
				windows = append(windows, s)
			}
		}
	}
	var latin []string
	for _, t := range tokenize(query) {
		if !hasCJK(t) && len([]rune(t)) >= 2 {
			latin = append(latin, t)
		}
	}
	if len(windows) == 0 && len(latin) == 0 {
		return nil, nil
	}

	var results []FHSGuideResult
	for _, g := range r.store.FHSGuides {
		score, ok := scoreFHS(&g, queryLower, windows, latin)
		if !ok || score < minFHSScore {
			continue
		}
		results = append(results, FHSGuideResult{Guide: g, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Guide.Title < results[j].Guide.Title
	})
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// scoreFHS scores one FHS page (identical to scoreNHC).
func scoreFHS(g *FHSGuide, queryLower string, cjkWindows []string, latin []string) (float64, bool) {
	titleLower := strings.ToLower(g.Title)
	bodyLower := strings.ToLower(g.Content)
	var score float64
	matchedAny := false

	if q := strings.TrimSpace(queryLower); len([]rune(q)) >= 2 && hasCJK(q) {
		if strings.Contains(titleLower, q) {
			score += 20
			matchedAny = true
		} else if strings.Contains(bodyLower, q) {
			score += 8
			matchedAny = true
		}
	}

	for _, w := range cjkWindows {
		w := strings.ToLower(w)
		var weight float64
		switch r := len([]rune(w)); {
		case r >= 4:
			weight = 8
		case r == 3:
			weight = 5
		default:
			weight = 2
		}
		if strings.Contains(titleLower, w) {
			score += weight * 2
			matchedAny = true
		} else if strings.Contains(bodyLower, w) {
			score += weight
			matchedAny = true
		}
	}

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
