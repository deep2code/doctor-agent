package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// DrugSafetyCheck queries the G6PD drug contraindication database.
type DrugSafetyCheck struct {
	store *knowledge.Store
}

// NewDrugSafetyCheck creates the drug safety tool.
func NewDrugSafetyCheck(store *knowledge.Store) *DrugSafetyCheck {
	return &DrugSafetyCheck{store: store}
}

func (t *DrugSafetyCheck) Name() string {
	return "drug_safety_check"
}

func (t *DrugSafetyCheck) Description() string {
	return "查询药物/食物/化学品对G6PD缺乏症患者的安全性。输入药物名称（中文或英文通用名或商品名），返回安全等级（safe/unsafe/caution/unknown）、风险机制、证据等级和替代方案建议。"
}

func (t *DrugSafetyCheck) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"drug_name": map[string]any{
				"type":        "string",
				"description": "药物名称（中文或英文通用名或商品名），如 '阿司匹林'、'primaquine'、'Bactrim'、'蚕豆'",
			},
		},
		"required": []string{"drug_name"},
	}
}

func (t *DrugSafetyCheck) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	drugName, ok := input["drug_name"].(string)
	if !ok || strings.TrimSpace(drugName) == "" {
		return &ToolResult{
			Success: false,
			Error:   "请提供药物名称参数 drug_name",
		}, nil
	}

	drugName = strings.TrimSpace(drugName)

	// Search drug database
	entry := t.store.GetDrugByName(drugName)
	if entry == nil {
		// Try case-insensitive search across all drugs
		for _, d := range t.store.GetAllDrugs() {
			if strings.EqualFold(d.GenericNameEN, drugName) ||
				strings.EqualFold(d.GenericNameZH, drugName) {
				entry = &d
				break
			}
			for _, tn := range d.TradeNames {
				if strings.EqualFold(tn, drugName) {
					entry = &d
					break
				}
			}
			if entry != nil {
				break
			}
		}
	}

	if entry == nil {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"drug_name":      drugName,
				"g6pd_safety":    "unknown",
				"risk_level":     "unknown",
				"message":        fmt.Sprintf("未在G6PD禁忌药物数据库中找到 '%s'。请核对药物名称是否正确，或咨询药师获取更完整的药物安全性信息。", drugName),
				"recommendation": "建议在使用前通过药师或药品说明书确认G6PD安全性。如果患者已知为G6PD缺乏，应优先选择已知安全的替代药物。",
			},
		}, nil
	}

	result := &ToolResult{
		Success: true,
		Data: map[string]any{
			"drug_name":        drugName,
			"generic_name_zh":  entry.GenericNameZH,
			"generic_name_en":  entry.GenericNameEN,
			"drug_class":       entry.DrugClass,
			"g6pd_safety":      entry.G6PDSafety,
			"risk_level":       entry.RiskLevel,
			"mechanism":        entry.Mechanism,
			"alternatives":     entry.Alternatives,
			"recommendation":   t.buildRecommendation(entry),
		},
	}

	// Add citations
	for _, c := range entry.Citations {
		result.Citations = append(result.Citations, CitationRef{
			Title: c.Title,
			DOI:   c.DOI,
			PMID:  c.PMID,
			Level: c.Level,
			Year:  c.Year,
		})
	}

	return result, nil
}

func (t *DrugSafetyCheck) buildRecommendation(entry *knowledge.DrugEntry) string {
	switch entry.G6PDSafety {
	case knowledge.G6PDUnsafe:
		return fmt.Sprintf("**禁止使用**。%s 对G6PD缺乏症患者不安全（风险等级：%s）。机制：%s。替代方案：%s。",
			entry.GenericNameZH, entry.RiskLevel, entry.Mechanism, strings.Join(entry.Alternatives, "、"))
	case knowledge.G6PDCaution:
		return fmt.Sprintf("**谨慎使用**。%s 在特定情况下可能引起溶血（风险等级：%s）。机制：%s。使用前必须评估获益/风险比，并在医生监督下使用。替代方案：%s。",
			entry.GenericNameZH, entry.RiskLevel, entry.Mechanism, strings.Join(entry.Alternatives, "、"))
	case knowledge.G6PDSafe:
		return fmt.Sprintf("**安全使用**。%s 在G6PD缺乏症患者中被认为是安全的。",
			entry.GenericNameZH)
	default:
		return fmt.Sprintf("**安全性未知**。%s 的G6PD安全性数据不充分。建议在使用前咨询药师或查阅最新药品说明书。",
			entry.GenericNameZH)
	}
}
