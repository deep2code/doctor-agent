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

// minRelevantScore is the minimum retrieval score for an entry to count as
// knowledge relevant to the query. Below it the query is treated as "no
// knowledge match" (so the agent steers instead of improvising). One exact
// keyword/condition hit scores >= 3; bare region/category hits score <= 2.
const minRelevantScore = 3.0

// Retrieve searches medical, food-risk and lab-test knowledge entries
// matching the query.
//
// Two-pass: pass 1 uses the query verbatim (behaviour identical to the
// original retriever — no regression risk for queries that already recall).
// Only when pass 1 finds zero relevant entries does pass 2 retry with
// synonym-expanded query text, so colloquial phrasings ("突然大哭" vs the
// indexed "哭闹") still recall.
func (r *KeywordRetriever) Retrieve(ctx context.Context, query string, topK int) ([]RetrievalResult, error) {
	if topK <= 0 {
		topK = 5
	}

	base, err := r.retrieveOnce(ctx, query, topK, false)
	if err != nil {
		return nil, err
	}
	precise := 0
	for _, rr := range base {
		if rr.Score >= minRelevantScore {
			precise++
		}
	}
	if precise > 0 {
		return base, nil
	}

	expanded := ExpandQuery(query)
	if expanded == query {
		return base, nil
	}
	extra, err := r.retrieveOnce(ctx, expanded, topK, true)
	if err != nil {
		return base, nil
	}
	// De-dup: expanded pass may re-recall a base entry (base wins).
	seen := make(map[string]bool, len(base))
	for _, rr := range base {
		seen[rr.Entry.ID] = true
	}
	out := base
	for _, rr := range extra {
		if !seen[rr.Entry.ID] {
			out = append(out, rr)
		}
	}
	return out, nil
}

