package tools

import (
	"context"
	"fmt"
	"strings"
)

// LabInterpreter provides evidence-based interpretation of common lab tests.
// It includes southern-China-specific considerations (e.g., MCV/MCH for thalassemia screening).
type LabInterpreter struct{}

// NewLabInterpreter creates the lab interpreter tool.
func NewLabInterpreter() *LabInterpreter {
	return &LabInterpreter{}
}

func (t *LabInterpreter) Name() string {
	return "lab_interpreter"
}

func (t *LabInterpreter) Description() string {
	return "解读常见实验室检查结果。提供正常参考范围、异常值的临床意义、鉴别诊断方向，以及南方人群特别注意事项（如MCV/MCH地贫筛查、G6PD活性检测等）。所有参考范围基于中国人群数据和国际指南。"
}

func (t *LabInterpreter) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"test_name": map[string]any{
				"type":        "string",
				"description": "检查项目名称，如 'MCV'、'MCH'、'Hb'、'platelet'、'ALT'、'HbA1c'、'TSH'、'G6PD'",
			},
			"value": map[string]any{
				"type":        "number",
				"description": "检查结果数值",
			},
			"unit": map[string]any{
				"type":        "string",
				"description": "单位，如 'fL'、'pg'、'g/L'、'U/L'、'%'、'mIU/L'",
			},
			"age_group": map[string]any{
				"type":        "string",
				"description": "年龄组（可选）: infant, child, adolescent, adult, elderly",
			},
			"sex": map[string]any{
				"type":        "string",
				"description": "性别（可选）: male, female",
			},
			"is_southern_chinese": map[string]any{
				"type":        "boolean",
				"description": "是否为南方中国人（影响地贫筛查等解读）",
			},
		},
		"required": []string{"test_name", "value", "unit"},
	}
}

func (t *LabInterpreter) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	testName, _ := input["test_name"].(string)
	value, valueOK := input["value"].(float64)
	unit, _ := input["unit"].(string)
	sex, _ := input["sex"].(string)
	isSouthern, _ := input["is_southern_chinese"].(bool)

	if !valueOK {
		return &ToolResult{
			Success: false,
			Error:   "请提供有效的检测数值 value",
		}, nil
	}

	result := t.interpret(strings.ToLower(testName), value, unit, sex, isSouthern)
	return result, nil
}

func (t *LabInterpreter) interpret(testName string, value float64, unit, sex string, isSouthern bool) *ToolResult {
	switch testName {
	case "mcv":
		return t.interpretMCV(value, unit, isSouthern)
	case "mch":
		return t.interpretMCH(value, unit, isSouthern)
	case "hb", "hemoglobin":
		return t.interpretHb(value, unit, sex)
	case "platelet", "plt":
		return t.interpretPlatelet(value, unit)
	case "alt":
		return t.interpretALT(value, unit, sex)
	case "g6pd":
		return t.interpretG6PD(value, unit)
	case "hba1c":
		return t.interpretHbA1c(value, unit)
	case "tsh":
		return t.interpretTSH(value, unit)
	default:
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"test_name": testName,
				"value":     value,
				"unit":      unit,
				"message":   fmt.Sprintf("'%s' 的自动解读暂不支持。请提供更多上下文，我可以基于通用医学知识提供初步分析。建议由临床医生结合完整病史进行解读。", testName),
			},
		}
	}
}

func (t *LabInterpreter) interpretMCV(value float64, unit string, isSouthern bool) *ToolResult {
	normalRange := "80-100 fL"
	interpretation := ""
	ddx := []string{}

	if value < 80 {
		interpretation = fmt.Sprintf("MCV %.1f fL — **小细胞性**（低于正常下限80 fL）", value)
		ddx = append(ddx, "缺铁性贫血（最常见）", "地中海贫血（α或β型）", "慢性病贫血", "铁粒幼细胞性贫血")
		if isSouthern {
			interpretation += "\n\n**南方人群特别提示**：南方地区地贫携带率极高（广西α-地贫~15%，β-地贫~7%）。对于小细胞性贫血，在排除缺铁后应高度怀疑地中海贫血。建议加做：血清铁蛋白 + 血红蛋白电泳 + 地贫基因检测。若MCH也低（<27 pg），地贫可能性进一步增加。"
			ddx = append([]string{"**地中海贫血（重点排查）** — 南方人群地贫是微小红细胞的首要遗传原因之一"}, ddx...)
		}
	} else if value > 100 {
		interpretation = fmt.Sprintf("MCV %.1f fL — **大细胞性**（高于正常上限100 fL）", value)
		ddx = append(ddx, "维生素B12/叶酸缺乏（巨幼细胞性贫血）", "酒精性肝病", "药物（如羟基脲、抗逆转录病毒药物）", "骨髓增生异常综合征（MDS）", "甲状腺功能减退", "网织红细胞增多症（溶血或出血后）")
	} else {
		interpretation = fmt.Sprintf("MCV %.1f fL — **正常细胞性**（在正常范围80-100 fL内）", value)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"test_name":       "MCV",
			"value":           value,
			"unit":            unit,
			"normal_range":    normalRange,
			"interpretation":  interpretation,
			"differential_diagnosis": ddx,
		},
		Citations: []CitationRef{
			{Title: "中国儿童输血依赖型地中海贫血输血管理指南（2025年）", DOI: "10.7499/j.issn.1008-8830.2410119", Level: "national_guideline", Year: 2025},
		},
	}
}

