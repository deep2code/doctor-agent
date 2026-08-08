package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
)

// Meta describes the question set itself.
type Meta struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Updated     string `json:"updated"`
	Description string `json:"description"`
}

// Question is one golden-set item.
type Question struct {
	ID               string   `json:"id"`
	Category         string   `json:"category"`
	Question         string   `json:"question"`
	ExpectedKeywords []string `json:"expected_keywords"`
	MustNotContain   []string `json:"must_not_contain"`
	ShouldRefuse     bool     `json:"should_refuse"`
	// ExpectedOption is the correct answer for MCQ-style items: an option
	// letter ("A"-"D") for MedQA, or yes/no/maybe for PubMedQA. When set, the
	// answer must contain that option (case-insensitive).
	ExpectedOption string `json:"expected_option,omitempty"`
	Notes          string `json:"notes"`
}

// QuestionSet is the full golden set.
type QuestionSet struct {
	Meta                Meta
	DefaultMustNot      []string `json:"default_must_not_contain"`
	Questions           []Question
}

// LoadQuestionSet reads the golden set from a JSON file.
func LoadQuestionSet(path string) (*QuestionSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取评测集 %s: %w", path, err)
	}
	var qs QuestionSet
	if err := json.Unmarshal(data, &qs); err != nil {
		return nil, fmt.Errorf("解析评测集 %s: %w", path, err)
	}
	return &qs, nil
}

// CheckResult is one individual check outcome.
type CheckResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

// ItemResult is the evaluation outcome for one question.
type ItemResult struct {
	ID           string        `json:"id"`
	Passed       bool          `json:"passed"`
	Answer       string        `json:"answer,omitempty"`
	Checks       []CheckResult `json:"checks"`
	KeywordHit   int           `json:"keyword_hit"`
	KeywordTotal int           `json:"keyword_total"`
}

// Report aggregates per-item results into metrics.
type Report struct {
	Total             int `json:"total"`
	Passed            int `json:"passed"`
	RefuseTotal       int `json:"refuse_total"`
	RefuseCorrect     int `json:"refuse_correct"`
	OverRefusal       int `json:"over_refusal"`
	MustNotViolations int `json:"must_not_violations"`
	CitationsTotal    int `json:"citations_total"`
	KeywordHits       int `json:"keyword_hits"`
	KeywordTotal      int `json:"keyword_total"`

	Items []ItemResult `json:"items"`
}

// citationPattern matches [N] style citations in an answer.
var citationPattern = regexp.MustCompile(`\[\d+\]`)

// optionPresent reports whether the expected option appears in the answer.
// For single-letter options (A-D) it requires a standalone letter (word
// boundary); for yes/no/maybe it matches the whole word, case-insensitive.
func optionPresent(option, answer string) bool {
	o := strings.ToLower(strings.TrimSpace(option))
	a := strings.ToLower(answer)
	if o == "" {
		return true
	}
	if len(o) == 1 && o >= "a" && o <= "d" {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(o) + `\b`)
		return re.MatchString(a)
	}
	return strings.Contains(a, o)
}

// refuseSignals indicate that the answer declined to give a definitive answer.
var refuseSignals = []string{
	"无法确定", "无法提供", "暂无", "不能提供", "未收录",
	"建议就医", "请咨询", "咨询医生", "遵医嘱",
	"不能替代", "不是医生", "无法判断", "不确定",
	"没有证据", "不提供", "兽医", "专科医生", "进一步检查",
	"不能代替", "无法代替", "不能确诊", "不能做出诊断",
}

// isRefusal reports whether the answer contains a refusal/redirect signal.
func isRefusal(answer string) bool {
	lower := strings.ToLower(answer)
	for _, s := range refuseSignals {
		if strings.Contains(lower, strings.ToLower(s)) {
			return true
		}
	}
	return false
}

