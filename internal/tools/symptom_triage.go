package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// SymptomTriage performs urgency assessment based on chief complaint.
type SymptomTriage struct {
	store *knowledge.Store
}

// NewSymptomTriage creates the symptom triage tool.
func NewSymptomTriage(store *knowledge.Store) *SymptomTriage {
	return &SymptomTriage{store: store}
}

func (t *SymptomTriage) Name() string {
	return "symptom_triage"
}

func (t *SymptomTriage) Description() string {
	return "根据主诉和伴随症状进行紧急分诊评估。返回紧急等级（emergency/urgent/routine/self_care）及具体行动建议。参考中国120急救指南和南方流行病学特征。"
}

func (t *SymptomTriage) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"chief_complaint": map[string]any{
				"type":        "string",
				"description": "主诉症状，如 '胸痛'、'发热39度头痛'、'吃了蚕豆后脸色发黄尿色深'",
			},
			"accompanying_symptoms": map[string]any{
				"type":        "string",
				"description": "伴随症状（可选），如 '出冷汗、恶心、左臂放射痛'",
			},
			"patient_context": map[string]any{
				"type":        "string",
				"description": "患者背景信息（可选），如 '52岁男性、有高血压史、广东人、已知G6PD缺乏'",
			},
		},
		"required": []string{"chief_complaint"},
	}
}

func (t *SymptomTriage) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	chiefComplaint, _ := input["chief_complaint"].(string)
	accompanying, _ := input["accompanying_symptoms"].(string)
	patientCtx, _ := input["patient_context"].(string)

	if strings.TrimSpace(chiefComplaint) == "" {
		return &ToolResult{
			Success: false,
			Error:   "请提供主诉症状",
		}, nil
	}

	fullText := chiefComplaint + " " + accompanying + " " + patientCtx
	rules := t.store.GetAllEmergencyRules()

	// Check against emergency triage rules
	for _, rule := range rules {
		for _, kw := range rule.KeywordsZH {
			if strings.Contains(fullText, kw) {
				return &ToolResult{
					Success: true,
					Data: map[string]any{
						"triage_level":      rule.Level,
						"matched_condition": rule.Condition,
						"matched_keyword":   kw,
						"action":            rule.ActionZH,
						"chief_complaint":   chiefComplaint,
					},
					Citations: t.buildTriageCitations(rule),
				}, nil
			}
		}
		for _, kw := range rule.Keywords {
			if strings.Contains(strings.ToLower(fullText), strings.ToLower(kw)) {
				return &ToolResult{
					Success: true,
					Data: map[string]any{
						"triage_level":      rule.Level,
						"matched_condition": rule.Condition,
						"matched_keyword":   kw,
						"action":            rule.ActionZH,
						"chief_complaint":   chiefComplaint,
					},
					Citations: t.buildTriageCitations(rule),
				}, nil
			}
		}
	}

	// No emergency pattern matched
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"triage_level":    "routine",
			"matched_condition": "no_emergency_pattern",
			"action":          fmt.Sprintf("根据主诉 '%s'，未匹配到需要立即急救的紧急模式。建议：如果症状持续或加重，请在24-48小时内前往全科门诊或相应专科就诊。如果出现以下任何情况，请立即就医：意识改变、呼吸困难、剧烈疼痛、大出血、高热不退。", chiefComplaint),
			"chief_complaint": chiefComplaint,
		},
	}, nil
}

func (t *SymptomTriage) buildTriageCitations(rule knowledge.EmergencyRule) []CitationRef {
	refs := make([]CitationRef, 0)
	if len(rule.Citations) > 0 {
		for _, c := range rule.Citations {
			refs = append(refs, CitationRef{
				Title: c.Title,
				DOI:   c.DOI,
				PMID:  c.PMID,
				Level: c.Level,
				Year:  c.Year,
			})
		}
	}
	return refs
}
