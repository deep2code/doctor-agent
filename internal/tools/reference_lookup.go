package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// ReferenceLookup searches the medical literature knowledge base.
type ReferenceLookup struct {
	store     *knowledge.Store
	retriever knowledge.Retriever
}

// NewReferenceLookup creates the reference lookup tool.
func NewReferenceLookup(store *knowledge.Store) *ReferenceLookup {
	return &ReferenceLookup{
		store:     store,
		retriever: knowledge.NewRetriever(store),
	}
}

func (t *ReferenceLookup) Name() string {
	return "reference_lookup"
}

func (t *ReferenceLookup) Description() string {
	return "在循证医学知识库中检索特定疾病或症状相关的文献和指南。输入疾病名称或关键词（中文或英文），返回匹配的医学文献列表（含DOI/PMID和证据等级）。"
}

func (t *ReferenceLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "检索关键词，如 '地中海贫血'、'G6PD deficiency'、'登革热诊断'",
			},
		},
		"required": []string{"query"},
	}
}

func (t *ReferenceLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{
			Success: false,
			Error:   "请提供检索关键词 query",
		}, nil
	}

	query = strings.TrimSpace(query)
	results, _ := t.retriever.Retrieve(ctx, query, 5)

	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query":       query,
				"result_count": 0,
				"message":     fmt.Sprintf("未在知识库中找到与 '%s' 直接相关的文献。建议尝试不同的关键词，或查询PubMed (pubmed.ncbi.nlm.nih.gov) 获取更全面的文献检索。", query),
			},
		}, nil
	}

	// Build references list
	refs := make([]map[string]any, 0, len(results))
	citations := make([]CitationRef, 0)

	for _, result := range results {
		entry := result.Entry
		ref := map[string]any{
			"condition_zh":   entry.ConditionZH,
			"condition_en":   entry.ConditionEN,
			"category":       entry.Category,
			"relevance_score": result.Score,
			"citations":      make([]map[string]any, 0),
		}

		citeList := ref["citations"].([]map[string]any)
		for _, c := range entry.Citations {
			citeList = append(citeList, map[string]any{
				"title":      c.Title,
				"journal":    c.Journal,
				"year":       c.Year,
				"doi":        c.DOI,
				"pmid":       c.PMID,
				"level":      c.Level,
				"level_label": evidenceLevelLabel(c.Level),
			})
			citations = append(citations, CitationRef{
				Title: c.Title,
				DOI:   c.DOI,
				PMID:  c.PMID,
				Level: c.Level,
				Year:  c.Year,
			})
		}
		ref["citations"] = citeList
		refs = append(refs, ref)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query":        query,
			"result_count": len(results),
			"results":      refs,
		},
		Citations: citations,
	}, nil
}

func evidenceLevelLabel(level string) string {
	switch level {
	case "national_guideline":
		return "国家指南 [Grade A-B]"
	case "international_guideline":
		return "国际指南 [Grade A-B]"
	case "meta_analysis":
		return "Meta分析 [Grade A]"
	case "rct":
		return "随机对照试验 [Grade A-B]"
	case "cohort":
		return "队列研究 [Grade B]"
	case "case_control":
		return "病例对照研究 [Grade C]"
	case "case_report":
		return "病例报告 [Grade D]"
	case "review":
		return "综述"
	case "expert_opinion":
		return "专家意见 [Grade D]"
	case "epidemiology":
		return "流行病学研究 [Grade B]"
	default:
		return level
	}
}