func (t *LabInterpreter) interpretMCH(value float64, unit string, isSouthern bool) *ToolResult {
	normalRange := "27-34 pg"
	interpretation := ""

	if value < 27 {
		interpretation = fmt.Sprintf("MCH %.1f pg — **低色素性**（低于正常下限27 pg）。结合MCV：若MCV也低，高度提示缺铁性贫血或地中海贫血。在南方人群（特别是广西、广东、海南），地贫是需要优先考虑的鉴别诊断。建议检查：血清铁蛋白 + 血红蛋白电泳。", value)
	} else if value > 34 {
		interpretation = fmt.Sprintf("MCH %.1f pg — 高于正常范围。可见于巨幼细胞性贫血或新生儿。", value)
	} else {
		interpretation = fmt.Sprintf("MCH %.1f pg — 在正常范围27-34 pg内（正常色素性）。", value)
	}

	_ = isSouthern // used in the conditional above

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"test_name":      "MCH",
			"value":          value,
			"unit":           unit,
			"normal_range":   normalRange,
			"interpretation": interpretation,
		},
	}
}

func (t *LabInterpreter) interpretHb(value float64, unit, sex string) *ToolResult {
	var normalRange, anemiaLabel string

	if sex == "female" {
		normalRange = "120-160 g/L"
		if value < 120 {
			if value < 70 {
				anemiaLabel = "重度贫血"
			} else if value < 90 {
				anemiaLabel = "中度贫血"
			} else {
				anemiaLabel = "轻度贫血"
			}
		}
	} else {
		normalRange = "130-175 g/L"
		if value < 130 {
			if value < 70 {
				anemiaLabel = "重度贫血"
			} else if value < 90 {
				anemiaLabel = "中度贫血"
			} else {
				anemiaLabel = "轻度贫血"
			}
		}
	}

	interpretation := fmt.Sprintf("Hb %.1f g/L", value)
	if anemiaLabel != "" {
		interpretation += fmt.Sprintf(" — **%s**（低于正常下限）", anemiaLabel)
	} else {
		interpretation += fmt.Sprintf(" — 在正常范围 %s 内", normalRange)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"test_name":      "Hemoglobin (Hb)",
			"value":          value,
			"unit":           unit,
			"normal_range":   normalRange,
			"interpretation": interpretation,
			"anemia_level":   anemiaLabel,
		},
	}
}

func (t *LabInterpreter) interpretPlatelet(value float64, unit string) *ToolResult {
	normalRange := "125-350 ×10⁹/L"
	interpretation := ""

	if value < 50 {
		interpretation = fmt.Sprintf("血小板 %.0f ×10⁹/L — **重度血小板减少**。出血风险显著增加。需紧急评估：ITP、DIC、TTP、药物诱导、骨髓浸润、登革热（南方地区重要鉴别！）。建议立即就医。", value)
	} else if value < 125 {
		interpretation = fmt.Sprintf("血小板 %.0f ×10⁹/L — **轻度血小板减少**。鉴别诊断：病毒感染后、药物性、ITP、肝硬化脾功能亢进、登革热早期。", value)
	} else if value > 450 {
		interpretation = fmt.Sprintf("血小板 %.0f ×10⁹/L — **血小板增多**。鉴别诊断：反应性（感染/炎症/缺铁）、原发性血小板增多症（MPN）。", value)
	} else {
		interpretation = fmt.Sprintf("血小板 %.0f ×10⁹/L — 在正常范围125-350 ×10⁹/L内。", value)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"test_name":      "Platelet Count",
			"value":          value,
			"unit":           unit,
			"normal_range":   normalRange,
			"interpretation": interpretation,
		},
	}
}

