package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// NewbornCareLookup searches WHO preterm/low-birth-weight care
// recommendations and China newborn screening programme knowledge.
type NewbornCareLookup struct {
	retriever *knowledge.KeywordRetriever
}

// NewNewbornCareLookup creates the newborn care lookup tool.
func NewNewbornCareLookup(store *knowledge.Store) *NewbornCareLookup {
	return &NewbornCareLookup{retriever: knowledge.NewRetriever(store)}
}

func (t *NewbornCareLookup) Name() string { return "newborn_care_lookup" }

func (t *NewbornCareLookup) Description() string {
	return "新生儿/早产儿护理与筛查知识检索：① WHO《早产或低出生体重儿护理建议》(2022, 26条推荐：袋鼠式护理/母乳喂养/铁锌维D补充/CPAP/咖啡因呼吸暂停/家庭参与等，中英双语含推荐强度)；② 中国新生儿疾病筛查项目（足跟血遗传代谢病筛查/听力筛查/先天性心脏病脉搏血氧筛查的时间窗与方法）。当询问早产儿护理、低出生体重儿喂养、袋鼠式护理、新生儿筛查项目、足跟血、听力复筛等问题时使用。"
}

func (t *NewbornCareLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "检索词，如 '袋鼠式护理'、'早产儿 母乳'、'咖啡因 呼吸暂停'、'足跟血'、'听力筛查'、'先心 筛查'",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "返回条数（默认 5，最大 8）",
			},
		},
		"required": []string{"query"},
	}
}

func (t *NewbornCareLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供检索词 query"}, nil
	}
	topK := 5
	if v, ok := input["top_k"].(float64); ok && int(v) >= 1 && int(v) <= 8 {
		topK = int(v)
	}

	results, _ := t.retriever.SearchNewbornCare(ctx, strings.TrimSpace(query), topK)
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "result_count": 0,
				"message": fmt.Sprintf("新生儿知识库中未找到与 '%s' 直接相关的条目。", query),
			},
		}, nil
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "result_count": len(results), "results": results,
		},
		Citations: []CitationRef{
			{ID: "who-preterm-lbw-2022", Title: "WHO recommendations for care of the preterm or low-birth-weight infant", Level: "who_guideline", Year: 2022},
			{ID: "cn-nbs", Title: "新生儿疾病筛查管理办法/新生儿先天性心脏病筛查项目", Level: "national_policy", Year: 2009},
		},
	}, nil
}