// retrieveOnce scores all entries against the given query text. When fallback
// is true the query is synonym-expanded and prose-body matching (strategy 6)
// is enabled; scores from this pass are kept low so verbatim matches always
// rank above them.
func (r *KeywordRetriever) retrieveOnce(ctx context.Context, query string, topK int, fallback bool) ([]RetrievalResult, error) {
	if fallback {
		query = ExpandQuery(query)
	}

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil, nil
	}

	// Food-risk and lab-test entries are indexed through their KnowledgeEntry
	// projection so their content reaches the prompt and the verifier too.
	// Prose corpora (FHS parenting / MSD Manual / MedlinePlus) are projected
	// with the article in Body and recalled via bigram-overlap matching.
	entries := r.store.GetAllMedical()
	entries = append(entries, r.store.FoodEntriesAsKnowledge()...)
	entries = append(entries, r.store.LabEntriesAsKnowledge()...)
	entries = append(entries, r.store.FHSGuidesAsKnowledge()...)
	entries = append(entries, r.store.MSDAsKnowledge()...)

	results := make([]RetrievalResult, 0, len(entries))

	for _, entry := range entries {
		score, matched := r.scoreEntry(&entry, query, queryTokens, fallback)
		if score < minRelevantScore {
			continue
		}
		results = append(results, RetrievalResult{
			Entry:           entry,
			Score:           score,
			MatchedKeywords: matched,
		})
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

// scoreEntry scores an entry against a query. Because tokenize() does not
// segment Chinese (a whole sentence becomes one token), token equality only
// works for Latin text; CJK recall relies on substring matching against the
// entry's Chinese keywords and symptom/risk fields.
func (r *KeywordRetriever) scoreEntry(entry *KnowledgeEntry, query string, queryTokens []string, fallback bool) (float64, []string) {
	var totalScore float64
	var matched []string
	matchedSet := make(map[string]bool)
	scored := make(map[string]bool)
	addMatch := func(kw string) {
		if !matchedSet[kw] {
			matchedSet[kw] = true
			matched = append(matched, kw)
		}
	}
	// scoreKW adds the per-keyword match score exactly once, even when the
	// same keyword is matched by several strategies below (token equality,
	// substring, bigram overlap) — preventing double-counting of one keyword.
	scoreKW := func(kw string) {
		if scored[kw] {
			return
		}
		scored[kw] = true
		totalScore += 3.0
		addMatch(kw)
	}

	queryLower := strings.ToLower(query)

	// 1. Token equality (works for Latin keywords like "G6PD", "EBV").
	for _, kw := range entry.Keywords {
		kwTokens := tokenize(kw)
		for _, qt := range queryTokens {
			for _, kt := range kwTokens {
				if strings.EqualFold(qt, kt) {
					scoreKW(kw)
					break
				}
			}
		}
	}

	// 2. Substring match against keywords. CJK keywords (>=2 runes) match any
	// containing query; Latin keywords need >=3 chars to avoid false hits on
	// short tokens like "AD".
	for _, kw := range entry.Keywords {
		kLower := strings.ToLower(kw)
		if !substringMatchable(kLower) {
			continue
		}
		if strings.Contains(queryLower, kLower) {
			scoreKW(kw)
		}
	}

	// 2b. Bigram overlap for longer CJK keywords. Catches spoken phrasing
	// with inserted particles ("我一喝牛奶就拉肚子" vs "喝牛奶拉肚子").
	for _, kw := range entry.Keywords {
		kLower := strings.ToLower(kw)
		if !hasCJK(kLower) || len([]rune(kLower)) < 4 {
			continue
		}
		kb := bigrams(kLower)
		qb := bigrams(queryLower)
		overlap := bigramOverlap(kb, qb)
		if overlap >= 2 && float64(overlap)/float64(len(kb)) >= 0.5 {
			scoreKW(kw)
		}
	}

	// 3. Condition name containment (kept from the original logic). Guard
	// against empty names: Contains(anything, "") is always true, which made
	// every entry with an empty ConditionEN/ConditionZH a universal match.
	condZHLower := strings.ToLower(entry.ConditionZH)
	condENLower := strings.ToLower(entry.ConditionEN)
	if condZHLower != "" && substringMatchable(condZHLower) && (strings.Contains(queryLower, condZHLower) || strings.Contains(condZHLower, queryLower)) {
		totalScore += 5.0
		addMatch(entry.ConditionZH)
	}
	if condENLower != "" && substringMatchable(condENLower) && (strings.Contains(queryLower, condENLower) || strings.Contains(condENLower, queryLower)) {
		totalScore += 5.0
		addMatch(entry.ConditionEN)
	}

	// 4. Symptom/risk/complication/differential/prevention field substring
	// matching — this is what lets symptom-style questions ("我一喝牛奶就
	// 拉肚子") recall the right entry.
	fields := [][]string{
		clinicalFeatures(entry),
		entry.RiskFactors,
		entry.Complications,
		entry.DifferentialDiagnosis,
		entry.Prevention,
	}
	for _, list := range fields {
		for _, item := range list {
			iLower := strings.ToLower(item)
			if !substringMatchable(iLower) {
				continue
			}
			if strings.Contains(queryLower, iLower) {
				totalScore += 2.0
				addMatch(item)
			}
		}
	}

	// 5. Region match: query containing the (Latin) region name.
	for _, region := range entry.Regions {
		if strings.Contains(queryLower, region) {
			totalScore += 1.0
			addMatch(region)
		}
	}

	// 6. Prose body bigram overlap (projected FHS/MSD/MedlinePlus articles).
	// Fallback only: evaluated when no curated-keyword strategy matched, so
	// precise entries always outrank long articles whose body merely contains
	// common characters. Score is kept below a single keyword hit (3.0).
	if fallback && totalScore == 0 && entry.Body != "" {
		bodyLower := strings.ToLower(entry.Body)
		qb := bigrams(queryLower)
		overlap := 0
		for bg := range qb {
			if strings.Contains(bodyLower, bg) {
				overlap++
			}
		}
		if overlap >= 10 && float64(overlap)/float64(len(qb)) >= 0.3 {
			totalScore += 2.0
			addMatch(entry.ConditionZH)
		}
	}

	categoryLower := strings.ToLower(entry.Category)
	if entry.Category != "" && strings.Contains(queryLower, categoryLower) {
		totalScore += 1.0
	}

	return totalScore, matched
}

// clinicalFeatures extracts the lab/imaging/clinical feature lists, guarding
// against a nil DiagnosticCriteria pointer.
func clinicalFeatures(e *KnowledgeEntry) []string {
	if e.Diagnosis == nil {
		return nil
	}
	return e.Diagnosis.ClinicalFeatures
}

// substringMatchable reports whether a keyword/field is safe to substring
// match: CJK text (>=2 runes) or Latin text of at least 3 chars.
func substringMatchable(s string) bool {
	n := len([]rune(s))
	if n < 2 {
		return false
	}
	if hasCJK(s) {
		return true
	}
	return n >= 3
}

// hasCJK reports whether the string contains any Han characters.
func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// bigrams returns the set of character bigrams of a string.
func bigrams(s string) map[string]bool {
	runes := []rune(s)
	out := make(map[string]bool)
	for i := 0; i+1 < len(runes); i++ {
		out[string(runes[i:i+2])] = true
	}
	return out
}

// bigramOverlap counts how many bigrams of a appear in b.
func bigramOverlap(a, b map[string]bool) int {
	n := 0
	for k := range a {
		if b[k] {
			n++
		}
	}
	return n
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
	docFreq      map[string]int
	totalDocs    int
	avgDocLength float64 // average document length across the corpus
}

func BM25Score(queryTokens []string, docTokens []string, idf *IDF, k1, b float64) float64 {
	if idf == nil || idf.totalDocs == 0 {
		return 0
	}

	docLen := float64(len(docTokens))
	if docLen == 0 {
		return 0
	}

	avgDocLen := idf.avgDocLength
	if avgDocLen == 0 {
		avgDocLen = 1 // fallback to avoid division by zero
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
