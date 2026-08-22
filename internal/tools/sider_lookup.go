package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

type SIDERLookupTool struct {
	store *knowledge.Store
}

func NewSIDERLookupTool(store *knowledge.Store) *SIDERLookupTool {
	return &SIDERLookupTool{store: store}
}

func (t *SIDERLookupTool) Name() string {
	return "sider_lookup"
}

func (t *SIDERLookupTool) Description() string {
	return "查询药物副作用和适应症信息。基于SIDER数据库(1430种药物, 5880种副作用)。"
}

func (t *SIDERLookupTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"drug_id": map[string]interface{}{
				"type":        "string",
				"description": "药物ID(如: CID100000085)或药物名称",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "返回结果数量(默认1，最多5)",
			},
		},
		"required": []string{"drug_id"},
	}
}

func (t *SIDERLookupTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	drugID, _ := args["drug_id"].(string)
	if drugID == "" {
		return &ToolResult{Success: false, Error: "请提供药物ID drug_id"}, nil
	}

	limit := 1
	if l, ok := args["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
		if limit > 5 {
			limit = 5
		}
	}

	data := t.store.GetSIDERData()
	if data == nil {
		return &ToolResult{Success: false, Error: "SIDER data not loaded"}, nil
	}

	drugIDLower := strings.ToLower(drugID)
	var matches []knowledge.SIDERDrug

	for _, drug := range data.Drugs {
		if strings.Contains(strings.ToLower(drug.ID), drugIDLower) {
			matches = append(matches, drug)
			if len(matches) >= limit {
				break
			}
		}
	}

	if len(matches) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"message": fmt.Sprintf("未找到药物'%s'的SIDER数据", drugID),
			},
		}, nil
	}

	var sb strings.Builder
	sb.WriteString("SIDER药物信息:\n\n")

	for i, drug := range matches {
		sb.WriteString(fmt.Sprintf("%d. 药物ID: %s\n", i+1, drug.ID))

		if len(drug.SideEffects) > 0 {
			sb.WriteString("   常见副作用:\n")
			for _, se := range drug.SideEffects[:min(10, len(drug.SideEffects))] {
				sb.WriteString(fmt.Sprintf("   - %s\n", se))
			}
		}

		if len(drug.Indications) > 0 {
			sb.WriteString("   适应症:\n")
			for _, ind := range drug.Indications[:min(5, len(drug.Indications))] {
				sb.WriteString(fmt.Sprintf("   - %s\n", ind))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("提示: 副作用信息仅供参考，具体用药请遵医嘱。")

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"results": sb.String(),
			"count":   len(matches),
		},
		Citations: []CitationRef{
			{ID: "sider", Title: "SIDER Side Effect Resource", Level: "database"},
		},
	}, nil
}
