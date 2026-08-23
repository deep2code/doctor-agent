package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

type LabReportInterpretTool struct {
	store *knowledge.Store
}

func NewLabReportInterpretTool(store *knowledge.Store) *LabReportInterpretTool {
	return &LabReportInterpretTool{store: store}
}

func (t *LabReportInterpretTool) Name() string {
	return "lab_report_interpret"
}

func (t *LabReportInterpretTool) Description() string {
	return "解读检验报告，分析各项指标的含义、正常范围和异常提示。支持血常规、肝肾功能、血糖血脂等常见检验项目。"
}

func (t *LabReportInterpretTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"test_name": map[string]interface{}{
				"type":        "string",
				"description": "检验项目名称(如: 血常规、肝功能、肾功能)",
			},
			"results": map[string]interface{}{
				"type":        "string",
				"description": "检验结果，格式: 项目名=值,项目名=值 (如: 白细胞=12.5,血红蛋白=95)",
			},
			"gender": map[string]interface{}{
				"type":        "string",
				"description": "患者性别(可选，影响参考范围)",
				"enum":        []string{"男", "女"},
			},
		},
		"required": []string{"test_name", "results"},
	}
}

// Normal ranges for common lab tests
var labReferenceRanges = map[string]map[string]struct {
	Min    float64
	Max    float64
	Unit   string
	MaleMin   float64
	MaleMax   float64
	FemaleMin float64
	FemaleMax float64
}{
	"血常规": {
		"白细胞":   {Min: 4, Max: 10, Unit: "×10⁹/L", MaleMin: 4, MaleMax: 10, FemaleMin: 4, FemaleMax: 10},
		"红细胞":   {Min: 4, Max: 5.5, Unit: "×10¹²/L", MaleMin: 4, MaleMax: 5.5, FemaleMin: 3.5, FemaleMax: 5},
		"血红蛋白": {Min: 120, Max: 160, Unit: "g/L", MaleMin: 120, MaleMax: 160, FemaleMin: 110, FemaleMax: 150},
		"血小板":   {Min: 100, Max: 300, Unit: "×10⁹/L", MaleMin: 100, MaleMax: 300, FemaleMin: 100, FemaleMax: 300},
		"中性粒细胞": {Min: 50, Max: 70, Unit: "%", MaleMin: 50, MaleMax: 70, FemaleMin: 50, FemaleMax: 70},
		"淋巴细胞": {Min: 20, Max: 40, Unit: "%", MaleMin: 20, MaleMax: 40, FemaleMin: 20, FemaleMax: 40},
	},
	"肝功能": {
		"谷丙转氨酶": {Min: 0, Max: 40, Unit: "U/L", MaleMin: 0, MaleMax: 40, FemaleMin: 0, FemaleMax: 40},
		"谷草转氨酶": {Min: 0, Max: 40, Unit: "U/L", MaleMin: 0, MaleMax: 40, FemaleMin: 0, FemaleMax: 40},
		"总胆红素":   {Min: 3.4, Max: 17.1, Unit: "μmol/L", MaleMin: 3.4, MaleMax: 17.1, FemaleMin: 3.4, FemaleMax: 17.1},
		"直接胆红素": {Min: 0, Max: 6.8, Unit: "μmol/L", MaleMin: 0, MaleMax: 6.8, FemaleMin: 0, FemaleMax: 6.8},
		"白蛋白":     {Min: 35, Max: 55, Unit: "g/L", MaleMin: 35, MaleMax: 55, FemaleMin: 35, FemaleMax: 55},
		"球蛋白":     {Min: 20, Max: 40, Unit: "g/L", MaleMin: 20, MaleMax: 40, FemaleMin: 20, FemaleMax: 40},
	},
	"肾功能": {
		"肌酐":     {Min: 53, Max: 106, Unit: "μmol/L", MaleMin: 53, MaleMax: 106, FemaleMin: 44, FemaleMax: 80},
		"尿素氮":   {Min: 2.9, Max: 8.2, Unit: "mmol/L", MaleMin: 2.9, MaleMax: 8.2, FemaleMin: 2.9, FemaleMax: 8.2},
		"尿酸":     {Min: 149, Max: 416, Unit: "μmol/L", MaleMin: 149, MaleMax: 416, FemaleMin: 89, FemaleMax: 357},
		"胱抑素C":  {Min: 0.51, Max: 1.09, Unit: "mg/L", MaleMin: 0.51, MaleMax: 1.09, FemaleMin: 0.51, FemaleMax: 1.09},
	},
	"血糖": {
		"空腹血糖": {Min: 3.9, Max: 6.1, Unit: "mmol/L", MaleMin: 3.9, MaleMax: 6.1, FemaleMin: 3.9, FemaleMax: 6.1},
		"餐后2小时血糖": {Min: 3.9, Max: 7.8, Unit: "mmol/L", MaleMin: 3.9, MaleMax: 7.8, FemaleMin: 3.9, FemaleMax: 7.8},
		"糖化血红蛋白": {Min: 4, Max: 6, Unit: "%", MaleMin: 4, MaleMax: 6, FemaleMin: 4, FemaleMax: 6},
	},
	"血脂": {
		"总胆固醇":   {Min: 2.8, Max: 5.17, Unit: "mmol/L", MaleMin: 2.8, MaleMax: 5.17, FemaleMin: 2.8, FemaleMax: 5.17},
		"甘油三酯":   {Min: 0, Max: 1.7, Unit: "mmol/L", MaleMin: 0, MaleMax: 1.7, FemaleMin: 0, FemaleMax: 1.7},
		"高密度脂蛋白": {Min: 1.16, Max: 1.42, Unit: "mmol/L", MaleMin: 1.16, MaleMax: 1.42, FemaleMin: 1.16, FemaleMax: 1.42},
		"低密度脂蛋白": {Min: 0, Max: 3.37, Unit: "mmol/L", MaleMin: 0, MaleMax: 3.37, FemaleMin: 0, FemaleMax: 3.37},
	},
	"电解质": {
		"钾":   {Min: 3.5, Max: 5.3, Unit: "mmol/L", MaleMin: 3.5, MaleMax: 5.3, FemaleMin: 3.5, FemaleMax: 5.3},
		"钠":   {Min: 137, Max: 147, Unit: "mmol/L", MaleMin: 137, MaleMax: 147, FemaleMin: 137, FemaleMax: 147},
		"氯":   {Min: 99, Max: 110, Unit: "mmol/L", MaleMin: 99, MaleMax: 110, FemaleMin: 99, FemaleMax: 110},
		"钙":   {Min: 2.1, Max: 2.55, Unit: "mmol/L", MaleMin: 2.1, MaleMax: 2.55, FemaleMin: 2.1, FemaleMax: 2.55},
	},
}