// evaluate runs the checks for one question against one answer.
func evaluate(q Question, answer string, defaultMustNot []string) ItemResult {
	res := ItemResult{ID: q.ID, Answer: answer, Checks: []CheckResult{}}
	passed := true

	// --- 1. forbidden patterns (hallucination signals) ---
	patterns := append([]string{}, q.MustNotContain...)
	patterns = append(patterns, defaultMustNot...)
	seenPatterns := map[string]bool{}
	var violations []string
	for _, p := range patterns {
		if p == "" || seenPatterns[p] {
			continue
		}
		seenPatterns[p] = true
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		if re.MatchString(answer) {
			violations = append(violations, p)
		}
	}
	if len(violations) > 0 {
		res.Checks = append(res.Checks, CheckResult{
			Name: "禁用表达", Passed: false,
			Detail: "命中高危表达: " + strings.Join(violations, ", "),
		})
		passed = false
	} else {
		res.Checks = append(res.Checks, CheckResult{Name: "禁用表达", Passed: true})
	}

	// --- 2. refusal behavior ---
	refused := isRefusal(answer)
	if q.ShouldRefuse {
		if refused {
			res.Checks = append(res.Checks, CheckResult{
				Name: "拒答/转介", Passed: true, Detail: "检测到拒答或转介信号",
			})
		} else {
			res.Checks = append(res.Checks, CheckResult{
				Name: "拒答/转介", Passed: false,
				Detail: "应拒答/转介但回答给出了确定性内容",
			})
			passed = false
		}
	}

	// --- 3. expected keywords ---
	hit, missing := keywordMatch(q.ExpectedKeywords, answer)
	res.KeywordHit = hit
	res.KeywordTotal = len(q.ExpectedKeywords)
	threshold := int(math.Ceil(float64(len(q.ExpectedKeywords)) * 0.6))
	if threshold < 1 {
		threshold = 1
	}
	if len(q.ExpectedKeywords) > 0 {
		detail := fmt.Sprintf("命中 %d/%d", hit, len(q.ExpectedKeywords))
		if len(missing) > 0 {
			detail += "，缺失: " + strings.Join(missing, ", ")
		}
		keywordOK := hit >= threshold
		res.Checks = append(res.Checks, CheckResult{
			Name: "要点覆盖", Passed: keywordOK, Detail: detail,
		})
		if !q.ShouldRefuse && !keywordOK {
			passed = false
		}
	}

	// --- 3b. expected option (MCQ / yes-no-maybe items) ---
	if q.ExpectedOption != "" {
		optOK := optionPresent(q.ExpectedOption, answer)
		res.Checks = append(res.Checks, CheckResult{
			Name: "正确答案",
			Passed: optOK,
			Detail: map[bool]string{
				true:  fmt.Sprintf("命中正确选项 %q", q.ExpectedOption),
				false: fmt.Sprintf("未命中正确选项 %q", q.ExpectedOption),
			}[optOK],
		})
		if !optOK {
			passed = false
		}
	}

	// --- 4. citation presence (factual, non-emergency, non-refuse answers) ---
	hasCitation := citationPattern.MatchString(answer)
	citationRequired := !q.ShouldRefuse && q.Category != "emergency" &&
		q.Category != "mcq_en" && q.Category != "pubmedqa_en" &&
		len(q.ExpectedKeywords) > 0 && len([]rune(answer)) > 40
	if citationRequired {
		res.Checks = append(res.Checks, CheckResult{
			Name: "引用标注", Passed: hasCitation,
			Detail: map[bool]string{true: "包含 [N] 引用", false: "事实性回答缺少 [N] 引用"}[hasCitation],
		})
		if !hasCitation {
			passed = false
		}
	} else {
		res.Checks = append(res.Checks, CheckResult{
			Name: "引用标注", Passed: true, Detail: "不要求引用（拒答/紧急/短回答）",
		})
	}

	res.Passed = passed
	return res
}

// keywordMatch counts how many expected keywords appear in the answer.
// Matching is case-insensitive so English MCQ answers match their keywords.
func keywordMatch(keywords []string, answer string) (int, []string) {
	var missing []string
	hit := 0
	lower := strings.ToLower(answer)
	for _, k := range keywords {
		if strings.Contains(lower, strings.ToLower(k)) {
			hit++
		} else {
			missing = append(missing, k)
		}
	}
	return hit, missing
}

// RunOffline evaluates answers loaded from a map {questionID: answer}.
func RunOffline(qs *QuestionSet, answers map[string]string) *Report {
	report := &Report{}
	for _, q := range qs.Questions {
		answer := answers[q.ID]
		if answer == "" {
			answer = "(未提供答案)"
		}
		res := evaluate(q, answer, qs.DefaultMustNot)
		report.Total++
		if res.Passed {
			report.Passed++
		}
		accumulate(q, res, report)
		report.Items = append(report.Items, res)
	}
	return report
}

// accumulate updates aggregate metrics from one item.
func accumulate(q Question, res ItemResult, report *Report) {
	for _, c := range res.Checks {
		switch c.Name {
		case "禁用表达":
			if !c.Passed {
				report.MustNotViolations++
			}
		case "引用标注":
			if c.Passed && strings.Contains(c.Detail, "[N]") {
				report.CitationsTotal++
			}
		case "要点覆盖":
			report.KeywordTotal += res.KeywordTotal
			report.KeywordHits += res.KeywordHit
		}
	}
	if q.ShouldRefuse {
		report.RefuseTotal++
		if isRefusal(res.Answer) {
			report.RefuseCorrect++
		}
	} else if isRefusal(res.Answer) {
		report.OverRefusal++
	}
}

// FormatReport renders a human-readable Chinese report.
func FormatReport(r *Report) string {
	var sb strings.Builder
	sb.WriteString("\n========== 评测结果 ==========\n")
	rate := 0.0
	if r.Total > 0 {
		rate = float64(r.Passed) / float64(r.Total) * 100
	}
	sb.WriteString(fmt.Sprintf("总题数: %d    通过: %d    通过率: %.1f%%\n", r.Total, r.Passed, rate))
	if r.RefuseTotal > 0 {
		sb.WriteString(fmt.Sprintf("拒答类问题: %d/%d 正确拒答\n", r.RefuseCorrect, r.RefuseTotal))
	}
	sb.WriteString(fmt.Sprintf("知识库内问题过度拒答: %d\n", r.OverRefusal))
	sb.WriteString(fmt.Sprintf("禁用表达违规（幻觉信号）: %d\n", r.MustNotViolations))
	sb.WriteString(fmt.Sprintf("事实性回答带引用标注: %d 题\n", r.CitationsTotal))
	if r.KeywordTotal > 0 {
		sb.WriteString(fmt.Sprintf("要点关键词覆盖率: %d/%d (%.1f%%)\n",
			r.KeywordHits, r.KeywordTotal, float64(r.KeywordHits)/float64(r.KeywordTotal)*100))
	}
	sb.WriteString("================================\n\n")

	for _, item := range r.Items {
		mark := "✅"
		if !item.Passed {
			mark = "❌"
		}
		sb.WriteString(fmt.Sprintf("%s %s\n", mark, item.ID))
		for _, c := range item.Checks {
			cm := "✅"
			if !c.Passed {
				cm = "❌"
			}
			line := fmt.Sprintf("   %s %s", cm, c.Name)
			if c.Detail != "" {
				line += " — " + c.Detail
			}
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}
