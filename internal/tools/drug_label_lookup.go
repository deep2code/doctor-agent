package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// DrugLabelLookup queries curated FDA drug labels (Chinese summaries derived
// from DailyMed/OpenFDA): indications, contraindications, warnings,
// interactions, adverse reactions, and dosage.
type DrugLabelLookup struct {
	retriever *knowledge.KeywordRetriever
}

// NewDrugLabelLookup creates the drug-label lookup tool.
func NewDrugLabelLookup(store *knowledge.Store) *DrugLabelLookup {
	return &DrugLabelLookup{
		retriever: knowledge.NewRetriever(store),
	}
}

func (t *DrugLabelLookup) Name() string {
	return "drug_label_lookup"
}

func (t *DrugLabelLookup) Description() string {
	return "查询美国 FDA 药品标签的中文要点（源自 DailyMed/OpenFDA，覆盖 WHO 基本药物清单常用药）：适应症、禁忌、警告与注意事项、药物相互作用、常见不良反应、常规剂量。输入中文或英文药品名（如 阿莫西林、amoxicillin、二甲双胍），返回该药官方标签要点。当用户询问某药'能不能吃/有什么副作用/和什么药冲突/怎么吃'时优先使用。"
}

func (t *DrugLabelLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "药品名（中文或英文），如 '阿莫西林'、'amoxicillin'、'二甲双胍'",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "返回条数（默认 3，最大 5）",
			},
		},
		"required": []string{"query"},
	}
}

func (t *DrugLabelLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供药品名 query"}, nil
	}
	topK := 3
	if v, ok := input["top_k"].(float64); ok && int(v) >= 1 && int(v) <= 5 {
		topK = int(v)
	}

	results, _ := t.retriever.RetrieveFDALabel(ctx, strings.TrimSpace(query), topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "result_count": 0,
				"message": fmt.Sprintf("FDA 标签库中未找到与 '%s' 匹配的药品。当前收录 WHO 基本药物清单对应药品的 FDA 标签中文要点。", query),
			},
		}, nil
	}

	drugs := make([]map[string]any, 0, len(results))
	for _, r := range results {
		d := r.Drug
		drugs = append(drugs, map[string]any{
			"name_zh":           d.NameZH,
			"name_en":           d.NameEN,
			"category":          d.Category,
			"indications":       d.Indications,
			"contraindications": d.Contraindications,
			"warnings":          d.Warnings,
			"interactions":      d.Interactions,
			"adverse_reactions": d.AdverseReactions,
			"dosage":            d.Dosage,
			"source_url":        d.SourceURL,
		})
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "result_count": len(results), "results": drugs,
		},
	}, nil
}
