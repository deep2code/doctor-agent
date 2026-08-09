package knowledge

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// VerificationReport is the result of a full knowledge-base integrity check.
type VerificationReport struct {
	DataVersion *DataVersion

	MedicalEntries    int
	DrugEntries       int
	FoodRiskEntries   int
	EmergencyRules    int
	LabTestReferences int

	TotalCitations     int
	CitationsWithDOI   int
	CitationsWithPMID  int
	CitationsWithURL   int
	CitationsWithRefID int // has DOI or PMID

	EntryIDIssues  []string
	CitationIssues []string
	Warnings       []string
	Errors         []string
}

// HasIssues reports whether any error or warning was found.
func (r *VerificationReport) HasIssues() bool {
	return len(r.Errors) > 0 || len(r.Warnings) > 0 ||
		len(r.EntryIDIssues) > 0 || len(r.CitationIssues) > 0
}

// CitationTraceability returns the fraction of citations carrying a real
// resolvable identifier (DOI or PMID) — the core "可溯源" metric.
func (r *VerificationReport) CitationTraceability() float64 {
	if r.TotalCitations == 0 {
		return 0
	}
	return float64(r.CitationsWithRefID) / float64(r.TotalCitations)
}

// DataVersion describes the knowledge base release and its authoritative sources.
type DataVersion struct {
	Version     string   `json:"version"`
	Updated     string   `json:"updated"`
	Description string   `json:"description"`
	Sources     []string `json:"sources"`
}

var (
	doiPattern  = regexp.MustCompile(`^10\.\d{4,9}/[^\s]*[^\s.]$`)
	pmidPattern = regexp.MustCompile(`^\d{1,8}$`)
	urlPattern  = regexp.MustCompile(`^https?://\S+$`)
)

// VerifyData runs integrity checks across the entire knowledge base:
// identifier uniqueness, citation completeness/format, traceability
// (DOI/PMID/URL presence), and structural sanity.
func VerifyData(store *Store) *VerificationReport {
	report := &VerificationReport{
		DataVersion: store.GetDataVersion(),
	}

	report.MedicalEntries = len(store.MedicalEntries)
	report.DrugEntries = len(store.DrugEntries)
	report.FoodRiskEntries = len(store.FoodRiskEntries)
	report.EmergencyRules = len(store.EmergencyRules)
	report.LabTestReferences = len(store.LabTestReferences)

	seenMedicalIDs := make(map[string]bool)
	seenDrugIDs := make(map[string]bool)
	seenCitationKeys := make(map[string]bool)

	// --- Medical entries ---
	for _, e := range store.MedicalEntries {
		if e.ID == "" {
			report.EntryIDIssues = append(report.EntryIDIssues, "医学条目缺少 ID (condition_zh="+e.ConditionZH+")")
		} else if seenMedicalIDs[e.ID] {
			report.EntryIDIssues = append(report.EntryIDIssues, fmt.Sprintf("医学条目 ID 重复: %s", e.ID))
		}
		seenMedicalIDs[e.ID] = true

		if e.ConditionZH == "" {
			report.EntryIDIssues = append(report.EntryIDIssues, fmt.Sprintf("医学条目 %s 缺少 condition_zh", e.ID))
		}

		if len(e.Citations) == 0 {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("医学条目 '%s' (%s) 缺少引用文献", e.ConditionZH, e.ID))
		}

		for region, prev := range e.Prevalence {
			if region == "" {
				report.Warnings = append(report.Warnings, fmt.Sprintf("条目 %s 存在空地区名的流行率数据", e.ID))
			}
			if prev.Rate < 0 || prev.Rate > 1 {
				report.CitationIssues = append(report.CitationIssues,
					fmt.Sprintf("条目 %s 地区 %s 流行率 %.4f 超出 [0,1] 范围", e.ID, region, prev.Rate))
			}
		}

		report.checkCitations(&e.Citations, e.ID, seenCitationKeys, report)
	}

	// --- Drug entries ---
	seenDrugNames := make(map[string]string) // generic name -> first entry ID
	for _, d := range store.DrugEntries {
		if d.ID == "" {
			report.EntryIDIssues = append(report.EntryIDIssues, "药物条目缺少 ID")
		} else if seenDrugIDs[d.ID] {
			report.EntryIDIssues = append(report.EntryIDIssues, fmt.Sprintf("药物条目 ID 重复: %s", d.ID))
		}
		seenDrugIDs[d.ID] = true

		// EN and ZH generic names share one lookup map in loader.go; a
		// collision would silently overwrite — flag it.
		for _, name := range []string{d.GenericNameEN, d.GenericNameZH} {
			if name == "" {
				continue
			}
			if prev, ok := seenDrugNames[name]; ok {
				report.EntryIDIssues = append(report.EntryIDIssues,
					fmt.Sprintf("药物通用名冲突 %q 同时被 %s 和 %s 使用", name, prev, d.ID))
			} else {
				seenDrugNames[name] = d.ID
			}
		}

		if d.GenericNameZH == "" || d.GenericNameEN == "" {
			report.Warnings = append(report.Warnings, fmt.Sprintf("药物条目 %s 缺少中英文通用名", d.ID))
		}
		if d.G6PDSafety == "" || d.RiskLevel == "" {
			report.Warnings = append(report.Warnings, fmt.Sprintf("药物条目 %s 缺少 g6pd_safety/risk_level", d.ID))
		}
		if len(d.Citations) == 0 {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("药物条目 '%s' (%s) 缺少引用文献", d.GenericNameZH, d.ID))
		}
		report.checkCitations(&d.Citations, d.ID, seenCitationKeys, report)
	}

	// --- Food risk entries ---
	for _, f := range store.FoodRiskEntries {
		if f.ID == "" {
			report.EntryIDIssues = append(report.EntryIDIssues, "食物风险条目缺少 ID")
		}
		if len(f.Citations) == 0 {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("食物风险条目 '%s' (%s) 缺少引用文献", f.FoodNameZH, f.ID))
		}
		report.checkCitations(&f.Citations, f.ID, seenCitationKeys, report)
	}

	// --- Emergency rules ---
	for _, r := range store.EmergencyRules {
		if r.ID == "" {
			report.EntryIDIssues = append(report.EntryIDIssues, "紧急分诊规则缺少 ID")
		}
		if r.Level == "" || r.ActionZH == "" {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("紧急分诊规则 %s 缺少 level/action_zh", r.ID))
		}
		if len(r.Keywords) == 0 && len(r.KeywordsZH) == 0 {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("紧急分诊规则 %s 缺少触发关键词", r.ID))
		}
		report.checkCitations(&r.Citations, r.ID, seenCitationKeys, report)
	}

	// --- Lab test references ---
	for _, l := range store.LabTestReferences {
		if l.ID == "" {
			report.EntryIDIssues = append(report.EntryIDIssues, "实验室检查条目缺少 ID")
		}
		if l.TestNameZH == "" || l.TestNameEN == "" {
			report.Warnings = append(report.Warnings, fmt.Sprintf("实验室检查条目 %s 缺少名称", l.ID))
		}
		report.checkCitations(&l.Citations, l.ID, seenCitationKeys, report)
	}

	return report
}

