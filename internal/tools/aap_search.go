package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// AapSearch searches the healthychildren.org (American Academy of Pediatrics)
// parenting encyclopedia — English articles on ages & stages, safety,
// family life, tips & tools.
type AapSearch struct {
	retriever *knowledge.KeywordRetriever
}

// NewAapSearch creates the AAP search tool.
func NewAapSearch(store *knowledge.Store) *AapSearch {
	return &AapSearch{
		retriever: knowledge.NewRetriever(store),
	}
}

func (t *AapSearch) Name() string {
	return "aap_search"
}

func (t *AapSearch) Description() string {
	return "检索美国儿科学会（AAP）healthychildren.org 育儿百科（英文文章：分龄发育里程碑/婴儿护理/安全预防/家庭生活/实用工具）。输入英文关键词（如 'teething'、'newborn feeding'、'car seat safety'），返回 AAP 官方育儿文章。当用户用英文提问育儿话题或需要英文权威育儿参考时使用。"
}

func (t *AapSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "英文检索关键词，如 'breastfeeding'、'diaper rash'、'car seat'",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "返回文章条数（默认 3，最大 5）",
			},
		},
		"required": []string{"query"},
	}
}

func (t *AapSearch) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供检索关键词 query"}, nil
	}
	topK := 3
	if v, ok := input["top_k"].(float64); ok && int(v) >= 1 && int(v) <= 5 {
		topK = int(v)
	}

	results, _ := t.retriever.RetrieveAAP(ctx, strings.TrimSpace(query), topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "result_count": 0,
				"message": fmt.Sprintf("AAP 育儿百科中未找到与 '%s' 直接相关的文章。", query),
			},
		}, nil
	}

	articles := make([]map[string]any, 0, len(results))
	for _, r := range results {
		content := r.Entry.Content
		if len([]rune(content)) > 1500 {
			content = string([]rune(content)[:1500]) + "…"
		}
		articles = append(articles, map[string]any{
			"title":   r.Entry.Title,
			"url":     r.Entry.URL,
			"content": content,
			"relevance": r.Score,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "result_count": len(results), "results": articles,
		},
	}, nil
}
