package prompt

import (
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/knowledge"
)

// Composer assembles the layered system prompt with dynamic content.
type Composer struct {
	formatter *knowledge.CitationFormatter
}

// NewComposer creates a new prompt composer.
func NewComposer() *Composer {
	return &Composer{
		formatter: knowledge.NewCitationFormatter(),
	}
}

// ComposeSystemPrompt assembles the full system prompt by layering foundation,
// clinical reasoning, southern genetics, southern environment, and safety rules,
// then injecting retrieved knowledge and tool definitions.
func (c *Composer) ComposeSystemPrompt(retrieved []knowledge.RetrievalResult, patientCtx string) string {
	var sb strings.Builder

	// Layer 0: Foundation (Medical Ethics & Role)
	sb.WriteString(LayerFoundation)
	sb.WriteString("\n\n")

	// Layer 1: Clinical Reasoning
	sb.WriteString(LayerClinicalReasoning)
	sb.WriteString("\n\n")

	// Layer 2: Southern Genetics
	sb.WriteString(LayerSouthernGenetics)
	sb.WriteString("\n\n")

	// Layer 3: Southern Environment & Diet
	sb.WriteString(LayerSouthernEnvironment)
	sb.WriteString("\n\n")

	// Layer 3.5: Everyday health problems (plain-language causes + similar cases)
	sb.WriteString(LayerEverydayHealth)
	sb.WriteString("\n\n")

	// Patient context injection (if available)
	if patientCtx != "" {
		sb.WriteString("## PATIENT CONTEXT\n\n")
		sb.WriteString(patientCtx)
		sb.WriteString("\n\n")
	}

	// Retrieved knowledge with citations
	if len(retrieved) > 0 {
		sb.WriteString(c.formatter.BuildCitationMap(retrieved))
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("## 可引用的循证医学文献\n\n")
		sb.WriteString("当前查询未检索到特异性知识条目。如果你认为用户的问题涉及事实性医学信息，请明确说明：")
		sb.WriteString("\"关于该问题，当前本地知识库中暂无直接相关的循证文献。以下回答基于一般医学知识框架，")
		sb.WriteString("不能替代专业医生的诊断。如果需要更具体的文献支持，建议查询PubMed或Cochrane Library。\"\n\n")
	}

	// Layer 4: Safety Rules
	sb.WriteString(LayerSafetyRules)
	sb.WriteString("\n")

	return sb.String()
}

// ComposeToolPrompt generates the prompt segment describing available tools.
func (c *Composer) ComposeToolPrompt(toolDescriptions []string) string {
	if len(toolDescriptions) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## AVAILABLE TOOLS\n\n")
	sb.WriteString("你可以使用以下工具获取精确的结构化医学数据。")
	sb.WriteString("工具返回的结果包含可靠的循证引用。\n\n")

	for i, desc := range toolDescriptions {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, desc))
	}
	sb.WriteString("\n使用工具的时机：\n")
	sb.WriteString("- 需要查询G6PD药物安全性时 → drug_safety_check\n")
	sb.WriteString("- 需要计算地贫遗传风险时 → genetic_risk_calculator\n")
	sb.WriteString("- 需要分析食物健康风险时 → food_risk_analyzer\n")
	sb.WriteString("- 需要紧急分诊评估时 → symptom_triage\n")
	sb.WriteString("- 需要检索特定医学文献时 → reference_lookup\n")
	sb.WriteString("- 需要解读实验室检查结果时 → lab_interpreter\n")
	sb.WriteString("\n工具返回的数据可以直接在你的回答中引用。\n")

	return sb.String()
}

// BuildPatientContext creates a patient context string for prompt injection.
func BuildPatientContext(pc *PatientContextSummary) string {
	if pc == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("以下是与当前用户相关的背景信息（用户自行提供）：\n")

	if pc.Region != "" {
		sb.WriteString(fmt.Sprintf("- 居住/来源地区: %s\n", pc.Region))
	}
	if pc.G6PDStatus != "" {
		sb.WriteString(fmt.Sprintf("- G6PD状态: %s\n", pc.G6PDStatus))
	}
	if pc.ThalassemiaTrait != "" {
		sb.WriteString(fmt.Sprintf("- 地中海贫血携带情况: %s\n", pc.ThalassemiaTrait))
	}
	if pc.KnownConditions != nil && len(pc.KnownConditions) > 0 {
		sb.WriteString(fmt.Sprintf("- 已知疾病: %s\n", strings.Join(pc.KnownConditions, "、")))
	}

	sb.WriteString("\n请结合这些背景信息进行风险评估和临床分析，但仍需遵循循证医学原则。\n")
	return sb.String()
}

// PatientContextSummary mirrors session.PatientContext for the prompt layer.
type PatientContextSummary struct {
	Region           string
	G6PDStatus       string
	ThalassemiaTrait string
	KnownConditions  []string
}
