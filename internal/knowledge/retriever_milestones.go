package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// milestoneAgeToMonth maps a CDC age_key to its month anchor.
func milestoneAgeToMonth(key string) int {
	switch key {
	case "2mo":
		return 2
	case "4mo":
		return 4
	case "6mo":
		return 6
	case "9mo":
		return 9
	case "12mo":
		return 12
	case "15mo":
		return 15
	case "18mo":
		return 18
	case "2yr":
		return 24
	case "30mo":
		return 30
	case "3yr":
		return 36
	case "4yr":
		return 48
	case "5yr":
		return 60
	}
	return -1
}

// MilestoneResult is one age's checklist plus navigation context.
type MilestoneResult struct {
	Age            *MilestoneAge `json:"age"`
	Definition     string        `json:"definition"`
	NextAgeKey     string        `json:"next_age_key,omitempty"` // checklist after this one
	NextAgeLabelZH string        `json:"next_age_label_zh,omitempty"`
	PrevAgeKey     string        `json:"prev_age_key,omitempty"`
	PrevAgeLabelZH string        `json:"prev_age_label_zh,omitempty"`
	Source         string        `json:"source"`
}

// RetrieveMilestones returns the CDC milestone checklist for the given age
// (in months, 0-71): the checklist whose anchor is the largest one ≤ age, so
// a 5-month-old gets the 4-month checklist. For age < 2 the 2-month
// checklist is returned with a note.
func (r *KeywordRetriever) RetrieveMilestones(ctx context.Context, ageMonths int) (*MilestoneResult, error) {
	r.store.ensureMilestones()
	ages := r.store.MilestoneAges
	if len(ages) == 0 {
		return nil, fmt.Errorf("milestones not loaded")
	}
	if ageMonths < 0 || ageMonths > 71 {
		return nil, fmt.Errorf("age out of range: CDC 清单覆盖出生至 5 周岁(0-71 月龄)，got %d", ageMonths)
	}
	sorted := make([]MilestoneAge, len(ages))
	copy(sorted, ages)
	sort.SliceStable(sorted, func(i, j int) bool {
		return milestoneAgeToMonth(sorted[i].AgeKey) < milestoneAgeToMonth(sorted[j].AgeKey)
	})

	idx := 0
	for i, a := range sorted {
		if milestoneAgeToMonth(a.AgeKey) <= ageMonths {
			idx = i
		}
	}
	res := &MilestoneResult{
		Age:        &sorted[idx],
		Definition: r.store.MilestoneDefinition,
		Source:     "CDC Learn the Signs. Act Early. (2022 修订版, 75% 儿童达成标准)",
	}
	if idx+1 < len(sorted) {
		res.NextAgeKey = sorted[idx+1].AgeKey
		res.NextAgeLabelZH = sorted[idx+1].AgeLabelZH
	}
	if idx > 0 {
		res.PrevAgeKey = sorted[idx-1].AgeKey
		res.PrevAgeLabelZH = sorted[idx-1].AgeLabelZH
	}
	return res, nil
}

// SearchMilestones finds checklist entries matching a free-text query across
// all ages (e.g. "还不 会说话", "走路"). Returns matched entries with their age.
type MilestoneMatch struct {
	AgeKey     string  `json:"age_key"`
	AgeLabelZH string  `json:"age_label_zh"`
	Domain     string  `json:"domain"` // social_emotional|language_communication|cognitive|movement_physical
	DomainZH   string  `json:"domain_zh"`
	TextZH     string  `json:"text_zh"`
	TextEN     string  `json:"text_en"`
	Score      float64 `json:"score"`
}

var milestoneDomains = []struct{ key, zh string }{
	{"social_emotional", "社交/情绪"},
	{"language_communication", "语言/沟通"},
	{"cognitive", "认知"},
	{"movement_physical", "运动/体格"},
}

// SearchMilestones searches zh+en milestone texts across every age.
func (r *KeywordRetriever) SearchMilestones(ctx context.Context, query string, topK int) ([]MilestoneMatch, error) {
	r.store.ensureMilestones()
	if topK <= 0 {
		topK = 8
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	windows := cjkWindows(q, 2, 6)
	latin := tokenize(q)
	var out []MilestoneMatch
	for _, age := range r.store.MilestoneAges {
		for _, dom := range milestoneDomains {
			zhField, enField := milestoneField(&age, dom.key)
			for i, zh := range zhField {
				en := ""
				if i < len(enField) {
					en = enField[i]
				}
				score := milestoneScore(zh, en, q, windows, latin)
				if score <= 0 {
					continue
				}
				out = append(out, MilestoneMatch{
					AgeKey: age.AgeKey, AgeLabelZH: age.AgeLabelZH,
					Domain: dom.key, DomainZH: dom.zh,
					TextZH: zh, TextEN: en, Score: score,
				})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

func milestoneField(a *MilestoneAge, dom string) (zh, en []string) {
	switch dom {
	case "social_emotional":
		return a.SocialEmotionalZH, a.SocialEmotional
	case "language_communication":
		return a.LanguageCommunicationZH, a.LanguageCommunication
	case "cognitive":
		return a.CognitiveZH, a.Cognitive
	case "movement_physical":
		return a.MovementPhysicalZH, a.MovementPhysical
	}
	return nil, nil
}

func milestoneScore(zh, en, q string, windows, latin []string) float64 {
	zhL, enL := strings.ToLower(zh), strings.ToLower(en)
	var score float64
	if strings.Contains(zhL, q) || strings.Contains(enL, q) {
		score += 10
	}
	for _, w := range windows {
		if strings.Contains(zhL, strings.ToLower(w)) {
			score += float64(len([]rune(w))) * 2
		}
	}
	for _, t := range latin {
		if len(t) >= 2 && strings.Contains(enL, t) {
			score += 4
		}
	}
	return score
}
