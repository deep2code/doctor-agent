package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

type DiseaseDrugLookupTool struct {
	store *knowledge.Store
}

func NewDiseaseDrugLookupTool(store *knowledge.Store) *DiseaseDrugLookupTool {
	return &DiseaseDrugLookupTool{store: store}
}

func (t *DiseaseDrugLookupTool) Name() string {
	return "disease_drug_lookup"
}

func (t *DiseaseDrugLookupTool) Description() string {
	return "查询疾病对应的常用药物和推荐药物。基于CMeKG疾病百科(8807种疾病)的治疗方案信息。"
}

func (t *DiseaseDrugLookupTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"disease": map[string]interface{}{
				"type":        "string",
				"description": "疾病名称(如: 高血压、糖尿病、肺炎)",
			},
		},
		"required": []string{"disease"},
	}
}

func (t *DiseaseDrugLookupTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	disease, _ := args["disease"].(string)
	if disease == "" {
		return &ToolResult{Success: false, Error: "请提供疾病名称 disease"}, nil
	}

	diseases := t.store.SearchDiseaseEncyclopedias(disease, 3)

	if len(diseases) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"message": fmt.Sprintf("未找到疾病'%s'的药物信息", disease),
			},
		}, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "疾病「%s」相关药物:\n\n", disease)

	for i, d := range diseases {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, d.NameZH)

		if len(d.CommonDrugs) > 0 {
			fmt.Fprintf(&sb, "   常用药物: %s\n", strings.Join(d.CommonDrugs[:min(5, len(d.CommonDrugs))], ", "))
		}

		if len(d.RecommendedDrugs) > 0 {
			fmt.Fprintf(&sb, "   推荐药物: %s\n", strings.Join(d.RecommendedDrugs[:min(5, len(d.RecommendedDrugs))], ", "))
		}

		if len(d.TreatmentMethods) > 0 {
			fmt.Fprintf(&sb, "   治疗方法: %s\n", strings.Join(d.TreatmentMethods[:min(3, len(d.TreatmentMethods))], ", "))
		}

		sb.WriteString("\n")
	}

	sb.WriteString("提示: 药物信息仅供参考，具体用药请遵医嘱。")

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
