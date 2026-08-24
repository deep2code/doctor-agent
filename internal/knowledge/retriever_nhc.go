package knowledge

import (
	"context"
	"sort"
	"strings"
)

// minNHCScore is the minimum score for an NHC guideline to count as a match.
const minNHCScore = 1.0

// RetrieveNHCGuide searches the embedded 国家卫健委 诊疗方案/指南 corpus by
// Chinese full-text. Same scoring as MSD: the full CJK query against the
// title dominates, longer CJK windows outrank 2-rune windows, and Latin
// tokens (H1N1, COVID) match case-insensitively.
func (r *KeywordRetriever) RetrieveNHCGuide(ctx context.Context, query string, topK int) ([]NHCGuideResult, error) {
	if topK <= 0 {
		topK = 3
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	queryLower := strings.ToLower(query)
	windows := cjkWindows(query, 2, 6)
	// NHC guideline titles use formal disease names; map colloquial queries
	// (流感 → 流行性感冒, 新冠 → 新型冠状病毒) so full-query/title matching
	// picks the right guideline instead of a mention in some other body.
	for key, syns := range nhcSynonyms {
		if strings.Contains(query, key) {
			for _, s := range syns {
				if !containsStr(windows, s) {
					windows = append(windows, s)
				}
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

	var results []NHCGuideResult
	r.store.ensureNHC()
	for _, g := range r.store.NHCGuides {
		score, ok := scoreNHC(&g, queryLower, windows, latin)
		if !ok || score < minNHCScore {
			continue
		}
		results = append(results, NHCGuideResult{Guide: g, Score: score})
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

// containsStr reports whether ss contains s.
func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// nhcSynonyms maps colloquial queries to the formal disease names used in
// NHC guideline titles, so 2-char queries that are not a contiguous substring
// of the title (流行性感冒 contains no 流感) still match the right guideline.
var nhcSynonyms = map[string][]string{
	"流感": {"流行性感冒"},
	"新冠": {"新型冠状病毒"},
	"甲流": {"流行性感冒"},
	"乙肝": {"乙型病毒性肝炎"},
	"手足口": {"手足口病"},
}

// nhcSynonymList expands query against nhcSynonyms.
func nhcSynonymList(query string) []string {
	var out []string
	for key, syns := range nhcSynonyms {
		if strings.Contains(query, key) {
			out = append(out, syns...)
		}
	}
	return out
}

// fhsSynonyms maps mainland colloquial parenting terms to Hong Kong FHS
// wording (辅食→固体食物, 睡姿→仰睡, 发烧→发热 etc.).
var fhsSynonyms = map[string][]string{
	"辅食":   {"固体食物"},
	"睡姿":   {"仰睡", "睡眠"},
	"发烧":   {"发热"},
	"宝宝":   {"婴儿", "幼儿"},
	"拉肚子": {"腹泻"},
	"喂奶":   {"母乳"},
}

// fhsSynonymList expands query against fhsSynonyms.
func fhsSynonymList(query string) []string {
	var out []string
	for key, syns := range fhsSynonyms {
		if strings.Contains(query, key) {
			out = append(out, syns...)
		}
	}
	return out
}

// scoreNHC scores one guideline: full query in title +20 / body +8; CJK
// windows 4+ runes +8, 3 runes +5, 2 runes +2 (title hits double); Latin
// tokens +4 title / +2 body.
func scoreNHC(g *NHCGuide, queryLower string, cjkWindows []string, latin []string) (float64, bool) {
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
