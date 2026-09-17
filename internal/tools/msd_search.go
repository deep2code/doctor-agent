package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// MSDSearch searches the MSD Manual (默沙东诊疗手册) Chinese consumer edition —
// 3300+ full-text Chinese medical pages written by independent experts.
type MSDSearch struct {
	retriever *knowledge.KeywordRetriever
}

// NewMSDSearch creates the MSD manual search tool.
func NewMSDSearch(store *knowledge.Store) *MSDSearch {
	return &MSDSearch{
		retriever: knowledge.NewRetriever(store),
	}
}

func (t *MSDSearch) Name() string {
	return "msd_search"
}

func (t *MSDSearch) Description() string {
	return "在默沙东诊疗手册（中文大众版）中检索疾病科普与诊疗信息。该手册由全球数百位医学专家撰写、独立同行评审，覆盖3000余种疾病的中文全文。输入症状、疾病名或英文关键词，返回匹配的章节全文。当用户询问常见疾病的病因、症状、诊断、治疗时使用。"
}

func (t *MSDSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "检索关键词，如 '荨麻疹'、'低血糖'、'G6PD'、'肾结石'",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "返回章节条数（默认 3，最大 5）",
			},
		},
		"required": []string{"query"},
	}
}

func (t *MSDSearch) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供检索关键词 query"}, nil
	}
	topK := 3
	if v, ok := input["top_k"].(float64); ok && int(v) >= 1 && int(v) <= 5 {
		topK = int(v)
	}

	results, _ := t.retriever.RetrieveMSD(ctx, strings.TrimSpace(query), topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "result_count": 0,
				"message": fmt.Sprintf("默沙东诊疗手册中未找到与 '%s' 直接相关的章节。", query),
			},
		}, nil
	}

	pages := make([]map[string]any, 0, len(results))
	for _, r := range results {
		content := r.Entry.Content
		if len([]rune(content)) > 2000 {
			content = string([]rune(content)[:2000]) + "…"
		}
		pages = append(pages, map[string]any{
			"title":   r.Entry.Title,
			"url":     r.Entry.URL,
			"updated": r.Entry.Updated,
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
