package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// VariantLookup queries the embedded ClinVar subset: pathogenic /
// likely-pathogenic variants of the core China high-burden genes (HBB, HBA1,
// HBA2, G6PD — thalassemia & G6PD deficiency).
type VariantLookup struct {
	retriever *knowledge.KeywordRetriever
}

// NewVariantLookup creates the variant lookup tool.
func NewVariantLookup(store *knowledge.Store) *VariantLookup {
	return &VariantLookup{
		retriever: knowledge.NewRetriever(store),
	}
}

func (t *VariantLookup) Name() string {
	return "variant_lookup"
}

func (t *VariantLookup) Description() string {
	return "在 ClinVar 数据库中检索基因变异（地贫 HBB/HBA1/HBA2、G6PD 缺乏症的致病及可能致病变异，共1400余条）。输入变异名（如 c.79G>A）、基因名（如 HBB、地中海贫血）或疾病名，返回变异、临床意义与相关表型。当用户询问基因检测报告的变异解读时使用。"
}

func (t *VariantLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "变异名/基因名/疾病名，如 'c.79G>A'、'HBB'、'β地中海贫血'、'G6PD'",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "返回条数（默认 10，最大 20）",
			},
		},
		"required": []string{"query"},
	}
}

func (t *VariantLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供检索关键词 query"}, nil
	}
	topK := 10
	if v, ok := input["top_k"].(float64); ok && int(v) >= 1 && int(v) <= 20 {
		topK = int(v)
	}

	results, _ := t.retriever.RetrieveClinVar(ctx, strings.TrimSpace(query), topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "result_count": 0,
				"message": fmt.Sprintf("ClinVar 子集中未找到与 '%s' 匹配的变异。当前收录 HBB/HBA1/HBA2/G6PD 基因的致病及可能致病变异。", query),
			},
		}, nil
	}

	variants := make([]map[string]any, 0, len(results))
	for _, r := range results {
		v := r.Variant
		variants = append(variants, map[string]any{
			"gene":                  v.Gene,
			"variation":             v.Variation,
			"cdna":                  v.Cdna,
			"clinical_significance": v.ClinicalSignificance,
			"traits":                v.Traits,
			"clinvar_id":            v.ClinVarID,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "result_count": len(results), "results": variants,
		},
	}, nil
}
