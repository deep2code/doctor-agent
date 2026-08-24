package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

type TTDLookupTool struct {
	store *knowledge.Store
}

func NewTTDLookupTool(store *knowledge.Store) *TTDLookupTool {
	return &TTDLookupTool{store: store}
}

func (t *TTDLookupTool) Name() string {
	return "ttd_lookup"
}

func (t *TTDLookupTool) Description() string {
	return "查询TTD治疗靶点数据库(4299个靶点+29782种药物)。用于查找药物靶点信息、药物别名和靶点类型。"
}

func (t *TTDLookupTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "搜索关键词(药物名/靶点名/基因名)",
			},
			"type": map[string]interface{}{
				"type":        "string",
				"description": "搜索类型(可选): drug/target",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "返回结果数量(默认5，最多20)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *TTDLookupTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return &ToolResult{Success: false, Error: "query is required"}, nil
	}

	searchType, _ := args["type"].(string)
	limit := 5
	if l, ok := args["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
		if limit > 20 {
			limit = 20
		}
	}

	data := t.store.GetTTDData()
	if data == nil {
		return &ToolResult{Success: false, Error: "TTD data not loaded"}, nil
	}

	query = strings.ToLower(query)
	var targetResults []knowledge.TTDTarget
	var drugResults []knowledge.TTDDrug

	// Search targets
	if searchType == "" || searchType == "target" {
		for _, target := range data.Targets {
			if strings.Contains(strings.ToLower(target.Name), query) ||
				strings.Contains(strings.ToLower(target.Uniprot), query) {
				targetResults = append(targetResults, target)
				if len(targetResults) >= limit {
					break
				}
			}
		}
	}

	// Search drugs
	if searchType == "" || searchType == "drug" {
		for _, drug := range data.Drugs {
			if strings.Contains(strings.ToLower(drug.Name), query) {
				drugResults = append(drugResults, drug)
				if len(drugResults) >= limit {
					break
				}
			}
			for _, syn := range drug.Synonyms {
				if strings.Contains(strings.ToLower(syn), query) {
					drugResults = append(drugResults, drug)
					if len(drugResults) >= limit {
						break
					}
					break
				}
			}
			if len(drugResults) >= limit {
				break
			}
		}
	}

	if len(targetResults) == 0 && len(drugResults) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"message": fmt.Sprintf("未找到与'%s'相关的TTD数据", query),
			},
		}, nil
	}

	var sb strings.Builder

	if len(targetResults) > 0 {
		fmt.Fprintf(&sb, "找到 %d 个靶点:\n\n", len(targetResults))
		for i, t := range targetResults {
			fmt.Fprintf(&sb, "%d. %s (%s)\n", i+1, t.Name, t.ID)
			if t.Uniprot != "" {
				fmt.Fprintf(&sb, "   UniProt: %s\n", t.Uniprot)
			}
			if t.Type != "" {
				fmt.Fprintf(&sb, "   类型: %s\n", t.Type)
			}
			sb.WriteString("\n")
		}
	}

	if len(drugResults) > 0 {
		fmt.Fprintf(&sb, "找到 %d 种药物:\n\n", len(drugResults))
		for i, d := range drugResults {
			fmt.Fprintf(&sb, "%d. %s (%s)\n", i+1, d.Name, d.ID)
			if len(d.Synonyms) > 0 {
				fmt.Fprintf(&sb, "   别名: %s\n", strings.Join(d.Synonyms[:min(5, len(d.Synonyms))], ", "))
			}
			sb.WriteString("\n")
		}
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"results": sb.String(),
		},
		Citations: []CitationRef{
			{ID: "ttd", Title: "Therapeutic Target Database", Level: "database"},
		},
	}, nil
}
