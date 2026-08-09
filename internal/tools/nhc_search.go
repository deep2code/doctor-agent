package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// NhcSearch searches the 国家卫健委 (National Health Commission) official
// 诊疗方案/诊疗指南 full texts — Chinese, authoritative, government-published.
type NhcSearch struct {
	retriever *knowledge.KeywordRetriever
}

// NewNhcSearch creates the NHC guideline search tool.
func NewNhcSearch(store *knowledge.Store) *NhcSearch {
	return &NhcSearch{
		retriever: knowledge.NewRetriever(store),
	}
}

func (t *NhcSearch) Name() string {
	return "nhc_search"
}

func (t *NhcSearch) Description() string {
	return "检索中国国家卫健委（NHC）官方诊疗方案与诊疗指南全文（39 篇中文：流感/脑血管病/肝癌/诺如病毒/猴痘/拉沙热/基孔肯雅热/儿童支原体肺炎/新冠/罕见病等）。输入疾病名或症状，返回官方指南的病因、诊断、治疗、预防等原文。当用户询问中国官方诊疗标准、某病在中国如何诊治、或需要国家级指南依据时使用。"
}

func (t *NhcSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "检索关键词，如 '流感'、'脑血管病'、'诺如病毒'、'H1N1'",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "返回指南条数（默认 3，最大 5）",
			},
		},
		"required": []string{"query"},
	}
}

func (t *NhcSearch) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供检索关键词 query"}, nil
	}
	topK := 3
	if v, ok := input["top_k"].(float64); ok && int(v) >= 1 && int(v) <= 5 {
		topK = int(v)
	}

	results, _ := t.retriever.RetrieveNHCGuide(ctx, strings.TrimSpace(query), topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "result_count": 0,
				"message": fmt.Sprintf("国家卫健委指南库中未找到与 '%s' 直接相关的诊疗方案/指南。", query),
			},
		}, nil
	}

	guides := make([]map[string]any, 0, len(results))
	for _, r := range results {
		content := r.Guide.Content
		if len([]rune(content)) > 2000 {
			content = string([]rune(content)[:2000]) + "…"
		}
		guides = append(guides, map[string]any{
			"title":   r.Guide.Title,
			"url":     r.Guide.URL,
			"year":    r.Guide.Year,
			"source":  r.Guide.Source,
			"content": content,
			"relevance": r.Score,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "result_count": len(results), "results": guides,
		},
	}, nil
}
