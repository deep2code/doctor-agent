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
	return "儿童生长评估（两种模式）：① 水平评估——输入性别、月龄和体重/身长(身高)/头围/BMI 测量值，按 WHO 儿童生长标准 + 中国 WS/T 423-2022《7岁以下儿童生长标准》(0-84月) 双标准计算 z 分数并判定（正常/低体重/生长迟缓/消瘦/超重/肥胖/重度等级）；6-18 岁学龄儿童按 WS/T 456-2014 + WS/T 586-2018 筛查（生长迟缓/消瘦/超重/肥胖）。② 速度评估（velocity）——输入两次测量的起始月龄、间隔月数(1/2/3/4/6)与增量（体重克数或身长/头围厘米数），按 WHO 2009 生长速度标准判断增长是否正常。当家长询问孩子身高体重是否达标、一个月长多少克正常、生长是否迟缓、是否超重肥胖时使用。"
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
			"velocity": map[string]any{
				"type":        "object",
				"description": "速度评估模式（与水平评估二选一）：提供后忽略 age_months/indicator/value",
				"properties": map[string]any{
					"from_month":      map[string]any{"type": "integer", "description": "间隔起始月龄（如出生起为 0）"},
					"interval_months": map[string]any{"type": "integer", "description": "测量间隔月数，须为 1/2/3/4/6 之一"},
					"indicator":       map[string]any{"type": "string", "description": "weight|length|head_circumference"},
					"delta":           map[string]any{"type": "number", "description": "区间增量：体重为克(g)，身长/头围为厘米(cm)"},
				},
				"required": []string{"from_month", "interval_months", "indicator", "delta"},
			},
		},
		"required": []string{"sex"},
	}
}

func (t *GrowthAssessment) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	sex, _ := input["sex"].(string)
	if sex == "" {
		return &ToolResult{Success: false, Error: "请提供 sex（男/女）"}, nil
	}
	// velocity mode
	if v, ok := input["velocity"].(map[string]any); ok && len(v) > 0 {
		fm, _ := v["from_month"].(float64)
		iv, _ := v["interval_months"].(float64)
		ind, _ := v["indicator"].(string)
		delta, _ := v["delta"].(float64)
		if ind == "" || delta <= 0 {
			return &ToolResult{Success: false, Error: "velocity 模式需要 from_month/interval_months/indicator/delta"}, nil
		}
		res, err := t.retriever.AssessGrowthVelocity(ctx, strings.TrimSpace(sex), int(fm), int(iv), ind, delta)
		if err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"mode": "velocity", "velocity": res,
				"verdicts":   []string{res.Result.Verdict},
				"references": []string{"WHO Growth Velocity Standards (2009)"},
			},
			Citations: []CitationRef{
				{ID: "who-velocity-2009", Title: "WHO growth velocity standards (2009)", Level: "international_standard", Year: 2009},
			},
		}, nil
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
	data := map[string]any{
		"sex": res.Sex, "age_months": res.AgeMonths,
		"indicator": res.IndicatorZH, "value": res.Value, "unit": res.Unit,
		"verdicts": verdicts,
		"china":    res.China,
		"who":      res.WHO,
	}
	citations := []CitationRef{
		{ID: "wst423-2022", Title: "WS/T 423-2022 7岁以下儿童生长标准", Level: "national_standard", Year: 2022},
		{ID: "who-cgs", Title: "WHO Child Growth Standards", Level: "international_standard", Year: 2006},
	}
	if res.SchoolAge != nil {
		data["school_age"] = res.SchoolAge
		verdicts = append(verdicts, res.SchoolAge.Verdict)
		data["verdicts"] = verdicts
		data["references"] = []string{
			"WS/T 456-2014《学龄儿童青少年营养不良筛查》",
			"WS/T 586-2018《学龄儿童青少年超重与肥胖筛查》",
		}
		citations = append(citations,
			CitationRef{ID: "wst456-2014", Title: "WS/T 456-2014 学龄儿童青少年营养不良筛查", Level: "national_standard", Year: 2014},
			CitationRef{ID: "wst586-2018", Title: "WS/T 586-2018 学龄儿童青少年超重与肥胖筛查", Level: "national_standard", Year: 2018})
	}
	return &ToolResult{Success: true, Data: data, Citations: citations}, nil
}
