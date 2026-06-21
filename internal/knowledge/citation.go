package knowledge

import (
	"fmt"
	"strings"
)

// CitationFormatter generates formatted reference strings from Citation data.
type CitationFormatter struct{}

// NewCitationFormatter creates a new formatter.
func NewCitationFormatter() *CitationFormatter {
	return &CitationFormatter{}
}

// FormatReference generates an AMA-style formatted reference string.
func (cf *CitationFormatter) FormatReference(c *Citation, index int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("[%d] ", index))

	// Type prefix
	typeLabel := "[" + cf.typeLabel(c.Type) + "] "

	sb.WriteString(typeLabel)

	// Journal article
	if c.Journal != "" {
		sb.WriteString(c.Title)
		sb.WriteString(". ")
		sb.WriteString(c.Journal)
		if c.Year > 0 {
			sb.WriteString(fmt.Sprintf(". %d", c.Year))
		}
		sb.WriteString(".")
	} else {
		// Non-journal reference (guideline, report, etc.)
		sb.WriteString(c.Title)
		if c.Year > 0 {
			sb.WriteString(fmt.Sprintf(" (%d)", c.Year))
		}
		sb.WriteString(".")
	}

	// DOI
	if c.DOI != "" {
		sb.WriteString(fmt.Sprintf(" DOI: %s", c.DOI))
	}

	// PMID
	if c.PMID != "" {
		sb.WriteString(fmt.Sprintf(" PMID: %s", c.PMID))
	}

	return sb.String()
}

// FormatAllReferences formats a slice of citations into numbered references.
func (cf *CitationFormatter) FormatAllReferences(citations []Citation) string {
	if len(citations) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### 参考文献\n\n")
	for i, c := range citations {
		sb.WriteString(cf.FormatReference(&c, i+1))
		sb.WriteString("\n")
	}
	return sb.String()
}

// FormatCitationSummary creates a compact inline citation summary.
// Example: "[国家指南(2025)] DOI: 10.xxx" for a single citation.
func (cf *CitationFormatter) FormatCitationSummary(c *Citation) string {
	var sb strings.Builder
	sb.WriteString(cf.typeLabel(c.Type))

	if c.Year > 0 {
		sb.WriteString(fmt.Sprintf(" (%d)", c.Year))
	}

	if c.DOI != "" {
		sb.WriteString(fmt.Sprintf(" — DOI: %s", c.DOI))
	} else if c.PMID != "" {
		sb.WriteString(fmt.Sprintf(" — PMID: %s", c.PMID))
	}

	return sb.String()
}

// BuildCitationMap creates a knowledge-entry-ID -> formatted citation mapping
// for inclusion in the system prompt.
func (cf *CitationFormatter) BuildCitationMap(entries []RetrievalResult) string {
	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 可引用的循证医学文献\n\n")
	sb.WriteString("以下是你可以在回答中引用的知识条目和文献来源。每个条目标注引用编号 [N]。\n\n")

	for i, result := range entries {
		e := result.Entry
		refIdx := i + 1
		sb.WriteString(fmt.Sprintf("**[%d] %s (%s)**\n", refIdx, e.ConditionZH, e.ConditionEN))
		sb.WriteString(fmt.Sprintf("  - ICD-10: %s | 分类: %s | 地区: %s\n",
			e.ICD10, e.Category, strings.Join(e.Regions, ", ")))

		// Prevalence summary
		if len(e.Prevalence) > 0 {
			sb.WriteString("  - 流行病学: ")
			first := true
			for region, prev := range e.Prevalence {
				if !first {
					sb.WriteString("; ")
				}
				sb.WriteString(fmt.Sprintf("%s %.1f%%", region, prev.Rate*100))
				first = false
			}
			sb.WriteString("\n")
		}

		// Citations with reference numbers
		if len(e.Citations) > 0 {
			sb.WriteString("  - 文献来源:\n")
			for j, c := range e.Citations {
				sb.WriteString(fmt.Sprintf("    引用 %d.%d: %s. %s (%d)",
					refIdx, j+1, c.Title, c.Journal, c.Year))
				if c.DOI != "" {
					sb.WriteString(fmt.Sprintf(" DOI: %s", c.DOI))
				}
				if c.PMID != "" {
					sb.WriteString(fmt.Sprintf(" PMID: %s", c.PMID))
				}
				sb.WriteString(fmt.Sprintf(" [证据等级: %s]", c.Level))
				sb.WriteString("\n")
			}
		}

		sb.WriteString("\n")
	}

	sb.WriteString("**重要：每条事实性陈述后面必须标注引用编号，如 [1]、[2]。不要引用上述列表中不存在的文献。只能引用检索到的知识条目中的文献来源。**\n")
	return sb.String()
}

func (cf *CitationFormatter) typeLabel(t string) string {
	switch t {
	case "guideline":
		return "临床指南"
	case "epidemiology":
		return "流行病学研究"
	case "meta_analysis":
		return "Meta分析"
	case "rct":
		return "随机对照试验"
	case "cohort":
		return "队列研究"
	case "case_control":
		return "病例对照研究"
	case "case_report":
		return "病例报告"
	case "review":
		return "综述"
	case "expert_opinion":
		return "专家意见"
	default:
		return t
	}
}
