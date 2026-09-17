package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

type DiseaseSymptomLookupTool struct {
	store *knowledge.Store
}

func NewDiseaseSymptomLookupTool(store *knowledge.Store) *DiseaseSymptomLookupTool {
	return &DiseaseSymptomLookupTool{store: store}
}

func (t *DiseaseSymptomLookupTool) Name() string {
	return "disease_symptom_lookup"
}

func (t *DiseaseSymptomLookupTool) Description() string {
	return "根据症状查询可能的疾病。基于CMeKG疾病百科(8807种疾病)的临床表现、诊断、治疗等全面信息。"
}

func (t *DiseaseSymptomLookupTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"symptom": map[string]interface{}{
				"type":        "string",
				"description": "症状描述(如: 头痛、发热、咳嗽)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "返回结果数量(默认5，最多20)",
			},
		},
		"required": []string{"symptom"},
	}
}

func (t *DiseaseSymptomLookupTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	symptom, _ := args["symptom"].(string)
	if symptom == "" {
		return &ToolResult{Success: false, Error: "请提供症状描述 symptom"}, nil
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
		if limit > 20 {
			limit = 20
		}
	}

	diseases := t.store.SearchDiseaseEncyclopedias(symptom, limit)

	if len(diseases) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"message": fmt.Sprintf("未找到与症状'%s'相关的疾病信息", symptom),
			},
		}, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "与症状「%s」相关的疾病:\n\n", symptom)

	for i, d := range diseases {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, d.NameZH)

		if len(d.Symptoms) > 0 {
			fmt.Fprintf(&sb, "   常见症状: %s\n", strings.Join(d.Symptoms[:min(3, len(d.Symptoms))], ", "))
		}

		if len(d.TreatmentMethods) > 0 {
			fmt.Fprintf(&sb, "   治疗方法: %s\n", strings.Join(d.TreatmentMethods[:min(2, len(d.TreatmentMethods))], ", "))
		}

		if d.CureRate != "" {
			fmt.Fprintf(&sb, "   治愈率: %s\n", d.CureRate)
		}

		sb.WriteString("\n")
	}

	sb.WriteString("提示: 以上信息仅供参考，具体诊断请咨询专业医生。")

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"results": sb.String(),
			"count":   len(diseases),
		},
		Citations: []CitationRef{
			{ID: "cmekg", Title: "CMeKG Disease Encyclopedia", Level: "database"},
		},
	}, nil
}
