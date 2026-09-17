package tools

import (
	"context"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// MilestoneLookup returns the CDC developmental milestone checklist for a
// child's age, or searches milestone items across ages by keyword.
type MilestoneLookup struct {
	retriever *knowledge.KeywordRetriever
}

// NewMilestoneLookup creates the milestone lookup tool.
func NewMilestoneLookup(store *knowledge.Store) *MilestoneLookup {
	return &MilestoneLookup{retriever: knowledge.NewRetriever(store)}
}

func (t *MilestoneLookup) Name() string { return "milestone_lookup" }

func (t *MilestoneLookup) Description() string {
	return "儿童发育里程碑查询（CDC Learn the Signs. Act Early. 2022 修订版）：按月龄返回该月龄段大多数(75%)儿童应达到的发育里程碑清单（社交情绪/语言沟通/认知/运动四类，中英双语），或按关键词（如 '走路'、'说话'、'微笑'）搜索里程碑出现的月龄。当家长询问孩子发育是否正常、几个月会坐会走会说话、发育是否落后时使用。覆盖 2 月龄至 5 周岁。"
}

func (t *MilestoneLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"age_months": map[string]any{
				"type":        "integer",
				"description": "月龄（0-71）。提供时返回该月龄对应的里程碑清单",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "可选：里程碑关键词搜索（如 '走路'、'叫妈妈'、'追视'），跨所有年龄返回匹配条目及出现月龄",
			},
		},
	}
}

func (t *MilestoneLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	ageMonths := -1
	if v, ok := input["age_months"].(float64); ok {
		ageMonths = int(v)
	}
	query, _ := input["query"].(string)
	query = strings.TrimSpace(query)

	if ageMonths < 0 && query == "" {
		return &ToolResult{Success: false, Error: "请提供 age_months（月龄）或 query（里程碑关键词）"}, nil
	}

	data := map[string]any{}
	if ageMonths >= 0 {
		res, err := t.retriever.RetrieveMilestones(ctx, ageMonths)
		if err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		data["checklist"] = res
	}
	if query != "" {
		matches, err := t.retriever.SearchMilestones(ctx, query, 8)
		if err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		data["matches"] = matches
		data["match_count"] = len(matches)
	}
	data["definition"] = "发育里程碑：大多数（75% 或更多）儿童在该年龄能做到的行为；若孩子未达成某项、丧失已会技能或家长有担忧，应尽早与医生沟通并要求发育筛查"
	return &ToolResult{
		Success: true,
		Data:    data,
		Citations: []CitationRef{
			{ID: "cdc-milestones", Title: "CDC Developmental Milestones (Learn the Signs. Act Early., 2022 revision)", Level: "public_health_authority", Year: 2022},
		},
	}, nil
}
