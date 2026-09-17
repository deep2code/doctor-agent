package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

type DrugInteractionCheckTool struct {
	store *knowledge.Store
}

func NewDrugInteractionCheckTool(store *knowledge.Store) *DrugInteractionCheckTool {
	return &DrugInteractionCheckTool{store: store}
}

func (t *DrugInteractionCheckTool) Name() string {
	return "drug_interaction_check"
}

func (t *DrugInteractionCheckTool) Description() string {
	return "查询两种药物之间是否存在相互作用或共同靶点。基于TTD数据库(4299个靶点+29782种药物)分析药物-靶点关系。"
}

func (t *DrugInteractionCheckTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"drug1": map[string]interface{}{
				"type":        "string",
				"description": "第一种药物名称",
			},
			"drug2": map[string]interface{}{
				"type":        "string",
				"description": "第二种药物名称",
			},
		},
		"required": []string{"drug1", "drug2"},
	}
}

func (t *DrugInteractionCheckTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	drug1, _ := args["drug1"].(string)
	drug2, _ := args["drug2"].(string)

	if drug1 == "" || drug2 == "" {
		return &ToolResult{Success: false, Error: "请提供两种药物名称 drug1 和 drug2"}, nil
	}

	data := t.store.GetTTDData()
	if data == nil {
		return &ToolResult{Success: false, Error: "TTD data not loaded"}, nil
	}

	drug1Lower := strings.ToLower(drug1)
	drug2Lower := strings.ToLower(drug2)

	// Find drug entries
	var found1, found2 *knowledge.TTDDrug
	for i, drug := range data.Drugs {
		nameLower := strings.ToLower(drug.Name)
		if nameLower == drug1Lower {
			found1 = &data.Drugs[i]
		}
		if nameLower == drug2Lower {
			found2 = &data.Drugs[i]
		}
		// Check synonyms
		for _, syn := range drug.Synonyms {
			if strings.ToLower(syn) == drug1Lower {
				found1 = &data.Drugs[i]
			}
			if strings.ToLower(syn) == drug2Lower {
				found2 = &data.Drugs[i]
			}
		}
	}

	if found1 == nil || found2 == nil {
		missing := drug1
		if found2 == nil {
			missing = drug2
		}
		return &ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"message": fmt.Sprintf("未找到药物 '%s' 的信息", missing),
			},
		}, nil
	}

	// Check common targets (intersection of synonyms)
	commonTargets := []string{}
	for _, t1 := range data.Targets {
		for _, t2 := range data.Targets {
			if t1.ID == t2.ID {
				// Check if both drugs target this
				if strings.Contains(strings.ToLower(t1.Name), drug1Lower) ||
					strings.Contains(strings.ToLower(t1.Name), drug2Lower) {
					commonTargets = append(commonTargets, t1.Name)
				}
			}
		}
	}

	// Build response
	var sb strings.Builder
	fmt.Fprintf(&sb, "药物相互作用分析: %s vs %s\n\n", drug1, drug2)

	fmt.Fprintf(&sb, "【%s】信息:\n", drug1)
	if found1.ID != "" {
		fmt.Fprintf(&sb, "  ID: %s\n", found1.ID)
	}
	if len(found1.Synonyms) > 0 {
		fmt.Fprintf(&sb, "  别名: %s\n", strings.Join(found1.Synonyms[:min(3, len(found1.Synonyms))], ", "))
	}
	sb.WriteString("\n")

	fmt.Fprintf(&sb, "【%s】信息:\n", drug2)
	if found2.ID != "" {
		fmt.Fprintf(&sb, "  ID: %s\n", found2.ID)
	}
	if len(found2.Synonyms) > 0 {
		fmt.Fprintf(&sb, "  别名: %s\n", strings.Join(found2.Synonyms[:min(3, len(found2.Synonyms))], ", "))
	}
	sb.WriteString("\n")

	if len(commonTargets) > 0 {
		sb.WriteString("⚠️ 共同靶点:\n")
		for _, target := range commonTargets {
			fmt.Fprintf(&sb, "  - %s\n", target)
		}
		sb.WriteString("\n提示: 两种药物作用于相同靶点，可能存在协同或拮抗作用，建议咨询医生。\n")
	} else {
		sb.WriteString("✅ 未发现直接的共同靶点\n")
		sb.WriteString("提示: 这并不意味着没有其他相互作用，用药前仍需咨询医生。\n")
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
