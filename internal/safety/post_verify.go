package safety

import (
	"fmt"
	"regexp"
	"strings"
)

// PostVerifyResult holds the outcome of response verification.
type PostVerifyResult struct {
	Passed            bool
	HasCitations      bool
	CitationCount     int
	UnverifiedClaims  []string
	HasDiagnosisClaim bool
	HasDosageAdvice   bool
	Warnings          []string
	CorrectedResponse string
}

// PostVerifier checks agent responses for hallucination risks and
// compliance with evidence-based medicine requirements.
type PostVerifier struct {
	// ReferenceIndex maps citation IDs to their knowledge base entries.
	// citation "ID" -> entry title for verification.
	ReferenceIndex map[string]string
}

// NewPostVerifier creates a new post-generation verifier.
func NewPostVerifier(refIndex map[string]string) *PostVerifier {
	return &PostVerifier{
		ReferenceIndex: refIndex,
	}
}

// citationPattern matches [N] style citations.
var citationPattern = regexp.MustCompile(`\[(\d+)\]`)

// forbiddenDiagnosticPattern matches forbidden diagnostic assertions.
var forbiddenDiagnosticPattern = regexp.MustCompile(
	`你得了|您得了|你患有|您患有|你肯定是|您肯定是|确诊为|我可以断定`)

// forbiddenDosagePattern matches dosage recommendations that should be flagged.
var forbiddenDosagePattern = regexp.MustCompile(
	`你应该服用|你应该吃.*片|你每天吃.*粒|自行购买.*服用`)

// Verify checks a response for hallucination risks and compliance.
func (v *PostVerifier) Verify(response string) *PostVerifyResult {
	result := &PostVerifyResult{
		Passed:   true,
		Warnings: make([]string, 0),
	}

	// 1. Check for citations
	citationMatches := citationPattern.FindAllStringSubmatch(response, -1)
	result.CitationCount = len(citationMatches)
	if result.CitationCount > 0 {
		result.HasCitations = true
	} else if v.isFactualResponse(response) {
		result.Warnings = append(result.Warnings,
			"响应中包含事实性陈述但未标注引用编号 [N]。请为每条事实性陈述标注引用来源。")
		result.Passed = false
	}

	// 2. Verify citations exist in knowledge base
	seenIDs := make(map[string]bool)
	for _, match := range citationMatches {
		citationID := match[1]
		if seenIDs[citationID] {
			continue
		}
		seenIDs[citationID] = true

		if title, ok := v.ReferenceIndex[citationID]; !ok {
			result.UnverifiedClaims = append(result.UnverifiedClaims,
				fmt.Sprintf("引用 [%s] 未在知识库中找到对应条目", citationID))
			result.Passed = false
		} else {
			_ = title // Citation verified
		}
	}

	// 3. Check for forbidden diagnostic claims
	if forbiddenDiagnosticPattern.MatchString(response) {
		result.HasDiagnosisClaim = true
		result.Warnings = append(result.Warnings,
			"响应中包含确定性诊断断言。AI不能做出确定性诊断——请使用'可能'、'需考虑'、'建议进一步检查排除'等措辞。")
		result.Passed = false
	}

	// 4. Check for unauthorized dosage recommendations
	if forbiddenDosagePattern.MatchString(response) {
		result.HasDosageAdvice = true
		result.Warnings = append(result.Warnings,
			"响应中包含具体药物剂量建议。药物剂量必须由执业医师确定——请改为'请遵医嘱服用'。")
		result.Passed = false
	}

	// 5. Build corrected response if needed
	if !result.Passed {
		result.CorrectedResponse = v.buildCorrection(response, result)
	}

	return result
}

// isFactualResponse checks if the response contains substantive medical claims
// rather than just conversational or procedural text.
func (v *PostVerifier) isFactualResponse(response string) bool {
	// Heuristic: check if response has clinical analysis content
	indicators := []string{
		"鉴别诊断", "诊断", "治疗", "clinical analysis",
		"建议检查", "流行病学", "prevalence", "指南推荐",
		"循证", "evidence-based", "GRADE",
	}

	for _, indicator := range indicators {
		if strings.Contains(strings.ToLower(response), strings.ToLower(indicator)) {
			return true
		}
	}
	return false
}

// buildCorrection appends verification warnings to the response.
func (v *PostVerifier) buildCorrection(original string, result *PostVerifyResult) string {
	var sb strings.Builder
	sb.WriteString(original)
	sb.WriteString("\n\n---\n")
	sb.WriteString("### 📋 内容质量核查\n\n")
	sb.WriteString("本回答经自动核查，发现以下需注意的问题：\n\n")

	for i, warning := range result.Warnings {
		sb.WriteString(fmt.Sprintf("%d. ⚠️ %s\n", i+1, warning))
	}

	if len(result.UnverifiedClaims) > 0 {
		sb.WriteString("\n**未验证的引用：**\n")
		for _, claim := range result.UnverifiedClaims {
			sb.WriteString(fmt.Sprintf("- %s\n", claim))
		}
	}

	sb.WriteString("\n> 本核查由系统自动进行，如有疑问请反馈。医学信息请以最新临床指南和执业医师意见为准。\n")
	return sb.String()
}
