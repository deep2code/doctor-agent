package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// LiteratureSearch searches the embedded Europe PMC literature corpus
// (16 China high-burden topics, 4400+ abstracts with real DOI/PMID).
type LiteratureSearch struct {
	retriever *knowledge.KeywordRetriever
}

// NewLiteratureSearch creates the literature search tool.
func NewLiteratureSearch(store *knowledge.Store) *LiteratureSearch {
	return &LiteratureSearch{
		retriever: knowledge.NewRetriever(store),
	}
}

func (t *LiteratureSearch) Name() string {
	return "literature_search"
}

func (t *LiteratureSearch) Description() string {
	return "在 Europe PMC 文献库中检索医学文献摘要（覆盖地中海贫血、G6PD缺乏症、鼻咽癌、乙肝、登革热、狂犬病、中暑、乳糖不耐受等16个中国重点疾病相关主题，共4400余篇，均含真实DOI/PMID）。输入疾病名或主题词（中文或英文），返回文献列表。当知识库条目需要更强文献证据时使用。"
}

func (t *LiteratureSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "检索主题或疾病关键词，如 '地中海贫血'、'登革热'、'dengue vaccine'、'G6PD'",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "返回文献条数（默认 5，最大 10）",
			},
		},
		"required": []string{"query"},
	}
}

func (t *LiteratureSearch) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{
			Success: false,
			Error:   "请提供检索关键词 query",
		}, nil
	}

	topK := 5
	if v, ok := input["top_k"].(float64); ok && int(v) >= 1 && int(v) <= 10 {
		topK = int(v)
	}

	results, _ := t.retriever.RetrieveLiterature(ctx, strings.TrimSpace(query), topK)

	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query":        query,
				"result_count": 0,
				"message":      fmt.Sprintf("文献库中未找到与 '%s' 直接相关的文献。建议尝试主题词（如 地中海贫血/登革热/乙肝）或英文关键词。", query),
			},
		}, nil
	}

	articles := make([]map[string]any, 0, len(results))
	citations := make([]CitationRef, 0, len(results))
	for _, r := range results {
		e := r.Entry
		abstract := e.Abstract
		if len([]rune(abstract)) > 500 {
			abstract = string([]rune(abstract)[:500]) + "…"
		}
		articles = append(articles, map[string]any{
			"id":            e.ID,
			"topic":         r.Topic.NameZH,
			"title":         e.Title,
			"journal":       e.Journal,
			"year":          e.Year,
			"doi":           e.DOI,
			"pmid":          e.PMID,
			"abstract":      abstract,
			"relevance":     r.Score,
		})
		citations = append(citations, CitationRef{
			ID:    e.ID,
			Title: e.Title,
			DOI:   e.DOI,
			PMID:  e.PMID,
			Year:  e.Year,
			Level: "pubmed_abstract",
		})
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query":        query,
			"result_count": len(results),
			"results":      articles,
		},
		Citations: citations,
	}, nil
}
