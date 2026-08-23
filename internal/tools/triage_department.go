package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// DepartmentMapping maps symptoms to recommended departments
var DepartmentMapping = map[string][]string{
	// 内科
	"头痛":     {"神经内科", "急诊科"},
	"头晕":     {"神经内科", "耳鼻喉科"},
	"发热":     {"发热门诊", "急诊科"},
	"咳嗽":     {"呼吸内科", "急诊科"},
	"胸痛":     {"心内科", "急诊科"},
	"心悸":     {"心内科", "急诊科"},
	"腹痛":     {"消化内科", "急诊科"},
	"腹泻":     {"消化内科", "急诊科"},
	"恶心":     {"消化内科", "急诊科"},
	"呕吐":     {"消化内科", "急诊科"},
	"便秘":     {"消化内科", "肛肠科"},
	"便血":     {"消化内科", "肛肠科"},
	"尿频":     {"泌尿外科", "肾内科"},
	"尿痛":     {"泌尿外科", "肾内科"},
	"血尿":     {"泌尿外科", "肾内科"},
	"水肿":     {"肾内科", "心内科"},
	"乏力":     {"内科", "血液科"},
	"消瘦":     {"内分泌科", "肿瘤科"},
	"多饮":     {"内分泌科"},
	"多尿":     {"内分泌科"},
	"多食":     {"内分泌科"},
	"关节痛":   {"风湿免疫科", "骨科"},
	"皮疹":     {"皮肤科"},
	"瘙痒":     {"皮肤科"},
	"失眠":     {"神经内科", "精神心理科"},
	"焦虑":     {"精神心理科"},
	"抑郁":     {"精神心理科"},
	"视力模糊": {"眼科"},
	"耳鸣":     {"耳鼻喉科"},
	"鼻塞":     {"耳鼻喉科"},
	"咽痛":     {"耳鼻喉科", "呼吸内科"},
	"牙痛":     {"口腔科"},
	"月经不调": {"妇科"},
	"痛经":     {"妇科"},
	"白带异常": {"妇科"},
	"乳腺肿块": {"乳腺外科", "妇科"},
	"睾丸疼痛": {"泌尿外科", "男科"},
	"腰痛":     {"骨科", "泌尿外科"},
	"背痛":     {"骨科", "疼痛科"},
	"颈椎痛":   {"骨科", "康复科"},
	"膝盖痛":   {"骨科", "风湿免疫科"},
	"外伤":     {"急诊科", "骨科"},
	"烫伤":     {"烧伤科", "急诊科"},
	"过敏":     {"变态反应科", "皮肤科"},
	"贫血":     {"血液科"},
	"出血":     {"血液科", "急诊科"},
	"肿块":     {"普外科", "肿瘤科"},
}

type TriageDepartmentTool struct {
	store *knowledge.Store
}

func NewTriageDepartmentTool(store *knowledge.Store) *TriageDepartmentTool {
	return &TriageDepartmentTool{store: store}
}

func (t *TriageDepartmentTool) Name() string {
	return "triage_department"
}

func (t *TriageDepartmentTool) Description() string {
	return "根据症状进行导诊分诊，推荐合适的科室和就医建议。支持多种症状组合分析。"
}

func (t *TriageDepartmentTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"symptoms": map[string]interface{}{
				"type":        "string",
				"description": "症状描述，多个症状用逗号分隔(如: 头痛,发热,咳嗽)",
			},
			"age": map[string]interface{}{
				"type":        "string",
				"description": "患者年龄(可选，用于儿童/老人特殊建议)",
			},
			"gender": map[string]interface{}{
				"type":        "string",
				"description": "患者性别(可选，用于妇科/男科推荐)",
				"enum":        []string{"男", "女"},
			},
		},
		"required": []string{"symptoms"},
	}
}

func (t *TriageDepartmentTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	symptoms, _ := args["symptoms"].(string)
	if symptoms == "" {
		return &ToolResult{Success: false, Error: "请提供症状描述 symptoms"}, nil
	}

	age, _ := args["age"].(string)
	gender, _ := args["gender"].(string)

	// Parse symptoms
	symptomList := strings.Split(symptoms, ",")
	for i := range symptomList {
		symptomList[i] = strings.TrimSpace(symptomList[i])
	}

	// Count department recommendations
	deptCount := make(map[string]int)
	matchedSymptoms := make(map[string][]string)

	for _, symptom := range symptomList {
		for keyword, depts := range DepartmentMapping {
			if strings.Contains(symptom, keyword) || strings.Contains(keyword, symptom) {
				for _, dept := range depts {
					deptCount[dept]++
					deptCount[dept] = deptCount[dept]
					matchedSymptoms[dept] = append(matchedSymptoms[dept], symptom)
				}
			}
		}
	}

	// Special handling for age
	if age != "" {
		if strings.Contains(age, "儿童") || strings.Contains(age, "小孩") || strings.Contains(age, "岁") {
			deptCount["儿科"] += 2
		}
		if strings.Contains(age, "老人") || strings.Contains(age, "老年") {
			deptCount["老年科"] += 1
		}
	}

	// Special handling for gender
	if gender == "女" {
		for _, symptom := range symptomList {
			if strings.Contains(symptom, "腹痛") || strings.Contains(symptom, "月经") {
				deptCount["妇科"] += 2
			}
		}
	}
	if gender == "男" {
		for _, symptom := range symptomList {
			if strings.Contains(symptom, "腹痛") && !strings.Contains(symptom, "上腹") {
				deptCount["泌尿外科"] += 1
			}
		}
	}

	if len(deptCount) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"message": fmt.Sprintf("未能根据症状「%s」匹配到具体科室，建议先到内科或急诊科就诊", symptoms),
			},
		}, nil
	}

	// Sort departments by count
	type deptScore struct {
		dept  string
		score int
	}
	var sorted []deptScore
	for dept, count := range deptCount {
		sorted = append(sorted, deptScore{dept, count})
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].score > sorted[i].score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("【导诊分诊建议】\n\n")

	// Primary recommendation
	if len(sorted) > 0 {
		sb.WriteString(fmt.Sprintf("▶ 首选科室: %s\n", sorted[0].dept))
		if symptoms, ok := matchedSymptoms[sorted[0].dept]; ok && len(symptoms) > 0 {
			sb.WriteString(fmt.Sprintf("  相关症状: %s\n", strings.Join(symptoms, ", ")))
		}
		sb.WriteString("\n")
	}

	// Other recommendations
	if len(sorted) > 1 {
		sb.WriteString("其他可选科室:\n")
		for i := 1; i < min(3, len(sorted)); i++ {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, sorted[i].dept))
		}
		sb.WriteString("\n")
	}

	// Triage level
	emergencySymptoms := []string{"胸痛", "呼吸困难", "意识模糊", "大量出血", "剧烈腹痛"}
	for _, symptom := range symptomList {
		for _, es := range emergencySymptoms {
			if strings.Contains(symptom, es) || strings.Contains(es, symptom) {
				sb.WriteString("⚠️ 紧急程度: 急诊 - 建议立即就医\n\n")
				break
			}
		}
	}

	// General advice
	sb.WriteString("【就医提示】\n")
	sb.WriteString("• 建议携带既往病历和检查报告\n")
	sb.WriteString("• 首诊建议上午空腹（如需抽血检查）\n")
	sb.WriteString("• 急诊情况请直接拨打120\n")

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"results":    sb.String(),
			"department": sorted[0].dept,
			"urgency":    "normal",
		},
		Citations: []CitationRef{
			{ID: "internal", Title: "Department Mapping Database", Level: "system"},
		},
	}, nil
}
