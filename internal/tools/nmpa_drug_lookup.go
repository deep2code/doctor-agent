package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// NMPADrugLookup queries the NMPA drug catalogue.
type NMPADrugLookup struct {
	store *knowledge.Store
}

// NewNMPADrugLookup creates the NMPA drug lookup tool.
func NewNMPADrugLookup(store *knowledge.Store) *NMPADrugLookup {
	return &NMPADrugLookup{store: store}
}

func (t *NMPADrugLookup) Name() string {
	return "nmpa_drug_lookup"
}

func (t *NMPADrugLookup) Description() string {
	return "在国家药品编码本位码信息(国家药品监督管理局NMPA)中查询药品信息。输入中文药品名，返回药品编码和来源(国产/进口)。当用户询问某药品的批准文号、药品编码、或查询药品是否经过国家批准时使用。收录167,615种药品(164,474国产+3,141进口)。"
}

func (t *NMPADrugLookup) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "中文药品名，如 '阿莫西林'、'布洛芬'",
			},
		},
		"required": []string{"query"},
	}
}

func (t *NMPADrugLookup) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &ToolResult{Success: false, Error: "请提供药品名 query"}, nil
	}

	query = strings.TrimSpace(query)

	// Direct name lookup
	if d := t.store.GetNMPADrugByName(query); d != nil {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query,
				"result_count": 1,
				"results": []map[string]any{
					{
						"drug_code": d.Code,
						"name_zh":   d.NameZH,
						"source":    d.Source,
					},
				},
			},
		}, nil
	}

	// Name substring search
	drugs := t.store.SearchNMPADrugs(query, 10)
	var matches []map[string]any
	for _, d := range drugs {
		matches = append(matches, map[string]any{
			"drug_code": d.Code,
			"name_zh":   d.NameZH,
			"source":    d.Source,
		})
	}

	if len(matches) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query": query, "result_count": 0,
				"message": fmt.Sprintf("NMPA药品库(167,615种)中未找到 '%s'。可能为中成药、中药饮片或未收录药品。", query),
			},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query": query, "result_count": len(matches), "results": matches,
		},
	}, nil
}
