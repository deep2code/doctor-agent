package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// EMLLookup queries the WHO Model List of Essential Medicines (24th list,
// 2025): INN name, dosage forms, first/second-choice indications, and
// whether the medicine is on the core or complementary list.
type EMLLookup struct {
	retriever *knowledge.KeywordRetriever
}

// NewEMLLookup creates the EML lookup tool.
func NewEMLLookup(store *knowledge.Store) *EMLLookup {
	return &EMLLookup{
		retriever: knowledge.NewRetriever(store),
	}
}

func (t *EMLLookup) Name() string {
	return "eml_lookup"
}

func (t *EMLLookup) Description() string {
	return "在 WHO 基本药物清单(第24版, 2025, 564种药物)中查询药品：国际非专利名(INN)、剂型规格、一线/二线适应症、属于核心清单还是补充清单。输入英文药名(如 amoxicillin)或中文药名(如 阿莫西林、二甲双胍)，返回 WHO 对该药的推荐定位。当用户询问某药是否为 WHO 推荐的基本药物、或需要权威的适应症与剂型信息时使用。"
}

func (t *EMLLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "药品名，英文 INN 或中文名，如 'amoxicillin'、'阿莫西林'、'二甲双胍'",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "返回条数（默认 5，最大 10）",
			},
		},
		"required": []string{"query"},
	}
}

func (t *EMLLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供药品名 query"}, nil
	}
	topK := 5
	if v, ok := input["top_k"].(float64); ok && int(v) >= 1 && int(v) <= 10 {
		topK = int(v)
	}

	results, _ := t.retriever.RetrieveEMLDrug(ctx, strings.TrimSpace(query), topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "result_count": 0,
				"message": fmt.Sprintf("WHO 基本药物清单(第24版)中未找到与 '%s' 匹配的药品。清单收录 WHO 推荐的必需药物（564种），可在 https://list.essentialmeds.org 查询完整列表。", query),
			},
		}, nil
	}

	entries := make([]map[string]any, 0, len(results))
	for _, r := range results {
		e := r.Entry
		listLabel := "核心清单"
		if e.List == "complementary" {
			listLabel = "补充清单"
		}
		indications := make([]string, 0, len(e.Indications))
		for _, ind := range e.Indications {
			choiceLabel := "一线"
			switch ind.Choice {
			case "second":
				choiceLabel = "二线"
			case "both":
				choiceLabel = "一线/二线"
			}
			indications = append(indications, fmt.Sprintf("%s: %s", choiceLabel, ind.Text))
		}
		entry := map[string]any{
			"name":                    e.Name,
			"name_zh":                 e.NameZH,
			"section":                 e.Section,
			"list":                    listLabel,
			"forms":                   e.Forms,
			"indications":             indications,
			"note":                    e.Note,
			"children_list":           e.Children,
			"square_box_listing":      e.SquareBox,
			"therapeutic_alternatives": e.TherapeuticAlternatives,
		}
		entries = append(entries, entry)
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "result_count": len(results), "results": entries,
		},
	}, nil
}
