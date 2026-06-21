package knowledge

import (
	"context"
	"math"
	"sort"
	"strings"
	"unicode"
)

// KeywordRetriever performs keyword-based and simple TF-IDF retrieval
// over the knowledge store. It avoids embedding models and vector
// databases, making the system self-contained.
type KeywordRetriever struct {
	store *Store
}

// NewKeywordRetriever creates a keyword retriever backed by the given store.
func NewKeywordRetriever(store *Store) *KeywordRetriever {
	return &KeywordRetriever{store: store}
}

// NewRetriever creates a keyword retriever (default retriever for backward compatibility).
func NewRetriever(store *Store) *KeywordRetriever {
	return &KeywordRetriever{store: store}
}

func (r *KeywordRetriever) Name() string {
	return "keyword"
}

// Retrieve searches medical knowledge entries matching the query.
func (r *KeywordRetriever) Retrieve(ctx context.Context, query string, topK int) ([]RetrievalResult, error) {
	if topK <= 0 {
		topK = 5
	}

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil, nil
	}

	entries := r.store.GetAllMedical()
	results := make([]RetrievalResult, 0, len(entries))

	for _, entry := range entries {
		score, matched := r.scoreEntry(&entry, queryTokens)
		if score > 0 {
			results = append(results, RetrievalResult{
				Entry:           entry,
				Score:           score,
				MatchedKeywords: matched,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// RetrieveDrugs searches drug entries matching the query.
func (r *KeywordRetriever) RetrieveDrugs(ctx context.Context, query string, topK int) ([]DrugRetrievalResult, error) {
	if topK <= 0 {
		topK = 5
	}

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil, nil
	}

	entries := r.store.GetAllDrugs()
	results := make([]DrugRetrievalResult, 0)

	for _, entry := range entries {
		score := r.scoreDrug(&entry, queryTokens)
		if score > 0 {
			results = append(results, DrugRetrievalResult{
				Entry: entry,
				Score: score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// RetrieveEmergencyRules finds matching emergency triage rules.
func (r *KeywordRetriever) RetrieveEmergencyRules(symptoms string) []EmergencyRule {
	tokens := tokenize(symptoms)
	rules := r.store.GetAllEmergencyRules()
	var matched []EmergencyRule

	for _, rule := range rules {
		for _, kw := range rule.Keywords {
			for _, token := range tokens {
				if strings.EqualFold(kw, token) {
					matched = append(matched, rule)
					goto nextRule
				}
			}
		}
		for _, kw := range rule.KeywordsZH {
			if strings.Contains(symptoms, kw) {
				matched = append(matched, rule)
				goto nextRule
			}
		}
	nextRule:
	}

	return matched
}

func (r *KeywordRetriever) scoreEntry(entry *KnowledgeEntry, queryTokens []string) (float64, []string) {
	var totalScore float64
	var matched []string

	for _, kw := range entry.Keywords {
		kwTokens := tokenize(kw)
		for _, qt := range queryTokens {
			for _, kt := range kwTokens {
				if strings.EqualFold(qt, kt) {
					totalScore += 3.0
					matched = append(matched, kw)
					break
				}
			}
		}
	}

	queryLower := strings.ToLower(strings.Join(queryTokens, " "))
	condZHLower := strings.ToLower(entry.ConditionZH)
	condENLower := strings.ToLower(entry.ConditionEN)

	if strings.Contains(queryLower, condZHLower) || strings.Contains(condZHLower, queryLower) {
		totalScore += 5.0
		matched = append(matched, entry.ConditionZH)
	}
	if strings.Contains(queryLower, condENLower) || strings.Contains(condENLower, queryLower) {
		totalScore += 5.0
		matched = append(matched, entry.ConditionEN)
	}

	for _, region := range entry.Regions {
		for _, qt := range queryTokens {
			if strings.EqualFold(qt, region) {
				totalScore += 1.0
				break
			}
		}
	}

	categoryLower := strings.ToLower(entry.Category)
	if strings.Contains(queryLower, categoryLower) {
		totalScore += 1.0
	}

	return totalScore, matched
}

func (r *KeywordRetriever) scoreDrug(entry *DrugEntry, queryTokens []string) float64 {
	var score float64
	queryLower := strings.ToLower(strings.Join(queryTokens, " "))

	if strings.Contains(queryLower, strings.ToLower(entry.GenericNameEN)) ||
		strings.Contains(queryLower, strings.ToLower(entry.GenericNameZH)) {
		score += 10.0
	}

	for _, tn := range entry.TradeNames {
		if strings.Contains(queryLower, strings.ToLower(tn)) {
			score += 8.0
			break
		}
	}

	for _, kw := range entry.Keywords {
		for _, qt := range queryTokens {
			if strings.EqualFold(qt, kw) {
				score += 3.0
				break
			}
		}
	}

	if strings.Contains(queryLower, strings.ToLower(entry.DrugClass)) {
		score += 2.0
	}

	return score
}

func tokenize(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})

	seen := make(map[string]bool)
	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) < 1 {
			continue
		}
		if len(f) == 1 && f[0] >= 'a' && f[0] <= 'z' {
			continue
		}
		if !seen[f] {
			seen[f] = true
			tokens = append(tokens, f)
		}
	}

	return tokens
}

// IDF calculation for BM25-like scoring (simplified).
type IDF struct {
	docFreq   map[string]int
	totalDocs int
}

func BM25Score(queryTokens []string, docTokens []string, idf *IDF, k1, b float64) float64 {
	if idf == nil || idf.totalDocs == 0 {
		return 0
	}

	avgDocLen := float64(idf.totalDocs)
	docLen := float64(len(docTokens))
	if docLen == 0 {
		return 0
	}

	var score float64
	for _, qt := range queryTokens {
		df, ok := idf.docFreq[qt]
		if !ok || df == 0 {
			continue
		}
		idfVal := math.Log(1 + (float64(idf.totalDocs)-float64(df)+0.5)/(float64(df)+0.5))

		tf := 0
		for _, dt := range docTokens {
			if dt == qt {
				tf++
			}
		}
		if tf == 0 {
			continue
		}

		tfNorm := (float64(tf) * (k1 + 1)) / (float64(tf) + k1*(1-b+b*docLen/avgDocLen))
		score += idfVal * tfNorm
	}

	return score
}
