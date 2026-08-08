package knowledge

import (
	"context"
	"sort"
	"strings"
)

// minTopicHits is the minimum topic score for a query to be considered
// "about" that topic. One Chinese keyword hit or one exact English token hit
// scores >= 1.0.
const minTopicHits = 0.75

// RetrieveLiterature searches the embedded Europe PMC corpus. Chinese queries
// are routed through the topic table (topic keywords carry Chinese + English
// synonyms); English queries additionally match article titles/abstracts.
func (r *KeywordRetriever) RetrieveLiterature(ctx context.Context, query string, topK int) ([]LiteratureResult, error) {
	if topK <= 0 {
		topK = 5
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	queryLower := strings.ToLower(query)
	tokens := tokenize(query)

	// 1. Score every topic against the query.
	type topicScore struct {
		topic LiteratureTopic
		score float64
	}
	var scored []topicScore
	for _, topic := range r.store.GetLiteratureTopics() {
		if s := scoreTopic(topic, queryLower, tokens); s >= minTopicHits {
			scored = append(scored, topicScore{topic, s})
		}
	}

	// 2a. Topic route hit -> rank articles inside the matched topics.
	if len(scored) > 0 {
		sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
		results := make([]LiteratureResult, 0, topK)
		for _, ts := range scored {
			articles := r.store.GetLiteratureByTopic(ts.topic.ID)
			ranked := rankTopicArticles(articles, queryLower, tokens)
			for _, art := range ranked {
				if len(results) >= topK {
					return results, nil
				}
				results = append(results, LiteratureResult{Entry: *art, Topic: ts.topic, Score: artScore(*art, queryLower, tokens)})
			}
		}
		return results, nil
	}

	// 2b. No topic hit -> global title/abstract match (English queries).
	var results []LiteratureResult
	for _, art := range r.store.LiteratureArticles {
		score := artScore(art, queryLower, tokens)
		if score > 0 {
			results = append(results, LiteratureResult{Entry: art, Score: score})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// scoreTopic scores how well the query matches a topic's keywords. Chinese
// keywords use bidirectional substring matching; Latin keywords match by
// token equality (score 1.0) or, failing that, by >=3-char substring with a
// length bonus so "dengue vaccine" beats plain "dengue". Each keyword counts
// at most once (best match wins), avoiding double counting.
func scoreTopic(t LiteratureTopic, queryLower string, tokens []string) float64 {
	score := 0.0
	qRunes := float64(len([]rune(queryLower)))
	for _, kw := range t.Keywords {
		kwLower := strings.ToLower(strings.TrimSpace(kw))
		if kwLower == "" {
			continue
		}
		if hasCJK(kwLower) {
			if len([]rune(kwLower)) >= 2 &&
				(strings.Contains(queryLower, kwLower) || strings.Contains(kwLower, queryLower)) {
				score += 1.0
			}
			continue
		}
		// Latin keyword: token equality is the strongest signal.
		tokenHit := false
		for _, tok := range tokens {
			if strings.EqualFold(tok, kwLower) {
				tokenHit = true
				break
			}
		}
		if tokenHit {
			score += 1.0
			continue
		}
		// Substring hit: bonus proportional to keyword length vs query length
		// (longer, more specific keyword = stronger match).
		if len([]rune(kwLower)) >= 3 && strings.Contains(queryLower, kwLower) {
			bonus := float64(len([]rune(kwLower))) / qRunes
			if bonus > 1.0 {
				bonus = 1.0
			}
			score += 1.0 + bonus
		}
	}
	return score
}

// artScore scores one article against the query by token/substring hits on
// title (3x) and abstract (1x). Returns 0 when nothing matched.
func artScore(art LiteratureEntry, queryLower string, tokens []string) float64 {
	titleLower := strings.ToLower(art.Title)
	abstractLower := strings.ToLower(art.Abstract)
	var score float64
	for _, tok := range tokens {
		if len(tok) < 3 {
			continue
		}
		if strings.Contains(titleLower, tok) {
			score += 3.0
		} else if strings.Contains(abstractLower, tok) {
			score += 1.0
		}
	}
	return score
}

// rankTopicArticles orders a topic's articles: relevance-matched first (by
// score desc), then newer years, then those with abstracts, then id for
// determinism.
func rankTopicArticles(articles []*LiteratureEntry, queryLower string, tokens []string) []*LiteratureEntry {
	out := make([]*LiteratureEntry, len(articles))
	copy(out, articles)
	hasEnglish := false
	for _, tok := range tokens {
		if len(tok) >= 3 && !hasCJK(tok) {
			hasEnglish = true
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if hasEnglish {
			sa, sb := artScore(*a, queryLower, tokens), artScore(*b, queryLower, tokens)
			if sa != sb {
				return sa > sb
			}
		}
		if a.Year != b.Year {
			return a.Year > b.Year
		}
		aa, ab := len(a.Abstract) > 0, len(b.Abstract) > 0
		if aa != ab {
			return aa
		}
		return a.ID < b.ID
	})
	return out
}
