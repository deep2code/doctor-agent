package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// MedlineSearch searches the MedlinePlus consumer health encyclopedia
// (US National Library of Medicine, English).
type MedlineSearch struct {
	retriever *knowledge.KeywordRetriever
}

// NewMedlineSearch creates the MedlinePlus search tool.
func NewMedlineSearch(store *knowledge.Store) *MedlineSearch {
	return &MedlineSearch{
		retriever: knowledge.NewRetriever(store),
	}
}

func (t *MedlineSearch) Name() string {
	return "medline_search"
}

func (t *MedlineSearch) Description() string {
	return "在 MedlinePlus（美国国家医学图书馆大众健康百科）中检索英文健康科普（1017 个主题，英文全文）。输入英文疾病名或症状，返回匹配的英文章节。当用户用英文提问或需要英文权威科普信息时使用。"
}

func (t *MedlineSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "英文检索关键词，如 'diabetes'、'sickle cell'、'food poisoning'",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "返回章节条数（默认 3，最大 5）",
			},
		},
		"required": []string{"query"},
	}
}

func (t *MedlineSearch) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供检索关键词 query"}, nil
	}
	topK := 3
	if v, ok := input["top_k"].(float64); ok && int(v) >= 1 && int(v) <= 5 {
		topK = int(v)
	}

	results, _ := t.retriever.RetrieveMedlinePlus(ctx, strings.TrimSpace(query), topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "result_count": 0,
				"message": fmt.Sprintf("MedlinePlus 中未找到与 '%s' 直接相关的章节。", query),
			},
		}, nil
	}

	pages := make([]map[string]any, 0, len(results))
	for _, r := range results {
		content := r.Entry.Content
		if len([]rune(content)) > 1500 {
			content = string([]rune(content)[:1500]) + "…"
		}
		pages = append(pages, map[string]any{
			"title":   r.Entry.Title,
			"url":     r.Entry.URL,
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
