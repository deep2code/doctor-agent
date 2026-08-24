package knowledge

import (
	"context"
	"sort"
	"strings"
)

// minClinVarScore is the minimum score for a variant to be returned.
const minClinVarScore = 1.0

// RetrieveClinVar searches the embedded ClinVar subset by gene (symbol or
// Chinese alias), cDNA change (e.g. "c.79G>A"), variation name, or trait
// (e.g. "beta Thalassemia").
func (r *KeywordRetriever) RetrieveClinVar(ctx context.Context, query string, topK int) ([]ClinVarResult, error) {
	if topK <= 0 {
		topK = 10
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	qLower := strings.ToLower(query)
	qRunes := []rune(strings.ToLower(query))

	var results []ClinVarResult
	r.store.ensureClinVar()
	for _, v := range r.store.ClinVarVariants {
		score, ok := scoreClinVar(&v, qLower, qRunes)
		if !ok || score < minClinVarScore {
			continue
		}
		results = append(results, ClinVarResult{Variant: v, Score: score})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func scoreClinVar(v *ClinVarVariant, qLower string, qRunes []rune) (float64, bool) {
	var score float64
	matched := false

	geneLower := strings.ToLower(v.Gene)
	// Gene symbol exact/substring.
	if qLower == geneLower || strings.Contains(geneLower, qLower) || strings.Contains(qLower, geneLower) {
		score += 5
		matched = true
	}
	// Gene aliases (Chinese).
	for _, a := range geneNames[v.Gene] {
		if strings.Contains(qLower, strings.ToLower(a)) || strings.Contains(strings.ToLower(a), qLower) {
			score += 4
			matched = true
			break
		}
	}

	// cDNA change (e.g. "c.79g>a") — normalize case and whitespace.
	cdna := strings.ToLower(strings.ReplaceAll(v.Cdna, " ", ""))
	qNorm := strings.ToLower(strings.ReplaceAll(qLower, " ", ""))
	if cdna != "" && strings.Contains(qNorm, cdna) {
		score += 10
		matched = true
	} else if cdna != "" && strings.Contains(cdna, qNorm) && len([]rune(qNorm)) >= 4 {
		score += 8
		matched = true
	}

	// Variation name (full NM_... string) — contains cDNA anyway.
	variationLower := strings.ToLower(v.Variation)
	if strings.Contains(qNorm, variationLower) || strings.Contains(variationLower, qNorm) {
		score += 6
		matched = true
	}

	// Traits (disease names, e.g. "beta thalassemia").
	for _, t := range v.Traits {
		tLower := strings.ToLower(t)
		if strings.Contains(qLower, tLower) || strings.Contains(tLower, qLower) {
			score += 3
			matched = true
		}
	}

	// Pathogenic/Likely pathogenic in query ("致病").
	if strings.Contains(qLower, "致病") && strings.Contains(strings.ToLower(v.ClinicalSignificance), "pathogenic") {
		score += 2
		matched = true
	}
	_ = qRunes
	return score, matched
}
