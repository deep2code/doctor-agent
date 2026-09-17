package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// FhsSearch searches the 香港卫生署家庭健康服务 (Family Health Service) parenting
// corpus — 100+ Simplified Chinese pages on breastfeeding, child care, sleep
// safety, feeding, development and parenting skills.
type FhsSearch struct {
	retriever *knowledge.KeywordRetriever
}

// NewFhsSearch creates the FHS search tool.
func NewFhsSearch(store *knowledge.Store) *FhsSearch {
	return &FhsSearch{
		retriever: knowledge.NewRetriever(store),
	}
}

func (t *FhsSearch) Name() string {
	return "fhs_search"
}

func (t *FhsSearch) Description() string {
	return "检索香港卫生署家庭健康服务（FHS）育儿知识库（103 页简体中文：母乳喂哺/辅食添加/婴儿睡眠安全/儿童发育/亲子沟通/家居安全/常见疑问）。输入育儿问题（如 '宝宝怎么加辅食'、'婴儿睡姿'、'宝宝发烧怎么办'），返回香港卫生署官方育儿资料全文。当用户询问婴幼儿喂养、睡眠、发育、亲子技巧等育儿问题时使用。"
}

func (t *FhsSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "检索关键词，如 '母乳'、'辅食'、'婴儿睡眠'、'宝宝发烧'",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "返回页面条数（默认 3，最大 5）",
			},
		},
		"required": []string{"query"},
	}
}

func (t *FhsSearch) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供检索关键词 query"}, nil
	}
	topK := 3
	if v, ok := input["top_k"].(float64); ok && int(v) >= 1 && int(v) <= 5 {
		topK = int(v)
	}

	results, _ := t.retriever.RetrieveFHSGuide(ctx, strings.TrimSpace(query), topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "result_count": 0,
				"message": fmt.Sprintf("香港卫生署育儿知识库中未找到与 '%s' 直接相关的页面。", query),
			},
		}, nil
	}

	pages := make([]map[string]any, 0, len(results))
	for _, r := range results {
		content := r.Guide.Content
		if len([]rune(content)) > 2000 {
			content = string([]rune(content)[:2000]) + "…"
		}
		pages = append(pages, map[string]any{
			"title":   r.Guide.Title,
			"url":     r.Guide.URL,
			"content": content,
			"relevance": r.Score,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "result_count": len(results), "results": pages,
		},
	}, nil
}
