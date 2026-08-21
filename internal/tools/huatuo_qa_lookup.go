package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

type HuatuoQALookupTool struct {
	store *knowledge.Store
}

func NewHuatuoQALookupTool(store *knowledge.Store) *HuatuoQALookupTool {
	return &HuatuoQALookupTool{store: store}
}

func (t *HuatuoQALookupTool) Name() string {
	return "huatuo_qa_lookup"
}

func (t *HuatuoQALookupTool) Description() string {
	return "搜索华佗26M医疗问答库(177K条)，包含16个科室的真实医患问答。用于查找具体疾病的治疗建议、症状解释和患者教育信息。"
}

func (t *HuatuoQALookupTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "搜索关键词(疾病名/症状/治疗)",
			},
			"department": map[string]interface{}{
				"type":        "string",
				"description": "科室筛选(可选): 妇产科/内科/皮肤性病科/儿科/眼耳鼻喉科/肿瘤科/神经科学/外科/男性健康科/感染与免疫科/口腔科/精神科/骨科/心血管科/内分泌科/中医科",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "返回结果数量(默认5，最多20)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *HuatuoQALookupTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
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

	pairs := t.store.GetHuatuoQA()
	if pairs == nil {
		return &ToolResult{Success: false, Error: "Huatuo QA data not loaded"}, nil
	}

	query = strings.ToLower(query)
	type result struct {
		ID       int
		Question string
		Answer   string
		Dept     string
		Score    int
		Disease  string
	}
	var results []result

	for _, qa := range pairs.QAPairs {
		if dept != "" && qa.Department != dept {
			continue
		}

		score := 0
		q := strings.ToLower(qa.Question)
		a := strings.ToLower(qa.Answer)
		d := strings.ToLower(qa.RelatedDiseases)

		if strings.Contains(q, query) || strings.Contains(d, query) {
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
				if strings.Contains(d, word) {
					score += 2
				}
			}
		}

		if score > 0 {
			results = append(results, result{
				ID:       qa.ID,
				Question: qa.Question,
				Answer:   qa.Answer,
				Dept:     qa.Department,
				Score:    score,
				Disease:  qa.RelatedDiseases,
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
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, r.Dept, truncate(r.Question, 100)))
		sb.WriteString(fmt.Sprintf("   回答: %s\n", truncate(r.Answer, 300)))
		if r.Disease != "" {
			sb.WriteString(fmt.Sprintf("   相关疾病: %s\n", r.Disease))
		}
		sb.WriteString("\n")
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"results": sb.String(),
		},
		Citations: []CitationRef{
			{ID: "huatuo26m-lite", Title: "Huatuo26M-Lite Medical QA Dataset", Level: "community"},
		},
	}, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
