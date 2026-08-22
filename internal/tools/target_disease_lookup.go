package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

type TargetDiseaseLookupTool struct {
	store *knowledge.Store
}

func NewTargetDiseaseLookupTool(store *knowledge.Store) *TargetDiseaseLookupTool {
	return &TargetDiseaseLookupTool{store: store}
}

func (t *TargetDiseaseLookupTool) Name() string {
	return "target_disease_lookup"
}

func (t *TargetDiseaseLookupTool) Description() string {
	return "查询基因/靶点相关的疾病信息。基于TTD数据库(4299个靶点)和ICD-10疾病分类(35862种疾病)。"
}

func (t *TargetDiseaseLookupTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target": map[string]interface{}{
				"type":        "string",
				"description": "基因或靶点名称(如: EGFR, VEGFR, BRCA1)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "返回结果数量(默认5，最多20)",
			},
		},
		"required": []string{"target"},
	}
}

func (t *TargetDiseaseLookupTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	target, _ := args["target"].(string)
	if target == "" {
		return &ToolResult{Success: false, Error: "请提供靶点名称 target"}, nil
	}

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

	targetLower := strings.ToLower(target)
	var matchedTargets []knowledge.TTDTarget

	for _, t := range data.Targets {
		if strings.Contains(strings.ToLower(t.Name), targetLower) ||
			strings.Contains(strings.ToLower(t.Uniprot), targetLower) ||
			strings.Contains(strings.ToLower(t.Type), targetLower) {
			matchedTargets = append(matchedTargets, t)
			if len(matchedTargets) >= limit {
				break
			}
		}
	}

	if len(matchedTargets) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"message": fmt.Sprintf("未找到与'%s'相关的靶点信息", target),
			},
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("靶点「%s」相关信息:\n\n", target))

	for i, t := range matchedTargets {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, t.Name))
		if t.ID != "" {
			sb.WriteString(fmt.Sprintf("   ID: %s\n", t.ID))
		}
		if t.Uniprot != "" {
			sb.WriteString(fmt.Sprintf("   UniProt: %s\n", t.Uniprot))
		}
		if t.Type != "" {
			sb.WriteString(fmt.Sprintf("   类型: %s\n", t.Type))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("提示: 靶点信息仅供参考，具体药物研发和临床应用请咨询专业人员。")

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"results": sb.String(),
			"count":   len(matchedTargets),
		},
		Citations: []CitationRef{
			{ID: "ttd", Title: "Therapeutic Target Database", Level: "database"},
		},
	}, nil
}