// checkCitations validates one citation list, updating the shared key set.
func (r *VerificationReport) checkCitations(citations *[]Citation, entryID string, seenKeys map[string]bool, report *VerificationReport) {
	for j, c := range *citations {
		report.TotalCitations++

		if c.Title == "" {
			report.CitationIssues = append(report.CitationIssues,
				fmt.Sprintf("条目 %s 的引用缺少 Title", entryID))
		}
		if c.Year < 1950 || c.Year > 2100 {
			report.CitationIssues = append(report.CitationIssues,
				fmt.Sprintf("条目 %s 引用年份异常: %d", entryID, c.Year))
		}
		if c.Level == "" {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("条目 %s 引用 '%s' 缺少证据等级 level", entryID, c.Title))
		}

		if c.DOI != "" {
			if doiPattern.MatchString(c.DOI) {
				report.CitationsWithDOI++
				report.CitationsWithRefID++
			} else {
				report.CitationIssues = append(report.CitationIssues,
					fmt.Sprintf("条目 %s 引用 DOI 格式无效: %q", entryID, c.DOI))
			}
		}
		if c.PMID != "" {
			if pmidPattern.MatchString(c.PMID) {
				report.CitationsWithPMID++
				if c.DOI == "" {
					report.CitationsWithRefID++
				}
			} else {
				report.CitationIssues = append(report.CitationIssues,
					fmt.Sprintf("条目 %s 引用 PMID 格式无效: %q", entryID, c.PMID))
			}
		}
		if c.URL != "" {
			if urlPattern.MatchString(c.URL) {
				report.CitationsWithURL++
			} else {
				report.CitationIssues = append(report.CitationIssues,
					fmt.Sprintf("条目 %s 引用 URL 格式无效: %q", entryID, c.URL))
			}
		}

		// Journal articles must be resolvable (DOI or PMID); pure-title refs
		// (guidelines, reports) are allowed without them.
		if c.Journal != "" && c.DOI == "" && c.PMID == "" {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("条目 %s 期刊文献 '%s' 缺少 DOI 和 PMID，无法溯源", entryID, c.Title))
		}

		// Citation key uniqueness (same keying as loader.ReferenceIndex)
		if c.Year > 0 {
			key := fmt.Sprintf("%s-cite-%d-%d", entryID, c.Year, j)
			if seenKeys[key] {
				report.CitationIssues = append(report.CitationIssues,
					fmt.Sprintf("引用键冲突（同条目同年多篇引用会被覆盖）: %s", key))
			}
			seenKeys[key] = true
		}
	}
}

// formatReportLine is a small helper to keep main output tidy.
func formatReportLine(ok bool, msg string) string {
	if ok {
		return "  ✅ " + msg
	}
	return "  ❌ " + msg
}

