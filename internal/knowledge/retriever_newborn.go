package knowledge

import (
	"context"
	"sort"
	"strings"
)

// NewbornResult is one matched newborn-care knowledge item.
type NewbornResult struct {
	Kind     string  `json:"kind"` // "who_recommendation" | "china_screening"
	ID       string  `json:"id"`
	TitleZH  string  `json:"title_zh"`
	Domain   string  `json:"domain,omitempty"`
	BodyZH   string  `json:"body_zh"`
	BodyEN   string  `json:"body_en,omitempty"`
	Strength string  `json:"strength,omitempty"`
	URL      string  `json:"url,omitempty"`
	Score    float64 `json:"score"`
}

// SearchNewbornCare searches the WHO preterm/LBW recommendations (zh+en) and
// the China newborn screening programme notes by free text. Queries like
// "袋鼠护理", "早产儿 喂养", "咖啡因 呼吸暂停", "足跟血", "听力筛查" all hit.
func (r *KeywordRetriever) SearchNewbornCare(ctx context.Context, query string, topK int) ([]NewbornResult, error) {
	r.store.ensureNewborn()
	doc := r.store.NewbornCare
	if doc == nil {
		return nil, nil
	}
	if topK <= 0 {
		topK = 5
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	windows := cjkWindows(q, 2, 6)
	var latin []string
	for _, t := range tokenize(q) {
		if !hasCJK(t) && len(t) >= 2 {
			latin = append(latin, t)
		}
	}

	var out []NewbornResult
	for _, rec := range doc.WHOPretermLBW.Recommendations {
		title := rec.TopicZH + " " + rec.Domain
		score := newbornScore(title, rec.RecommendationZH, rec.RecommendationEN, q, windows, latin)
		if score <= 0 {
			continue
		}
		out = append(out, NewbornResult{
			Kind: "who_recommendation", ID: rec.ID, TitleZH: rec.TopicZH,
			Domain: rec.Domain, BodyZH: rec.RecommendationZH,
			BodyEN: rec.RecommendationEN, Strength: rec.StrengthEN,
			URL: doc.WHOPretermLBW.URL, Score: score,
		})
	}
	for _, sp := range doc.ChinaScreening {
		score := newbornScore(sp.TitleZH, strings.Join(sp.Points, " "), "", q, windows, latin)
		if score <= 0 {
			continue
		}
		out = append(out, NewbornResult{
			Kind: "china_screening", ID: sp.ID, TitleZH: sp.TitleZH,
			BodyZH: strings.Join(sp.Points, "\n"), URL: sp.URL, Score: score,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

func newbornScore(title, zh, en, q string, windows, latin []string) float64 {
	titleL, zhL := strings.ToLower(title), strings.ToLower(zh)
	enL := strings.ToLower(en)
	var score float64
	if strings.Contains(titleL, q) {
		score += 12
	} else if zhL != "" && strings.Contains(zhL, q) {
		score += 6
	} else if enL != "" && strings.Contains(enL, q) {
		score += 4
	}
	for _, w := range windows {
		w = strings.ToLower(w)
		weight := float64(len([]rune(w)))
		if strings.Contains(titleL, w) {
			score += weight * 2
		} else if zhL != "" && strings.Contains(zhL, w) {
			score += weight
		} else if enL != "" && strings.Contains(enL, w) {
			score += weight
		}
	}
	for _, t := range latin {
		if strings.Contains(titleL, t) {
			score += 4
		} else if enL != "" && strings.Contains(enL, t) {
			score += 2
		}
	}
	return score
}