func (t *LabReportInterpretTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	testName, _ := args["test_name"].(string)
	resultsStr, _ := args["results"].(string)
	gender, _ := args["gender"].(string)

	if testName == "" || resultsStr == "" {
		return &ToolResult{Success: false, Error: "请提供检验项目名称和结果"}, nil
	}

	// Parse results
	results := make(map[string]float64)
	for _, pair := range strings.Split(resultsStr, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			val, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if err == nil {
				results[strings.TrimSpace(parts[0])] = val
			}
		}
	}

	// Find reference ranges
	var ranges map[string]struct {
		Min    float64
		Max    float64
		Unit   string
		MaleMin   float64
		MaleMax   float64
		FemaleMin float64
		FemaleMax float64
	}

	for testNameKey, r := range labReferenceRanges {
		if strings.Contains(testName, testNameKey) || strings.Contains(testNameKey, testName) {
			ranges = r
			break
		}
	}

	if ranges == nil {
		return &ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"message": fmt.Sprintf("暂不支持「%s」的解读，目前支持: 血常规、肝功能、肾功能、血糖、血脂、电解质", testName),
			},
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【%s检验报告解读】\n\n", testName))

	abnormalItems := []string{}
	highItems := []string{}
	lowItems := []string{}

	for itemName, value := range results {
		ref, ok := ranges[itemName]
		if !ok {
			continue
		}

		min, max := ref.Min, ref.Max
		if gender == "男" {
			min, max = ref.MaleMin, ref.MaleMax
		} else if gender == "女" {
			min, max = ref.FemaleMin, ref.FemaleMax
		}

		status := "✓ 正常"
		if value > max {
			status = "↑ 偏高"
			highItems = append(highItems, itemName)
			abnormalItems = append(abnormalItems, itemName)
		} else if value < min {
			status = "↓ 偏低"
			lowItems = append(lowItems, itemName)
			abnormalItems = append(abnormalItems, itemName)
		}

		sb.WriteString(fmt.Sprintf("• %s: %.2f %s (参考范围: %.2f-%.2f) %s\n",
			itemName, value, ref.Unit, min, max, status))
	}

	sb.WriteString("\n")

	if len(abnormalItems) == 0 {
		sb.WriteString("【结论】✓ 所有项目均在正常范围内\n")
	} else {
		sb.WriteString("【异常提示】\n")
		if len(highItems) > 0 {
			sb.WriteString(fmt.Sprintf("• 偏高项目: %s\n", strings.Join(highItems, ", ")))
		}
		if len(lowItems) > 0 {
			sb.WriteString(fmt.Sprintf("• 偏低项目: %s\n", strings.Join(lowItems, ", ")))
		}
		sb.WriteString("\n")
		sb.WriteString("【建议】\n")
		sb.WriteString("• 单项轻度异常通常无特殊意义，需结合临床症状综合判断\n")
		sb.WriteString("• 多项异常建议到相应科室进一步检查\n")
		sb.WriteString("• 请将报告交由医生进行专业解读\n")
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"results":   sb.String(),
			"abnormal":  abnormalItems,
			"high":      highItems,
			"low":       lowItems,
		},
		Citations: []CitationRef{
			{ID: "lab_ref", Title: "Clinical Laboratory Reference Ranges", Level: "reference"},
		},
	}, nil
}
