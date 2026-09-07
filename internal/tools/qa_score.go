package tools

import "strings"

// scoreQAPair scores one QA pair against the lower-cased query. Shared by
// huatuo_qa and medical_qa datasets: full-query containment in question (or
// related-disease, when non-empty) +10, answer +5, space-split word hits
// (>=2 chars) question +3 / disease +2. disease="" reproduces the
// medical_qa semantics exactly.
func scoreQAPair(question, answer, disease, lq string) int {
	q := strings.ToLower(question)
	a := strings.ToLower(answer)
	score := 0
	if strings.Contains(q, lq) || (disease != "" && strings.Contains(strings.ToLower(disease), lq)) {
		score += 10
	}
	if strings.Contains(a, lq) {
		score += 5
	}
	for _, word := range strings.Fields(lq) {
		if len(word) < 2 {
			continue
		}
		if strings.Contains(q, word) {
			score += 3
		}
		if disease != "" && strings.Contains(strings.ToLower(disease), word) {
			score += 2
		}
	}
	return score
}
