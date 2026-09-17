package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// VisitPrep 生成就诊准备单：把对话中已经明确的症状、用药、检查整理成
// 一份结构化清单，方便用户直接带给医生看，减少门诊沟通成本。
// 内容字段由对话 LLM 从会话中提取，工具只负责格式化（不产生医学判断）。
type VisitPrep struct{}

func NewVisitPrep() *VisitPrep { return &VisitPrep{} }

func (t *VisitPrep) Name() string { return "visit_prep" }

func (t *VisitPrep) Description() string {
	return "把当前对话中已经明确的病情信息整理成一份「就诊准备单」（主诉、症状时间线、正在用的药、过敏史、已做过的检查、想问医生的问题、建议挂号科室），用户可复制或打印带给医生。当用户要去医院/复诊/咨询医生，或对话已积累了较完整的病情信息时主动使用。只整理对话中出现过的信息，不要编造。"
}

func (t *VisitPrep) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"chief_complaint":   map[string]any{"type": "string", "description": "一句话主诉（如：反复咳嗽2周，夜间加重）"},
			"symptom_timeline":  map[string]any{"type": "string", "description": "症状出现/变化时间线，多行"},
			"current_meds":      map[string]any{"type": "string", "description": "正在使用的药物和剂量，多行，没有则留空"},
			"allergies":         map[string]any{"type": "string", "description": "已知过敏（药物/食物），没有则留空"},
			"past_history":      map[string]any{"type": "string", "description": "相关既往史/慢性病"},
			"recent_tests":      map[string]any{"type": "string", "description": "近期做过的检查和关键结果"},
			"red_flags":         map[string]any{"type": "string", "description": "对话中提到的需警惕症状（高热不退、咯血等），没有则留空"},
			"questions_to_ask":  map[string]any{"type": "string", "description": "建议问医生的问题，每行一条"},
			"target_department": map[string]any{"type": "string", "description": "建议挂号科室"},
			"patient_profile":   map[string]any{"type": "string", "description": "年龄/性别等基本信息"},
		},
		"required": []string{"chief_complaint", "questions_to_ask", "target_department"},
	}
}

func listInput(input map[string]any, key string) []string {
	v, _ := input[key].(string)
	var out []string
	for _, l := range strings.Split(v, "\n") {
		l = strings.TrimSpace(l)
		l = strings.TrimPrefix(l, "- ")
		l = strings.TrimPrefix(l, "• ")
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func (t *VisitPrep) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	s := func(k string) string {
		v, _ := input[k].(string)
		return strings.TrimSpace(v)
	}
	chief := s("chief_complaint")
	dept := s("target_department")
	questions := listInput(input, "questions_to_ask")
	if chief == "" || len(questions) == 0 || dept == "" {
		return &ToolResult{Success: false, Error: "需要 chief_complaint、questions_to_ask（每行一条）、target_department 三个字段"}, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# 就诊准备单\n\n生成时间：%s\n\n", time.Now().Format("2006-01-02 15:04"))
	sb.WriteString("> 本单据由 AI 根据对话整理，供就诊沟通参考，不构成诊断。\n\n")

	sb.WriteString("## 基本信息\n")
	if p := s("patient_profile"); p != "" {
		fmt.Fprintf(&sb, "- 患者情况：%s\n", p)
	}
	fmt.Fprintf(&sb, "- 主诉：%s\n", chief)
	fmt.Fprintf(&sb, "- 建议科室：%s\n", dept)

	sections := []struct{ title, key string }{
		{"症状时间线", "symptom_timeline"},
		{"正在使用的药物", "current_meds"},
		{"过敏史", "allergies"},
		{"既往史/慢性病", "past_history"},
		{"近期检查", "recent_tests"},
		{"需向医生说明的警示症状", "red_flags"},
	}
	for _, sec := range sections {
		items := listInput(input, sec.key)
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "\n## %s\n", sec.title)
		for _, it := range items {
			fmt.Fprintf(&sb, "- %s\n", it)
		}
	}

	fmt.Fprintf(&sb, "\n## 想问医生的问题\n")
	for i, q := range questions {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, q)
	}
	sb.WriteString("\n---\n就诊时可按此单顺序向医生说明；医生当面诊断优先于本单内容。\n")

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"markdown":   sb.String(),
			"instruct":   "把上面的 markdown 完整展示给用户，提示可以复制保存或打印，就诊时给医生看。",
			"department": dept,
		},
	}, nil
}