// ReportText renders the verification report as human-readable Chinese text.
func ReportText(report *VerificationReport) string {
	var sb strings.Builder

	if report.DataVersion != nil {
		sb.WriteString(fmt.Sprintf("知识库版本: %s (更新于 %s)\n", report.DataVersion.Version, report.DataVersion.Updated))
		if report.DataVersion.Description != "" {
			sb.WriteString(fmt.Sprintf("说明: %s\n", report.DataVersion.Description))
		}
		if len(report.DataVersion.Sources) > 0 {
			sb.WriteString("数据来源:\n")
			for _, s := range report.DataVersion.Sources {
				sb.WriteString(fmt.Sprintf("  - %s\n", s))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("医学知识条目: %d\n", report.MedicalEntries))
	sb.WriteString(fmt.Sprintf("药物条目: %d\n", report.DrugEntries))
	sb.WriteString(fmt.Sprintf("食物风险条目: %d\n", report.FoodRiskEntries))
	sb.WriteString(fmt.Sprintf("紧急分诊规则: %d\n", report.EmergencyRules))
	sb.WriteString(fmt.Sprintf("实验室检查条目: %d\n", report.LabTestReferences))
	sb.WriteString(fmt.Sprintf("引用总数: %d (DOI: %d, PMID: %d, URL: %d)\n",
		report.TotalCitations, report.CitationsWithDOI, report.CitationsWithPMID, report.CitationsWithURL))
	sb.WriteString(fmt.Sprintf("引用可溯源率 (DOI/PMID): %.1f%%\n", report.CitationTraceability()*100))

	if len(report.EntryIDIssues) == 0 && len(report.CitationIssues) == 0 && len(report.Warnings) == 0 && len(report.Errors) == 0 {
		sb.WriteString("\n✅ 知识库校验全部通过\n")
		return sb.String()
	}

	sb.WriteString("\n")
	for _, err := range report.Errors {
		sb.WriteString(formatReportLine(false, err) + "\n")
	}
	for _, issue := range report.EntryIDIssues {
		sb.WriteString(formatReportLine(false, "ID/结构: "+issue) + "\n")
	}
	for _, issue := range report.CitationIssues {
		sb.WriteString(formatReportLine(false, "引用: "+issue) + "\n")
	}
	for _, w := range report.Warnings {
		sb.WriteString(formatReportLine(true, "警告: "+w) + "\n")
	}
	return sb.String()
}

// URLCheckIssue records a citation URL that could not be reached.
type URLCheckIssue struct {
	ID  string
	URL string
	Err string
}

// CheckURLLiveness probes every citation URL with HTTP HEAD requests (falling
// back to GET when the server rejects HEAD) and returns the unreachable ones.
// The check is concurrent but bounded by maxConcurrent, and every URL gets a
// per-request timeout. URLs that are not http(s) are skipped.
func CheckURLLiveness(store *Store, timeout time.Duration, maxConcurrent int) []URLCheckIssue {
	if maxConcurrent <= 0 {
		maxConcurrent = 8
	}
	client := &http.Client{Timeout: timeout}

	// Collect (id, url) references from every citation-bearing entry type.
	type urlRef struct{ id, url string }
	var refs []urlRef
	seen := make(map[string]bool)
	add := func(id string, citations []Citation) {
		for _, c := range citations {
			if c.URL == "" || seen[c.URL] {
				continue
			}
			seen[c.URL] = true
			refs = append(refs, urlRef{id: id, url: c.URL})
		}
	}
	for _, e := range store.MedicalEntries {
		add(e.ID, e.Citations)
	}
	for _, d := range store.DrugEntries {
		add(d.ID, d.Citations)
	}
	for _, f := range store.FoodRiskEntries {
		add(f.ID, f.Citations)
	}
	for _, l := range store.LabTestReferences {
		add(l.ID, l.Citations)
	}

	sem := make(chan struct{}, maxConcurrent)
	var mu sync.Mutex
	var issues []URLCheckIssue
	var wg sync.WaitGroup

	for _, ref := range refs {
		wg.Add(1)
		sem <- struct{}{}
		go func(ref urlRef) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := probeURL(client, ref.url); err != nil {
				mu.Lock()
				issues = append(issues, URLCheckIssue{ID: ref.id, URL: ref.url, Err: err.Error()})
				mu.Unlock()
			}
		}(ref)
	}
	wg.Wait()
	return issues
}

// probeURL performs HEAD with a GET fallback; any non-2xx/3xx status or
// transport error is reported.
func probeURL(client *http.Client, url string) error {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil
	}
	do := func(method string) (int, error) {
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("User-Agent", "doctor-agent-verify/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		// Drain a little to allow connection reuse, then close.
		_, _ = io.CopyN(io.Discard, resp.Body, 4096)
		return resp.StatusCode, nil
	}

	code, err := do(http.MethodHead)
	if err == nil && code < 400 {
		return nil
	}
	// HEAD unsupported (405/501) or transport error → retry with GET.
	if err != nil {
		if code, err2 := do(http.MethodGet); err2 == nil && code < 400 {
			return nil
		}
		return err
	}
	if code == http.StatusMethodNotAllowed || code == http.StatusNotImplemented {
		if code2, err2 := do(http.MethodGet); err2 == nil && code2 < 400 {
			return nil
		}
	}
	return fmt.Errorf("status %d", code)
}
