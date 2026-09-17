package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// DrugLookup queries the national medical-insurance drug catalogue
// (国家医保药品目录 2024): Chinese drug name -> insurance category/forms.
type DrugLookup struct {
	retriever *knowledge.KeywordRetriever
}

// NewDrugLookup creates the drug lookup tool.
func NewDrugLookup(store *knowledge.Store) *DrugLookup {
	return &DrugLookup{
		retriever: knowledge.NewRetriever(store),
	}
}

func (t *DrugLookup) Name() string {
	return "drug_lookup"
}

func (t *DrugLookup) Description() string {
	return "在国家医保药品目录(2024年版, 1170种西药)中查询药品信息：中文药名、医保类别(甲类全额报销/乙类自付部分)、剂型。输入中文药品名(如 阿莫西林、二甲双胍、布洛芬)，返回是否在目录内及医保分类。当用户询问某药是否纳入医保/医保类别时使用。"
}

func (t *DrugLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "中文药品名，如 '阿莫西林'、'二甲双胍'",
			},
		},
		"required": []string{"query"},
	}
}

func (t *DrugLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供药品名 query"}, nil
	}

	results, _ := t.retriever.RetrieveMedinsDrug(ctx, strings.TrimSpace(query), 5)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "result_count": 0,
				"message": fmt.Sprintf("国家医保药品目录(2024)中未找到 '%s'。可能为目录外药品、中成药或中药饮片(当前仅收录西药部分)。", query),
			},
		}, nil
	}

	drugs := make([]map[string]any, 0, len(results))
	for _, r := range results {
		cat := r.Drug.Category
		catDesc := "乙类(需自付一定比例)"
		if cat == "甲" {
			catDesc = "甲类(全额纳入医保报销)"
		}
		drugs = append(drugs, map[string]any{
			"name":       r.Drug.Name,
			"category":   cat,
			"category_desc": catDesc,
			"forms":      r.Drug.Forms,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "result_count": len(results), "results": drugs,
		},
	}, nil
}