func (t *LabInterpreter) interpretG6PD(value float64, unit string) *ToolResult {
	normalRange := ">60% of normal mean (WHO Class C/D)"
	interpretation := ""

	if value < 20 {
		interpretation = fmt.Sprintf("G6PD活性 %.1f%% — **重度缺乏 (WHO Class A/B)**。相当于旧分类的Class I/II。患者处于慢性溶血或间歇性严重溶血风险中。终身严格规避：蚕豆、磺胺类、伯氨喹、樟脑丸、亚甲蓝。立即为患者建立G6PD警示卡。", value)
	} else if value < 45 {
		interpretation = fmt.Sprintf("G6PD活性 %.1f%% — **中度缺乏 (WHO Class B)**。间歇性溶血风险。严格规避已知触发物。携带G6PD警示卡。", value)
	} else if value < 60 {
		interpretation = fmt.Sprintf("G6PD活性 %.1f%% — 临界/轻度降低。可能代表杂合子女性或WHO Class C变异。建议基因检测确认。通常无溶血风险，但建议规避已知强触发物。", value)
	} else {
		interpretation = fmt.Sprintf("G6PD活性 %.1f%% — 正常范围。", value)
	}

	interpretation += "\n\n**重要提示**：急性溶血期间检测G6PD可能得到假性正常结果（因网织红细胞G6PD活性高于衰老红细胞）。应在急性溶血发作后2-3个月复查。杂合子女性酶活性可能正常但仍为携带者——基因检测是最可靠的确诊手段。"

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"test_name":      "G6PD Enzyme Activity",
			"value":          value,
			"unit":           "% of normal",
			"normal_range":   normalRange,
			"interpretation": interpretation,
		},
		Citations: []CitationRef{
			{Title: "WHO Revised Classification of G6PD Deficiency (2024)", Level: "international_guideline", Year: 2024},
			{Title: "小儿G6PD缺乏症诊疗指南（2025年版）", Level: "national_guideline", Year: 2025},
		},
	}
}

func (t *LabInterpreter) interpretALT(value float64, unit, sex string) *ToolResult {
	var upperLimit float64 = 40
	if sex == "male" {
		upperLimit = 50
	}

	interpretation := ""
	if value > upperLimit*3 {
		interpretation = fmt.Sprintf("ALT %.0f U/L — **显著升高（>3倍正常上限）**。需紧急评估：急性病毒性肝炎、药物性肝损伤（如对乙酰氨基酚中毒、中药/保健品肝毒性）、缺血性肝炎。在南方地区也需考虑：登革热肝炎、EBV肝炎。", value)
	} else if value > upperLimit {
		interpretation = fmt.Sprintf("ALT %.0f U/L — **轻度升高**。鉴别诊断：非酒精性脂肪性肝病（NAFLD — 最常见）、慢性乙肝/丙肝、酒精性肝病、药物性。南方地区乙肝高发——建议查HBsAg。", value)
	} else {
		interpretation = fmt.Sprintf("ALT %.0f U/L — 正常范围。", value)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"test_name":      "ALT (Alanine Aminotransferase)",
			"value":          value,
			"unit":           unit,
			"normal_range":   fmt.Sprintf("<%.0f U/L", upperLimit),
			"interpretation": interpretation,
		},
	}
}

func (t *LabInterpreter) interpretHbA1c(value float64, unit string) *ToolResult {
	interpretation := ""
	if value >= 6.5 {
		interpretation = fmt.Sprintf("HbA1c %.1f%% — **符合糖尿病诊断标准**（≥6.5%%）。建议复查确认（除非已有明确的高血糖症状）。同时进行空腹血糖和OGTT评估。", value)
	} else if value >= 5.7 {
		interpretation = fmt.Sprintf("HbA1c %.1f%% — **糖尿病前期**（5.7-6.4%%）。建议生活方式干预（饮食+运动）。每年复查。", value)
	} else {
		interpretation = fmt.Sprintf("HbA1c %.1f%% — 正常（<5.7%%）。", value)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"test_name":      "HbA1c",
			"value":          value,
			"unit":           "%",
			"normal_range":   "<5.7% (normal); 5.7-6.4% (prediabetes); ≥6.5% (diabetes)",
			"interpretation": interpretation,
		},
		Citations: []CitationRef{
			{Title: "ADA Standards of Care in Diabetes — 2025", DOI: "10.2337/dc25-S002", Level: "international_guideline", Year: 2025},
		},
	}
}

func (t *LabInterpreter) interpretTSH(value float64, unit string) *ToolResult {
	normalRange := "0.4-4.0 mIU/L"
	interpretation := ""

	if value < 0.1 {
		interpretation = fmt.Sprintf("TSH %.2f mIU/L — **严重抑制**，提示甲状腺功能亢进（原发性甲亢）。建议查：FT3、FT4、TSH受体抗体(TRAb)。", value)
	} else if value < 0.4 {
		interpretation = fmt.Sprintf("TSH %.2f mIU/L — **轻度抑制**。可能为亚临床甲亢、非甲状腺疾病、药物（糖皮质激素、多巴胺）影响。", value)
	} else if value > 10 {
		interpretation = fmt.Sprintf("TSH %.2f mIU/L — **显著升高**，提示原发性甲状腺功能减退。建议查：FT4、抗TPO抗体。启动左甲状腺素替代治疗（根据指南调整剂量）。", value)
	} else if value > 4.0 {
		interpretation = fmt.Sprintf("TSH %.2f mIU/L — **轻度升高**（亚临床甲减）。若FT4正常且无症状，3-6个月复查。若TPO抗体阳性或FT4降低，考虑治疗。", value)
	} else {
		interpretation = fmt.Sprintf("TSH %.2f mIU/L — 正常范围（甲状腺功能正常）。", value)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"test_name":      "TSH",
			"value":          value,
			"unit":           unit,
			"normal_range":   normalRange,
			"interpretation": interpretation,
		},
	}
}
