package tools

import (
	"context"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// GrowthAssessment evaluates a child's measurement (weight / length-height /
// head circumference / BMI) against the WHO Child Growth Standards and the
// China WS/T 423-2022 standard, returning interpolated z-scores and verdicts.
type GrowthAssessment struct {
	retriever *knowledge.KeywordRetriever
}

// NewGrowthAssessment creates the growth assessment tool.
func NewGrowthAssessment(store *knowledge.Store) *GrowthAssessment {
	return &GrowthAssessment{retriever: knowledge.NewRetriever(store)}
}

func (t *GrowthAssessment) Name() string { return "growth_assessment" }

func (t *GrowthAssessment) Description() string {
	return "儿童生长评估：输入性别、月龄和体重/身长(身高)/头围/BMI 测量值，按 WHO 儿童生长标准与中国卫生行业标准 WS/T 423-2022《7岁以下儿童生长标准》计算 z 分数（-3SD~+3SD 区间线性插值），返回判定（正常/低体重/生长迟缓/消瘦/超重/肥胖/重度等级）与中位数参考。当家长询问孩子身高体重是否达标、生长是否迟缓、是否超重肥胖时使用。支持 0-84 月龄（WHO 部分为 0-60 月龄）。"
}

func (t *GrowthAssessment) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"sex": map[string]any{
				"type":        "string",
				"description": "性别：男 或 女",
			},
			"age_months": map[string]any{
				"type":        "integer",
				"description": "月龄（0-84）。也可用 age_years+age_months_extra 组合，但优先直接给总月龄",
			},
			"indicator": map[string]any{
				"type":        "string",
				"description": "指标：weight(体重,kg) | length_height(身长/身高,cm) | head_circumference(头围,cm) | bmi(体重指数)",
			},
			"value": map[string]any{
				"type":        "number",
				"description": "测量值（体重 kg / 身长身高 cm / 头围 cm / BMI kg/m²）",
			},
		},
		"required": []string{"sex", "age_months", "indicator", "value"},
	}
}

func (t *GrowthAssessment) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	sex, _ := input["sex"].(string)
	if sex == "" {
		return &ToolResult{Success: false, Error: "请提供 sex（男/女）"}, nil
	}
	ageMonths := 0
	if v, ok := input["age_months"].(float64); ok {
		ageMonths = int(v)
	} else if y, ok := input["age_years"].(float64); ok {
		ageMonths = int(y) * 12
		if m, ok2 := input["age_months_extra"].(float64); ok2 {
			ageMonths += int(m)
		}
	}
	indicator, _ := input["indicator"].(string)
	value := 0.0
	if v, ok := input["value"].(float64); ok {
		value = v
	}
	if indicator == "" || value <= 0 {
		return &ToolResult{Success: false, Error: "请提供 indicator（weight/length_height/head_circumference/bmi）与 value（测量值）"}, nil
	}

	res, err := t.retriever.AssessGrowth(ctx, strings.TrimSpace(sex), ageMonths, strings.TrimSpace(indicator), value)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	verdicts := []string{}
	if res.China != nil {
		verdicts = append(verdicts, res.China.Verdict)
	}
	if res.WHO != nil {
		verdicts = append(verdicts, res.WHO.Verdict)
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"sex": res.Sex, "age_months": res.AgeMonths,
			"indicator": res.IndicatorZH, "value": res.Value, "unit": res.Unit,
			"verdicts": verdicts,
			"china":    res.China,
			"who":      res.WHO,
			"references": []string{
				"WS/T 423-2022《7岁以下儿童生长标准》(国家卫健委 2022)",
				"WHO Child Growth Standards (2006/2007)",
			},
		},
		Citations: []CitationRef{
			{ID: "wst423-2022", Title: "WS/T 423-2022 7岁以下儿童生长标准", Level: "national_standard", Year: 2022},
			{ID: "who-cgs", Title: "WHO Child Growth Standards", Level: "international_standard", Year: 2006},
		},
	}, nil
}
