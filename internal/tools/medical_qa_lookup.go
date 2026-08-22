package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

type MedicalQALookupTool struct {
	store *knowledge.Store
}

func NewMedicalQALookupTool(store *knowledge.Store) *MedicalQALookupTool {
	return &MedicalQALookupTool{store: store}
}

func (t *MedicalQALookupTool) Name() string {
	return "medical_qa_lookup"
}

func (t *MedicalQALookupTool) Description() string {
	return "搜索中文医疗问答库(50万条)，包含6个科室(男科/内科/妇产科/儿科/肿瘤科/外科)的真实医患问答。用于查找具体疾病的治疗建议和患者教育信息。"
}

func (t *MedicalQALookupTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "搜索关键词(疾病名/症状/治疗)",
			},
			"department": map[string]interface{}{
				"type":        "string",
				"description": "科室筛选(可选): 男科/内科/妇产科/儿科/肿瘤科/外科",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "返回结果数量(默认5，最多20)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *MedicalQALookupTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return &ToolResult{Success: false, Error: "query is required"}, nil
	}

	dept, _ := args["department"].(string)
	limit := 5
	if l, ok := args["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
		if limit > 20 {
			limit = 20
		}
	}

	data := t.store.GetMedicalQA()
	if data == nil {
		return &ToolResult{Success: false, Error: "Medical QA data not loaded"}, nil
	}

	query = strings.ToLower(query)
	type result struct {
		Question string
		Answer   string
		Dept     string
		Score    int
	}
	var results []result

	for _, qa := range data.QAPairs {
		if dept != "" && qa.Department != dept {
			continue
		}

		score := 0
		q := strings.ToLower(qa.Question)
		a := strings.ToLower(qa.Answer)

		if strings.Contains(q, query) {
			score += 10
		}
		if strings.Contains(a, query) {
			score += 5
		}
		for _, word := range strings.Fields(query) {
			if len(word) >= 2 {
				if strings.Contains(q, word) {
					score += 3
				}
			}
		}

		if score > 0 {
			results = append(results, result{
				Question: qa.Question,
				Answer:   qa.Answer,
				Dept:     qa.Department,
				Score:    score,
			})
		}
	}

	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > limit {
		results = results[:limit]
	}

	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"message": fmt.Sprintf("未找到与'%s'相关的医疗问答", query),
			},
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("找到 %d 条相关问答:\n\n", len(results)))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, r.Dept, truncateStr(r.Question, 100)))
		sb.WriteString(fmt.Sprintf("   回答: %s\n\n", truncateStr(r.Answer, 300)))
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"results": sb.String(),
		},
		Citations: []CitationRef{
			{ID: "medical-qa", Title: "Chinese Medical Dialogue Dataset", Level: "community"},
		},
	}, nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
