package knowledge

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"
)

// minCorpusScore is the minimum score for a corpus document to count as a match.
const minCorpusScore = 1.0

// RetrieveCorpus searches the unified medkb corpora (CorpusDoc) with the same
// scoring family as NHC/MSD: full query against the title zone dominates,
// longer CJK windows outrank 2-rune windows, Latin tokens match
// case-insensitively. sources filters by CorpusDoc.Source (nil/empty = all).
func (r *KeywordRetriever) RetrieveCorpus(ctx context.Context, query string, topK int, sources ...string) ([]CorpusResult, error) {
	if topK <= 0 {
		topK = 3
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	srcSet := make(map[string]bool, len(sources))
	for _, s := range sources {
		if s != "" {
			srcSet[s] = true
		}
	}

	queryLower := strings.ToLower(query)
	windows := cjkWindows(query, 2, 6)
	// zh→en bridge for English corpora (LactMed/StatPearls): expand common
	// Chinese drug/disease names so 哺乳期布洛芬 queries reach "Ibuprofen".
	var phrases []string
	for key, syns := range corpusSynonyms {
		if strings.Contains(query, key) {
			phrases = append(phrases, syns...)
		}
	}
	var latin []string
	for _, t := range tokenize(query) {
		if !hasCJK(t) && len([]rune(t)) >= 2 {
			latin = append(latin, t)
		}
	}
	if len(windows) == 0 && len(latin) == 0 && len(phrases) == 0 {
		return nil, nil
	}

	var results []CorpusResult
	r.store.ensureCorpus()
	for i := range r.store.CorpusDocs {
		d := &r.store.CorpusDocs[i]
		if len(srcSet) > 0 && !srcSet[d.Source] {
			continue
		}
		score, ok := scoreCorpus(d, queryLower, windows, phrases, latin)
		if !ok || score < minCorpusScore {
			continue
		}
		results = append(results, CorpusResult{Doc: *d, Score: score, Excerpt: corpusExcerpt(d, queryLower, windows, phrases, latin)})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Doc.Title < results[j].Doc.Title
	})
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// corpusSynonyms bridges common Chinese queries to English corpus wording.
// Keys are substrings of the query; values are English phrases to add.
var corpusSynonyms = map[string][]string{
	"布洛芬":        {"ibuprofen"},
	"对乙酰氨基酚":     {"acetaminophen", "paracetamol"},
	"阿莫西林":       {"amoxicillin"},
	"青霉素":        {"penicillin"},
	"头孢":         {"cephalosporin"},
	"阿司匹林":       {"aspirin"},
	"母乳喂养":       {"breastfeeding", "breast feeding"},
	"哺乳期":        {"breastfeeding", "lactation"},
	"哮喘":         {"asthma"},
	"唐氏":         {"down syndrome"},
	"地中海贫血":      {"thalassemia"},
	"蚕豆病":        {"g6pd"},
	"苯丙酮尿症":      {"phenylketonuria"},
	"脊髓性肌萎缩":     {"spinal muscular atrophy"},
	"血友病":        {"hemophilia"},
	"先天性甲状腺功能减低": {"congenital hypothyroidism"},
}

// scoreCorpus scores one corpus document: full query title +20 / summary +10 /
// body +6; CJK windows 4+ runes +8, 3 +5, 2 +2 (title zone double); English
// phrases +8 title zone / +3 body zone; Latin tokens +4 / +2.
// Title zone = Title, TitleZH and Keywords; body zone = Summary and Body.
func scoreCorpus(d *CorpusDoc, queryLower string, cjkWindows, phrases, latin []string) (float64, bool) {
	titleLower := strings.ToLower(d.Title + " " + d.TitleZH)
	for _, k := range d.Keywords {
		titleLower += " " + strings.ToLower(k)
	}
	summaryLower := strings.ToLower(d.Summary)
	bodyLower := strings.ToLower(d.Body)
	var score float64
	matchedAny := false

	// Full-query containment (CJK or Latin, 2+ runes).
	if q := strings.TrimSpace(queryLower); len([]rune(q)) >= 2 {
		if strings.Contains(titleLower, q) {
			score += 20
			matchedAny = true
		} else if strings.Contains(summaryLower, q) {
			score += 10
			matchedAny = true
		} else if strings.Contains(bodyLower, q) {
			score += 6
			matchedAny = true
		}
		// Exact title match outranks substring hits ("Asthma" > "Allergic asthma").
		if strings.TrimSpace(strings.ToLower(d.Title)) == q {
			score += 15
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
		} else if strings.Contains(summaryLower, w) || strings.Contains(bodyLower, w) {
			score += weight
			matchedAny = true
		}
	}

	for _, p := range phrases {
		p := strings.ToLower(p)
		if strings.Contains(titleLower, p) {
			score += 8
			matchedAny = true
		} else if strings.Contains(summaryLower, p) || strings.Contains(bodyLower, p) {
			score += 3
			matchedAny = true
		}
	}

	for _, t := range latin {
		if strings.Contains(titleLower, t) {
			score += 4
			matchedAny = true
		} else if strings.Contains(summaryLower, t) || strings.Contains(bodyLower, t) {
			score += 2
			matchedAny = true
		}
	}
	return score, matchedAny
}

// corpusExcerpt cuts a ±700-rune window of Body around the earliest term
// match, falling back to the head of the body when nothing matches inside it.
func corpusExcerpt(d *CorpusDoc, queryLower string, cjkWindows, phrases, latin []string) string {
	body := d.Body
	if body == "" {
		body = d.Summary
	}
	runes := []rune(body)
	if len(runes) <= 1400 {
		return body
	}
	lower := strings.ToLower(body)
	best := -1
	consider := func(term string) {
		term = strings.ToLower(strings.TrimSpace(term))
		if len([]rune(term)) < 2 {
			return
		}
		if i := strings.Index(lower, term); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	consider(queryLower)
	for _, ws := range append(append([]string{}, cjkWindows...), phrases...) {
		consider(ws)
	}
	for _, t := range latin {
		consider(t)
	}
	start := 0
	if best > 0 {
		// best is a byte offset; convert to a rune index without panicking on
		// multibyte boundaries by trimming partial runes.
		for best > 0 && !utf8.RuneStart(body[best]) {
			best--
		}
		start = len([]rune(body[:best])) - 300
		if start < 0 {
			start = 0
		}
	}
	end := start + 1400
	if end > len(runes) {
		end = len(runes)
	}
	out := string(runes[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…\n\n[truncated]"
	}
	return out
}
