package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// DiseaseEncyclopediaLookup queries the CMeKG disease encyclopedia.
type DiseaseEncyclopediaLookup struct {
	store *knowledge.Store
}

// NewDiseaseEncyclopediaLookup creates the disease encyclopedia lookup tool.
func NewDiseaseEncyclopediaLookup(store *knowledge.Store) *DiseaseEncyclopediaLookup {
	return &DiseaseEncyclopediaLookup{store: store}
}

func (t *DiseaseEncyclopediaLookup) Name() string {
	return "disease_encyclopedia_lookup"
}

func (t *DiseaseEncyclopediaLookup) Description() string {
	return "查询疾病百科数据库(CMeKG, 8,807种疾病)。输入疾病名称，返回详细的疾病信息：症状、病因、预防、治疗方法、常用药物、推荐食物、禁忌食物、并发症、检查项目、治疗科室、治疗周期、治愈概率、医疗费用等。当用户询问某种疾病的详细信息时使用。"
}

func (t *DiseaseEncyclopediaLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"disease": map[string]any{
				"type":        "string",
				"description": "疾病名称，如 '高血压'、'糖尿病'、'感冒'",
			},
		},
		"required": []string{"disease"},
	}
}

func (t *DiseaseEncyclopediaLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	disease, ok := input["disease"].(string)
	if !ok || strings.TrimSpace(disease) == "" {
		return &ToolResult{Success: false, Error: "请提供疾病名称 disease"}, nil
	}

	disease = strings.TrimSpace(disease)

	// Direct name lookup
	if d := t.store.GetDiseaseEncyclopediaByName(disease); d != nil {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"disease": disease,
				"result_count": 1,
				"results": []map[string]any{
					{
						"name": d.NameZH,
						"description": d.Description,
						"symptoms": d.Symptoms,
						"etiology": d.Etiology,
						"prevention": d.Prevention,
						"treatment_methods": d.TreatmentMethods,
						"treatment_departments": d.TreatmentDepartments,
						"common_drugs": d.CommonDrugs,
						"recommended_drugs": d.RecommendedDrugs,
						"recommended_foods": d.RecommendedFoods,
						"foods_to_avoid": d.FoodsToAvoid,
						"complications": d.Complications,
						"diagnostic_tests": d.DiagnosticTests,
						"treatment_duration": d.TreatmentDuration,
						"cure_rate": d.CureRate,
						"cost_estimate": d.CostEstimate,
						"high_risk_groups": d.HighRiskGroups,
						"incidence_rate": d.IncidenceRate,
					},
				},
			},
		}, nil
	}

	// Name substring search
	diseases := t.store.SearchDiseaseEncyclopedias(disease, 5)
	var matches []map[string]any
	for _, d := range diseases {
		matches = append(matches, map[string]any{
			"name": d.NameZH,
			"description": d.Description[:min(100, len(d.Description))] + "...",
			"symptoms": d.Symptoms,
		})
	}

	if len(matches) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"disease": disease, "result_count": 0,
				"message": fmt.Sprintf("疾病百科(8,807种)中未找到 '%s'。请确认疾病名称或尝试更简短的关键词。", disease),
			},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"disease": disease, "result_count": len(matches), "results": matches,
		},
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
