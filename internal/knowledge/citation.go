package knowledge

import (
	"fmt"
	"strconv"
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

	fmt.Fprintf(&sb, "[%d] ", index)

	// Type prefix
	typeLabel := "[" + cf.typeLabel(c.Type) + "] "

	sb.WriteString(typeLabel)

	// Journal article
	if c.Journal != "" {
		sb.WriteString(c.Title)
		sb.WriteString(". ")
		sb.WriteString(c.Journal)
		if c.Year > 0 {
			fmt.Fprintf(&sb, ". %d", c.Year)
		}
		sb.WriteString(".")
	} else {
		// Non-journal reference (guideline, report, etc.)
		sb.WriteString(c.Title)
		if c.Year > 0 {
			fmt.Fprintf(&sb, " (%d)", c.Year)
		}
		sb.WriteString(".")
	}

	// DOI
	if c.DOI != "" {
		fmt.Fprintf(&sb, " DOI: %s", c.DOI)
	}

	// PMID
	if c.PMID != "" {
		fmt.Fprintf(&sb, " PMID: %s", c.PMID)
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
		fmt.Fprintf(&sb, " (%d)", c.Year)
	}

	if c.DOI != "" {
		fmt.Fprintf(&sb, " — DOI: %s", c.DOI)
	} else if c.PMID != "" {
		fmt.Fprintf(&sb, " — PMID: %s", c.PMID)
	}

	return sb.String()
}

// BuildCitationMap creates a knowledge-entry-ID -> formatted citation mapping
// for inclusion in the system prompt. Citations are numbered flatly [1]..[N]
// so the model can reference them directly.
func (cf *CitationFormatter) BuildCitationMap(entries []RetrievalResult) string {
	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 可引用的循证医学文献\n\n")
	sb.WriteString("以下是你可以在回答中引用的知识条目和文献来源。每个文献条目有唯一编号 [N]。\n\n")

	flat := flattenCitations(entries)
	for _, fc := range flat {
		c := fc.citation
		e := fc.result.Entry
		fmt.Fprintf(&sb, "**[%d] %s** （来自条目: %s）\n", fc.number, c.Title, e.ConditionZH)
		if c.Journal != "" {
			fmt.Fprintf(&sb, "  - %s", c.Journal)
			if c.Year > 0 {
				fmt.Fprintf(&sb, " (%d)", c.Year)
			}
			sb.WriteString("\n")
		}
		if c.DOI != "" {
			fmt.Fprintf(&sb, "  - DOI: %s\n", c.DOI)
		}
		if c.PMID != "" {
			fmt.Fprintf(&sb, "  - PMID: %s\n", c.PMID)
		}
		fmt.Fprintf(&sb, "  - 证据等级: %s\n", c.Level)
		sb.WriteString("\n")
	}

	sb.WriteString("**重要：每条事实性陈述后面必须标注引用编号，如 [1]、[2]。不要引用上述列表中不存在的文献编号。只能引用检索到的知识条目中的文献来源。**\n")
	return sb.String()
}

// flatCitation is a citation paired with its flat global number and source entry.
type flatCitation struct {
	number   int
	result   RetrievalResult
	citation Citation
}

// flattenCitations assigns a flat 1..N number to every citation across the
// retrieved entries, preserving order. Used by both the prompt builder and
// the post-verification source map so numbering always matches.
func flattenCitations(entries []RetrievalResult) []flatCitation {
	var flat []flatCitation
	n := 1
	for _, r := range entries {
		for _, c := range r.Entry.Citations {
			flat = append(flat, flatCitation{number: n, result: r, citation: c})
			n++
		}
	}
	return flat
}

// CitedSource provides a citation number -> full source context mapping,
// used by post-generation verification to check claim-support consistency.
type CitedSource struct {
	Number      string   `json:"number"`
	EntryID     string   `json:"entry_id"`
	ConditionZH string   `json:"condition_zh"`
	Citation    Citation `json:"citation"`
	EntryText   string   `json:"entry_text"`
}

// BuildCitedSources maps citation numbers "1".."N" (same numbering as
// BuildCitationMap) to their source context. Empty when nothing was retrieved.
func BuildCitedSources(entries []RetrievalResult) map[string]CitedSource {
	sources := make(map[string]CitedSource)
	for _, fc := range flattenCitations(entries) {
		num := strconv.Itoa(fc.number)
		e := fc.result.Entry
		sources[num] = CitedSource{
			Number:      num,
			EntryID:     e.ID,
			ConditionZH: e.ConditionZH,
			Citation:    fc.citation,
			EntryText:   entrySummary(&e),
		}
	}
	return sources
}

// entrySummary renders a compact evidence summary of a knowledge entry for
// the semantic verifier to judge claim-support against.
func entrySummary(e *KnowledgeEntry) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (%s) — 分类: %s", e.ConditionZH, e.ConditionEN, e.Category)
	if e.ICD10 != "" {
		fmt.Fprintf(&sb, ", ICD-10: %s", e.ICD10)
	}
	sb.WriteString("\n")

	if len(e.Prevalence) > 0 {
		sb.WriteString("流行病学: ")
		first := true
		for region, prev := range e.Prevalence {
			if !first {
				sb.WriteString("; ")
			}
			fmt.Fprintf(&sb, "%s %.1f%%", region, prev.Rate*100)
			first = false
		}
		sb.WriteString("\n")
	}

	if e.Diagnosis != nil {
		d := e.Diagnosis
		if len(d.LabTests) > 0 {
			sb.WriteString("诊断检查: " + strings.Join(d.LabTests, ", ") + "\n")
		}
		if d.GoldStandard != "" {
			sb.WriteString("金标准: " + d.GoldStandard + "\n")
		}
	}

	if len(e.Treatment) > 0 {
		sb.WriteString("治疗:")
		for i, t := range e.Treatment {
			if i >= 5 {
				break
			}
			sb.WriteString(" " + t.Method)
			if t.Indication != "" {
				sb.WriteString("（" + t.Indication + "）")
			}
			sb.WriteString(";")
		}
		sb.WriteString("\n")
	}

	if len(e.RiskFactors) > 0 {
		sb.WriteString("风险因素: " + strings.Join(e.RiskFactors, ", ") + "\n")
	}
	if len(e.Prevention) > 0 {
		sb.WriteString("预防: " + strings.Join(e.Prevention, ", ") + "\n")
	}

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

// AddToolSource registers a tool-returned reference (e.g. a literature_search
// article) as a verifiable citation source. The PMID is used as the citation
// key when present: PMIDs are pure digits, so a "[PMID]" reference in the
// response matches the verifier's [N] pattern and resolves here. Articles
// without a PMID get no number key (the model can't reference them by [N]);
// their title context is still registered under "doi:" so DOI-style
// references verify too.
func AddToolSource(sources map[string]CitedSource, title, doi, pmid string, year int, level string, text string) {
	if pmid != "" {
		sources[pmid] = CitedSource{
			Number:      pmid,
			ConditionZH: title,
			Citation: Citation{
				Title:   title,
				DOI:     doi,
				PMID:    pmid,
				Year:    year,
				Level:   level,
				Journal: "",
			},
			EntryText: text,
		}
	}
	if doi != "" {
		sources["doi:"+doi] = CitedSource{
			Number:      doi,
			ConditionZH: title,
			Citation: Citation{
				Title: title,
				DOI:   doi,
				PMID:  pmid,
				Year:  year,
				Level: level,
			},
			EntryText: text,
		}
	}
}
