package server

import (
	"context"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/doctor-agent/internal/agent"
	"github.com/doctor-agent/internal/auth"
	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/database"
	"github.com/doctor-agent/internal/embedding"
	"github.com/doctor-agent/internal/knowledge"
	"github.com/doctor-agent/internal/llm"
	"github.com/doctor-agent/internal/session"
	"github.com/jung-kurt/gofpdf"
)

//go:embed web/index.html
var webUIIndex string

//go:embed web/admin.html
var adminUIIndex string

//go:embed web/landing.html
var landingTmpl string

//go:embed web/map.html
var mapTmpl string

//go:embed web/stats.html
var statsTmpl string

//go:embed web/share.html
var sharePageTmpl string

//go:embed web/export_pdf.html
var exportPDFTmpl string

// Shared stylesheets for the public marketing pages, split out of the old
// single landing <style> block. They are inlined into each page at startup
// (see buildPage) so pages stay single-file: no external assets, works
// offline, no file server needed.
//
//go:embed web/shared/base.css
var cssBase string

//go:embed web/shared/map.css
var cssMap string

//go:embed web/shared/stats.css
var cssStats string

//go:embed web/shared/bm.css
var cssBM string

// Lazy-loaded JavaScript libraries for the chat UI (extracted from the
// monolithic index.html to reduce first-paint payload). Served as static
// assets; the HTML references them via <script src="...">.
//
//go:embed web/mermaid.min.js
var jsMermaid string

//go:embed web/three.min.js
var jsThree string

//go:embed web/anatomy.js
var jsAnatomy string

//go:embed web/anatomy-anim.js
var jsAnatomyAnim string

//go:embed web/qrcode.min.js
var jsQRCode string

//go:embed web/favicon.ico
var faviconICO []byte

// Server wraps the HTTP API server for the doctor agent.
type Server struct {
	cfg   *config.Config
	agent *agent.Agent
	auth  *auth.Service
	db    *database.DB
	http  *http.Server
	limiter *rateLimiter

	// Build metadata (injected from main.go via SetBuildInfo).
	gitCommit string
	buildTime string

	// Pre-rendered public pages (shared CSS inlined + canonical/og:url
	// injected once per process in New; see buildPage).
	pageLanding string
	pageMap     string
	pageStats   string
	// 分享页模板（/s/{id}），payload 每请求注入。
	pageShareTmpl string
}

// SetBuildInfo injects the git commit hash and build timestamp from main.go
// (populated via -ldflags at build time). Call once after construction.
func (s *Server) SetBuildInfo(commit, built string) {
	s.gitCommit = commit
	s.buildTime = built
}

// New creates a new HTTP server.
func New(cfg *config.Config, ag *agent.Agent, authSvc *auth.Service) *Server {
	return NewWithDB(cfg, ag, authSvc, nil)
}

// NewWithDB creates a new HTTP server with access to the application
// database, enabling server-side session persistence and session APIs.
func NewWithDB(cfg *config.Config, ag *agent.Agent, authSvc *auth.Service, db *database.DB) *Server {
	s := &Server{
		cfg:     cfg,
		agent:   ag,
		auth:    authSvc,
		db:      db,
		limiter: newRateLimiter(cfg.RateLimit),

		// 分享页模板：payload 按请求注入（见 handleSharePage），不预渲染。
		pageShareTmpl: sharePageTmpl,
	}

	// Pre-render the public pages once: shared CSS inlined into each page's
	// style placeholder, plus canonical/og:url tags when PublicBaseURL is set.
	s.pageLanding = buildPage(landingTmpl, cssBM, cfg.PublicBaseURL, "/")
	s.pageMap = buildPage(mapTmpl, cssMap, cfg.PublicBaseURL, "/map")
	s.pageStats = buildPage(statsTmpl, cssStats, cfg.PublicBaseURL, "/stats")

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleLanding)
	mux.HandleFunc("/map", s.handleMapPage)
	mux.HandleFunc("/stats", s.handleStatsPage)
	mux.HandleFunc("/robots.txt", s.handleRobots)
	mux.HandleFunc("/sitemap.xml", s.handleSitemap)
	mux.HandleFunc("/llms.txt", s.handleLLMsTxt)
	mux.HandleFunc("/app", s.handleAppUI)
	// Lazy-loaded JS assets for the chat UI (extracted to cut first-paint).
	mux.HandleFunc("/mermaid.min.js", s.handleMermaidJS)
	mux.HandleFunc("/three.min.js", s.handleThreeJS)
	mux.HandleFunc("/anatomy.js", s.handleAnatomyJS)
	mux.HandleFunc("/anatomy-anim.js", s.handleAnatomyAnimJS)
	mux.HandleFunc("/qrcode.min.js", s.handleQRCodeJS)
	mux.HandleFunc("/favicon.ico", s.handleFavicon)
	mux.HandleFunc("/media/", s.handleMedia)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/chat", s.handleChat)
	mux.HandleFunc("/chat/stream", s.handleChatStream)
	mux.HandleFunc("/feedback", s.handleFeedback)
	// Session APIs (server-side persistence for the chat UI)
	if db != nil {
		mux.HandleFunc("/sessions", s.handleSessions)
		mux.HandleFunc("/sessions/", s.handleSessionByID)
		mux.HandleFunc("/family", s.handleFamily)
		mux.HandleFunc("/family/", s.handleFamilyByID)
mux.HandleFunc("/share", s.handleShare)
		mux.HandleFunc("/share/", s.handleSharePage)
	}
	// Admin endpoints
	mux.HandleFunc("/admin/users", s.handleAdminUsers)
	mux.HandleFunc("/admin/users/", s.handleAdminUser)
	// Sync endpoints
	mux.HandleFunc("/admin/sync", s.handleAdminSync)
	mux.HandleFunc("/admin/sync/status", s.handleAdminSyncStatus)
	// Knowledge management (upload/update medical knowledge)
	mux.HandleFunc("/admin/knowledge", s.handleAdminKnowledge)
	mux.HandleFunc("/admin/knowledge/stats", s.handleAdminKnowledgeStats)
	mux.HandleFunc("/admin/knowledge/export", s.handleAdminKnowledgeExport)
	mux.HandleFunc("/admin/knowledge/versions", s.handleAdminKnowledgeVersions)
	// Session management
	mux.HandleFunc("/admin/sessions", s.handleAdminSessions)
	mux.HandleFunc("/admin/sessions/", s.handleAdminSessionByID)
	// Feedback with details
	mux.HandleFunc("/admin/feedback", s.handleAdminFeedback)
	mux.HandleFunc("/admin/feedback/stats", s.handleAdminFeedbackStats)
	// Audit logs
	mux.HandleFunc("/admin/audit-logs", s.handleAdminAuditLogs)
	// System config
	mux.HandleFunc("/admin/config", s.handleAdminConfig)
	mux.HandleFunc("/admin/config/", s.handleAdminConfigByKey)
	// API stats
	mux.HandleFunc("/admin/api-stats", s.handleAdminAPIStats)
	mux.HandleFunc("/admin/api-stats/summary", s.handleAdminAPIStatsSummary)
	// User behavior analysis
	mux.HandleFunc("/admin/analytics", s.handleAdminAnalytics)
	// Batch operations
	mux.HandleFunc("/admin/batch/users", s.handleAdminBatchUsers)
	mux.HandleFunc("/admin/batch/knowledge", s.handleAdminBatchKnowledge)
	// Data export
	mux.HandleFunc("/admin/export", s.handleAdminExport)
	// Admin UI (single-file page, Basic-auth guarded by the browser)
	mux.HandleFunc("/admin", s.handleAdminUI)

	s.http = &http.Server{
		Addr:         fmt.Sprintf("%s:%s", cfg.ServerHost, cfg.ServerPort),
		Handler:      s.withMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// Start begins listening and serving HTTP requests.
func (s *Server) Start() error {
	slog.Info("Starting HTTP server", "addr", s.http.Addr)
	return s.http.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

// handleLanding serves the marketing landing page (embedded single-file HTML,
// no external assets; works offline). It is the site's front door at "/".
func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	serveHTML(w, r, s.pageLanding)
}

// handleMapPage serves the disease-map marketing page (province-level
// prevalence data; see also the crawlable data table in the page).
func (s *Server) handleMapPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/map" {
		http.NotFound(w, r)
		return
	}
	serveHTML(w, r, s.pageMap)
}

// handleStatsPage serves the health-statistics marketing page.
func (s *Server) handleStatsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/stats" {
		http.NotFound(w, r)
		return
	}
	serveHTML(w, r, s.pageStats)
}

// serveHTML writes a pre-rendered page with the house style (GET-only).
func serveHTML(w http.ResponseWriter, r *http.Request, page string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, page)
}

// escapeHTML 转义 HTML 特殊字符。
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// renderMarkdown 简单的 Markdown 转 HTML（用于 PDF 导出）。
func renderMarkdown(md string) string {
	lines := strings.Split(md, "\n")
	var out []string
	var inCodeBlock bool
	var codeContent []string
	var inList bool
	var listType string
	var inTable bool
	var tableRows [][]string

	flushList := func() {
		if inList {
			out = append(out, "</ul>")
			inList = false
		}
	}

	flushTable := func() {
		if len(tableRows) > 0 {
			out = append(out, "<table>")
			for i, row := range tableRows {
				tag := "td"
				if i == 0 {
					tag = "th"
				}
				out = append(out, "<tr>")
				for _, cell := range row {
					cell = strings.TrimSpace(cell)
					out = append(out, "<"+tag+">"+cell+"</"+tag+">")
				}
				out = append(out, "</tr>")
			}
			out = append(out, "</table>")
			tableRows = nil
		}
	}

	for _, line := range lines {
		// 代码块
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inCodeBlock {
				out = append(out, "<pre><code>"+strings.Join(codeContent, "\n")+"</code></pre>")
				codeContent = nil
				inCodeBlock = false
			} else {
				inCodeBlock = true
			}
			continue
		}
		if inCodeBlock {
			codeContent = append(codeContent, line)
			continue
		}

		line = strings.TrimSpace(line)
		if line == "" {
			flushList()
			flushTable()
			continue
		}

		// 跳过 Markdown 表格分隔线
		if strings.HasPrefix(line, "|") && strings.Contains(line, "---") {
			continue
		}

		// 表格
		if strings.HasPrefix(line, "|") {
			flushList()
			cells := strings.Split(strings.Trim(line, "|"), "|")
			var row []string
			for _, cell := range cells {
				cell = strings.TrimSpace(cell)
				row = append(row, cell)
			}
			if len(row) > 0 {
				tableRows = append(tableRows, row)
			}
			inTable = true
			continue
		} else if inTable {
			flushTable()
			inTable = false
		}

		// 标题
		if strings.HasPrefix(line, "#") {
			flushList()
			re := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
			m := re.FindStringSubmatch(line)
			if m != nil {
				level := len(m[1])
				out = append(out, "<h"+fmt.Sprint(level)+">"+m[2]+"</h"+fmt.Sprint(level)+">")
				continue
			}
		}

		// 粗体
		if strings.Contains(line, "**") {
			re := regexp.MustCompile(`\*\*([^*]+)\*\*`)
			line = re.ReplaceAllString(line, "<strong>$1</strong>")
		}

		// 斜体
		if strings.Contains(line, "*") && !strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "*") {
			re := regexp.MustCompile(`\*([^*]+)\*`)
			line = re.ReplaceAllString(line, "<em>$1</em>")
		}

		// 行内代码
		re := regexp.MustCompile("`([^`]+)`")
		line = re.ReplaceAllString(line, "<code>$1</code>")

		// 链接
		re = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
		line = re.ReplaceAllString(line, "<a href=\"$2\" target=\"_blank\">$1</a>")

		// 无序列表
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			flushList()
			if !inList || listType != "ul" {
				flushList()
				out = append(out, "<ul>")
				inList = true
				listType = "ul"
			}
			item := strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")
			out = append(out, "<li>"+item+"</li>")
			continue
		}

		// 有序列表
		re = regexp.MustCompile(`^(\d+)\.\s+(.+)$`)
		m := re.FindStringSubmatch(line)
		if m != nil {
			flushList()
			if !inList || listType != "ol" {
				flushList()
				out = append(out, "<ol>")
				inList = true
				listType = "ol"
			}
			out = append(out, "<li>"+m[2]+"</li>")
			continue
		}

		// 分割线
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "***") {
			out = append(out, "<hr>")
			continue
		}

		// 引用
		if strings.HasPrefix(line, "> ") {
			out = append(out, "<blockquote>"+strings.TrimPrefix(line, "> ")+"</blockquote>")
			continue
		}

		// 普通段落
		flushList()
		out = append(out, "<p>"+line+"</p>")
	}

	flushList()
	flushTable()

	return strings.Join(out, "\n")
}

// renderMarkdownToPDF 将 Markdown 渲染到 PDF 中
func renderMarkdownToPDF(pdf *gofpdf.Fpdf, md string) {
	lines := strings.Split(md, "\n")
	var inList bool
	var listType string
	var inTable bool
	var tableRows [][]string

	flushList := func() {
		if inList {
			pdf.Ln(4)
			inList = false
		}
	}

	flushTable := func() {
		if len(tableRows) > 0 {
			pdf.Ln(4)
			// 计算列宽
			colCount := 0
			if len(tableRows) > 0 {
				colCount = len(tableRows[0])
			}
			if colCount == 0 {
				colCount = 3
			}
			colWidth := 180.0 / float64(colCount)

			// 绘制表头
			pdf.SetFont("Helvetica", "B", 9)
			pdf.SetFillColor(238, 246, 248)
			for _, cell := range tableRows[0] {
				pdf.CellFormat(colWidth, 6, cell, "1", 0, "C", true, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetFont("Helvetica", "", 9)
			pdf.SetFillColor(255, 255, 255)

			// 绘制数据行
			for i := 1; i < len(tableRows); i++ {
				for _, cell := range tableRows[i] {
					pdf.CellFormat(colWidth, 6, cell, "1", 0, "L", false, 0, "")
				}
				pdf.Ln(-1)
			}
			pdf.Ln(4)
			tableRows = nil
			inTable = false
		}
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 跳过 mermaid 代码块
		if strings.HasPrefix(line, "```mermaid") || strings.HasPrefix(line, "```") && strings.Contains(line, "mermaid") {
			continue
		}

		// 跳过空代码块
		if strings.HasPrefix(line, "```") {
			continue
		}

		// 跳过表格分隔线
		if strings.HasPrefix(line, "|") && strings.Contains(line, "---") {
			continue
		}

		// 表格
		if strings.HasPrefix(line, "|") {
			flushList()
			cells := strings.Split(strings.Trim(line, "|"), "|")
			var row []string
			for _, cell := range cells {
				cell = strings.TrimSpace(cell)
				row = append(row, cell)
			}
			if len(row) > 0 && !(len(row) == 1 && row[0] == "") {
				tableRows = append(tableRows, row)
			}
			inTable = true
			continue
		} else if inTable {
			flushTable()
		}

		if line == "" {
			flushList()
			flushTable()
			pdf.Ln(3)
			continue
		}

		// 标题
		if strings.HasPrefix(line, "##") {
			flushList()
			flushTable()
			level := strings.Index(line, " ")
			if level > 0 {
				text := strings.TrimSpace(line[level:])
				fontSize := 14 - (level - 2)
				if fontSize < 10 {
					fontSize = 10
				}
				pdf.SetFont("Helvetica", "B", float64(fontSize))
				pdf.Cell(180, 7, text)
				pdf.Ln(6)
			}
			continue
		}

		// 无序列表
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			flushTable()
			if !inList || listType != "ul" {
				flushList()
				pdf.Ln(2)
				inList = true
				listType = "ul"
			}
			item := strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")
			// 处理粗体
			item = strings.ReplaceAll(item, "**", "")
			pdf.SetFont("Helvetica", "", 10)
			pdf.Cell(8, 5, "•")
			pdf.Cell(170, 5, item)
			pdf.Ln(5)
			continue
		}

		// 有序列表
		re := regexp.MustCompile(`^(\d+)\.\s+(.+)$`)
		m := re.FindStringSubmatch(line)
		if m != nil {
			flushTable()
			if !inList || listType != "ol" {
				flushList()
				pdf.Ln(2)
				inList = true
				listType = "ol"
			}
			pdf.SetFont("Helvetica", "", 10)
			pdf.Cell(8, 5, m[1]+".")
			pdf.Cell(170, 5, m[2])
			pdf.Ln(5)
			continue
		}

		// 分割线
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "***") {
			flushList()
			flushTable()
			pdf.Ln(4)
			pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
			pdf.Ln(4)
			continue
		}

		// 清理 Markdown 格式
		// 移除代码标记
		line = strings.ReplaceAll(line, "`", "")
		// 移除粗体标记
		line = strings.ReplaceAll(line, "**", "")
		// 移除斜体标记
		line = strings.ReplaceAll(line, "*", "")
		// 处理链接 [text](url) -> text
		re = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
		line = re.ReplaceAllString(line, "$1")

		// 普通段落
		flushList()
		flushTable()
		pdf.SetFont("Helvetica", "", 10)
		pdf.MultiCell(180, 5, line, "", "L", false)
	}

	flushList()
	flushTable()
}

// buildPage resolves a page template once per process: the shared base CSS
// plus the page-specific CSS replace the /*__CSS__*/ style placeholder, and
// canonical/og:url tags replace <!--__HEAD_URL__--> when base is configured
// (omitted otherwise — a wrong guessed canonical is worse than none).
func buildPage(tmpl, pageCSS, base, path string) string {
	html := strings.Replace(tmpl, "/*__CSS__*/", cssBase+pageCSS, 1)
	var head strings.Builder
	if base != "" {
		fmt.Fprintf(&head, "<link rel=\"canonical\" href=\"%s%s\" />\n", base, path)
		fmt.Fprintf(&head, "<meta property=\"og:url\" content=\"%s%s\" />", base, path)
	}
	return strings.Replace(html, "<!--__HEAD_URL__-->", head.String(), 1)
}

// publicBaseURL returns the absolute origin for crawler files: the configured
// PUBLIC_BASE_URL when set, else inferred from the request (scheme from TLS /
// X-Forwarded-Proto, host from r.Host) so sitemap.xml etc. still carry valid
// absolute URLs on self-hosted deployments behind a reverse proxy.
func (s *Server) publicBaseURL(r *http.Request) string {
	if s.cfg.PublicBaseURL != "" {
		return s.cfg.PublicBaseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	}
	return scheme + "://" + r.Host
}

// handleRobots serves robots.txt: public marketing pages are open to search
// engines and AI crawlers alike; the API and admin surface stay disallowed.
func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, strings.Replace(robotsTXT, "{BASE}", s.publicBaseURL(r), 1))
}

// handleSitemap serves sitemap.xml for the public marketing pages.
func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	base := s.publicBaseURL(r)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = fmt.Fprintf(w, sitemapXML, base+"/", base+"/map", base+"/stats")
}

// handleLLMsTxt serves llms.txt (https://llmstxt.org): a markdown overview of
// the site written for LLM crawlers — pages, tools, knowledge sources.
func (s *Server) handleLLMsTxt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, strings.ReplaceAll(llmsTXT, "{BASE}", s.publicBaseURL(r)))
}

// robotsTXT is the robots.txt template; {BASE} is replaced per request.
const robotsTXT = `# robots.txt — 医答 Doctor Agent
# 公开内容页（/ /map /stats）对搜索引擎与 AI 爬虫开放；API 与管理端一律禁止。

User-agent: *
Allow: /
Disallow: /admin
Disallow: /chat
Disallow: /sessions
Disallow: /feedback
Disallow: /app

# AI / LLM 爬虫：显式欢迎公开内容页
User-agent: GPTBot
User-agent: ChatGPT-User
User-agent: OAI-SearchBot
User-agent: ClaudeBot
User-agent: Claude-Web
User-agent: anthropic-ai
User-agent: PerplexityBot
User-agent: Google-Extended
User-agent: Bytespider
User-agent: CCBot
User-agent: Amazonbot
User-agent: meta-externalagent
User-agent: Applebot-Extended
User-agent: DuckAssistBot
Allow: /
Disallow: /admin
Disallow: /chat
Disallow: /sessions
Disallow: /feedback
Disallow: /app

Sitemap: {BASE}/sitemap.xml
`

// sitemapXML is the sitemap template; the three %s verbs are filled with the
// site base + page paths per request.
const sitemapXML = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s</loc><changefreq>weekly</changefreq><priority>1.0</priority></url>
  <url><loc>%s</loc><changefreq>monthly</changefreq><priority>0.8</priority></url>
  <url><loc>%s</loc><changefreq>monthly</changefreq><priority>0.8</priority></url>
</urlset>
`

// llmsTXT is the llms.txt template; {BASE} occurrences are replaced per request.
const llmsTXT = `# 医答 Doctor Agent

> 医答是面向中国全人群的免费循证医学 AI 问答助手：基于权威医学知识库与 35+ 项专业医学工具，提供有依据、可溯源的健康解答——从日常小病到中国高发慢病，每个关键结论均附出处（临床指南、PubMed 文献等）。

医答（Doctor Agent）是可自部署的单二进制医聊 AI。回答经过「检索 → 推理 → 核验」闭环，由急诊识别、安全护栏、引用核验三层守护。

重要声明：医答是循证医学辅助工具，仅供健康参考，不能替代专业医生的诊断与治疗；紧急情况请拨打 120 或前往急诊。

## Pages

- [首页]({BASE}/): 产品概览——核心能力、工作原理、35+ 项专业医学工具矩阵、人体部位自诊、常见问题。
- [疾病地图]({BASE}/map): 全国 34 个省级行政区高发疾病分布，按主导类别（遗传代谢 / 肿瘤 / 传染 / 慢病 / 营养寄生）着色，含分省高血压、糖尿病患病率数据。
- [数据洞察]({BASE}/stats): 中国人发病全景 10 个维度的真实权威数据——按病种、年份、地区、省份、性别、年龄、教育程度、职业与精神健康，全部标注出处。

## Tools

- 用药安全核查：检查药物相互作用、禁忌症与剂量安全性
- 症状分诊：根据症状描述推荐就诊科室与紧急程度
- 遗传病风险计算：基于家族史与表型评估遗传性疾病携带风险
- 检验单解读：上传化验单图片，逐项解释指标含义与异常
- 药物相互作用：检测多药并用时的潜在冲突与不良反应风险
- 药物查询：检索药品说明书、适应症、副作用与医保信息

## Knowledge Sources

- 《中国心血管健康与疾病报告 2021/2022》（国家心血管病中心）：历次全国高血压抽样调查（1959–2018）；心血管病现患 3.3 亿
- 中国疾控中心慢病中心：2018 年全国慢性病及危险因素监测；2023 年糖尿病患病人数 2.33 亿及分省差异
- 国家卫生健康委《中国居民营养与慢性病状况报告（2020 年）》：分年龄段高血压患病率；抑郁症 2.1%、焦虑障碍 4.98%
- 国家癌症中心（JNCC 2024）：2022 年恶性肿瘤新发 482.47 万例、死亡 257.42 万例，肺癌居首
- 中国政府网《新中国 75 年经济社会发展成就报告》(2024)：甲乙类传染病报告发病率长期序列（1955→2023）
- 《柳叶刀》：慢阻肺 9990 万（2018 中国成人肺部健康研究）；慢性肾病 1.52 亿（2025）

## Usage

对话入口为浏览器应用（{BASE}/app），需交互式使用；本站内容页均可自由抓取与引用，引用时请保留来源标注。
`

// handleAppUI serves the built-in chat web interface (embedded single-file HTML
// — no build step, no external assets; works offline) at "/app". The chat APIs
// (/chat, /chat/stream, /sessions, ...) remain on their own paths.
func (s *Server) handleAppUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/app" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, webUIIndex)
}

func (s *Server) handleMermaidJS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.WriteString(w, jsMermaid)
}

func (s *Server) handleThreeJS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.WriteString(w, jsThree)
}

// handleAnatomyJS 人体解剖图组件（分层 SVG + 缩放弹窗）。
func (s *Server) handleAnatomyJS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.WriteString(w, jsAnatomy)
}

// handleAnatomyAnimJS 解剖图动画播放器（生理 + 病理动画）。
func (s *Server) handleAnatomyAnimJS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.WriteString(w, jsAnatomyAnim)
}

// handleQRCodeJS 二维码生成库。
func (s *Server) handleQRCodeJS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.WriteString(w, jsQRCode)
}

// handleFavicon 提供网站图标。
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(faviconICO)
}

// mediaNameRe 限制媒体文件名为单级安全文件名。
var mediaNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*\.(webm|mp4|png|jpg)$`)

// handleMedia 提供磁盘媒体文件（3D 渲染动画 webm 等，MEDIA_DIR 目录）。
// 只允许单级文件名（无子目录/路径穿越），带长缓存。
func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/media/")
	if name == "" || name != path.Base(name) || !mediaNameRe.MatchString(name) {
		http.Error(w, "invalid media name", http.StatusBadRequest)
		return
	}
	full := filepath.Join(s.cfg.MediaDir, name)
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.Error(w, "media not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, full)
}

// handleHealth responds with server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "healthy",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"git_commit": s.gitCommit,
		"build_time": s.buildTime,
	})
}

// ChatImage represents an uploaded image in a chat message.
type ChatImage struct {
	Base64Data string `json:"base64_data"`
	MediaType  string `json:"media_type"`
}

// ChatRequest is the JSON body for /chat endpoints.
type ChatRequest struct {
	Message        string      `json:"message"`
	ConversationID string      `json:"conversation_id,omitempty"`
	Images         []ChatImage `json:"images,omitempty"`
	MemberID       int64       `json:"member_id,omitempty"` // 家庭健康档案成员
}

// ChatResponse is the JSON response for /chat.
type ChatResponse struct {
	ConversationID string `json:"conversation_id"`
	Reply          string `json:"reply"`
	IsEmergency    bool   `json:"is_emergency"`
	IsOutOfScope   bool   `json:"is_out_of_scope"`
	Timestamp      string `json:"timestamp"`
	// 本次回答的 token 消耗与估算成本（未知模型价格时 cost 为 0）
	Usage   *llm.TokenUsage `json:"usage,omitempty"`
	CostUSD float64         `json:"cost_usd,omitempty"`
	CostCNY float64         `json:"cost_cny,omitempty"`
	Model   string          `json:"model,omitempty"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Cap the request body to bound memory use; reject oversized messages.
	// Allow up to 10MB for image uploads
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})
		return
	}

	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "message field is required",
		})
		return
	}
	if len(req.Message) > 20000 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "message too long (max 20000 chars)",
		})
		return
	}

	if req.ConversationID == "" || !session.ValidID(req.ConversationID) {
		req.ConversationID = fmt.Sprintf("conv-%d", time.Now().UnixNano())
	}

	sess := s.agent.GetOrCreateSession(req.ConversationID)

	// 家庭健康档案注入: 本次问答自动带上成员背景 (基础病/过敏/用药)
	if req.MemberID > 0 && s.db != nil {
		if err := s.applyFamilyMember(sess, req.MemberID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "member not found"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Convert images to llm.ImageInput format
	var images []llm.ImageInput
	for _, img := range req.Images {
		images = append(images, llm.ImageInput{
			Base64Data: img.Base64Data,
			MediaType:  img.MediaType,
		})
	}

	var resp *agent.Response
	var err error
	if len(images) > 0 {
		resp, err = s.agent.ProcessMessageWithImages(ctx, sess, req.Message, images)
	} else {
		resp, err = s.agent.ProcessMessage(ctx, sess, req.Message)
	}
	if err != nil {
		slog.Error("Agent processing error",
			"error", err,
			"endpoint", "/chat",
			"conversation_id", req.ConversationID,
			"message_len", len(req.Message),
			"has_images", len(images) > 0,
		)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "internal processing error",
		})
		return
	}

	chatResp := ChatResponse{
		ConversationID: req.ConversationID,
		Reply:          resp.Text,
		IsEmergency:    resp.IsEmergency,
		IsOutOfScope:   resp.IsOutOfScope,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}
	if resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
		u := resp.Usage
		chatResp.Usage = &u
		chatResp.CostUSD = resp.CostUSD
		chatResp.CostCNY = resp.CostCNY
		chatResp.Model = resp.Model
	}
	writeJSON(w, http.StatusOK, chatResp)
}

// handleChatStream streams the agent's response as SSE events:
//   - `delta` events carry incremental text chunks as they are generated
//   - a final `done` event carries the complete ChatResponse
//   - `error` events are emitted on processing failure
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Allow up to 10MB for image uploads
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "message field is required"})
		return
	}
	if len(req.Message) > 20000 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "message too long (max 20000 chars)"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Streaming responses can outlive the server's global write timeout.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	_ = rc.EnableFullDuplex()

	if req.ConversationID == "" || !session.ValidID(req.ConversationID) {
		req.ConversationID = fmt.Sprintf("conv-%d", time.Now().UnixNano())
	}
	sess := s.agent.GetOrCreateSession(req.ConversationID)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	sendEvent := func(event string, payload any) {
		data, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	// Collect deltas during generation; the live, pre-verification text must
	// NOT be streamed to the client. L3 citation verification and L4 disclaimer
	// run inside the agent only after the full response is produced, so we
	// buffer and then stream the VERIFIED text below (otherwise post-
	// verification corrections reach the user unguarded via raw deltas).
	var rawDelta strings.Builder
	onDelta := func(chunk string) {
		rawDelta.WriteString(chunk)
	}

	onStep := func(ev agent.StepEvent) {
		sendEvent("step", ev)
	}

	// Convert images to llm.ImageInput format
	var images []llm.ImageInput
	for _, img := range req.Images {
		images = append(images, llm.ImageInput{
			Base64Data: img.Base64Data,
			MediaType:  img.MediaType,
		})
	}

	var resp *agent.Response
	var err error
	if len(images) > 0 {
		resp, err = s.agent.ProcessMessageStreamWithImages(ctx, sess, req.Message, images, onDelta, onStep)
	} else {
		resp, err = s.agent.ProcessMessageStream(ctx, sess, req.Message, onDelta, onStep)
	}
	if err != nil {
		slog.Error("Agent stream processing error",
			"error", err,
			"endpoint", "/chat/stream",
			"conversation_id", req.ConversationID,
			"message_len", len(req.Message),
			"has_images", len(images) > 0,
		)
		sendEvent("error", map[string]any{"error": "internal processing error"})
		return
	}

	// Stream the post-verification, disclaimer-applied response (resp.Text) so
	// the client only ever renders safe content, preserving token-level
	// streaming UX without exposing unverified model output. Emergency
	// short-circuits stay atomic (no deltas), matching the prior contract.
	if !resp.IsEmergency {
		streamVerifiedText(sendEvent, resp.Text)
	}

	doneResp := ChatResponse{
		ConversationID: req.ConversationID,
		Reply:          resp.Text,
		IsEmergency:    resp.IsEmergency,
		IsOutOfScope:   resp.IsOutOfScope,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}
	if resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
		u := resp.Usage
		doneResp.Usage = &u
		doneResp.CostUSD = resp.CostUSD
		doneResp.CostCNY = resp.CostCNY
		doneResp.Model = resp.Model
	}
	sendEvent("done", doneResp)
}

// streamVerifiedText emits the (already post-verification / disclaimer-applied)
// text as SSE delta chunks so the client never sees unverified model output
// during generation.
func streamVerifiedText(send func(string, any), text string) {
	const chunkSize = 24
	runes := []rune(text)
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		send("delta", map[string]any{"text": string(runes[i:end])})
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Warn("Failed to encode JSON response", "error", err)
	}
}

// handleFeedback collects user feedback (thumbs up/down) for responses.
func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4<<10) // 4 KiB
	var req struct {
		SessionID string `json:"session_id"`
		MessageID string `json:"message_id"`
		Rating    string `json:"rating"` // "up" or "down"
		Comment   string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}

	if req.Rating != "up" && req.Rating != "down" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "rating must be 'up' or 'down'"})
		return
	}

	// Log feedback for now (could persist to database later)
	slog.Info("User feedback received",
		"session_id", req.SessionID,
		"message_id", req.MessageID,
		"rating", req.Rating,
		"comment", req.Comment,
		"ip", clientIP(r),
	)

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// handleSessions lists persisted conversations (most recently updated first).
// GET /sessions
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "session persistence disabled"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	recs, err := s.db.ListAllSessions(200)
	if err != nil {
		slog.Error("Listing sessions", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to list sessions"})
		return
	}

	type item struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		UpdatedAt string `json:"updated_at"`
		CreatedAt string `json:"created_at"`
	}
	out := make([]item, 0, len(recs))
	for _, r2 := range recs {
		if r2.ID == "" {
			continue
		}
		out = append(out, item{
			ID:        r2.ID,
			Title:     r2.Title,
			UpdatedAt: r2.UpdatedAt.Format(time.RFC3339),
			CreatedAt: r2.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// handleSessionByID reads, updates, deletes or exports one persisted conversation.
//   GET    /sessions/{id}          → {id, title, messages:[{role, content}]}
//   GET    /sessions/{id}?export=1 → 下载 JSON 文件
//   PUT    /sessions/{id}          → 重命名会话 {title: "新标题"}
//   DELETE /sessions/{id}          → 204
func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "session persistence disabled"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if id == "" || !session.ValidID(id) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid session id"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		// 检查是否有 export 参数
		exportType := r.URL.Query().Get("export")
		if exportType == "pdf" {
			// 导出会话为真正的 PDF 文件
			rec, err := s.db.GetSession(id)
			if err != nil || rec == nil {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			msgs, err := s.db.GetSessionMessages(id)
			if err != nil {
				slog.Error("Getting session messages for PDF export", "id", id, "error", err)
				http.Error(w, "failed to get session messages", http.StatusInternalServerError)
				return
			}
			title := rec.Title
			if title == "" {
				title = "新对话"
			}

			// 生成 PDF
			pdf := gofpdf.New("P", "mm", "A4", "")
			pdf.SetFont("Helvetica", "", 11)
			pdf.AddPage()

			// 标题
			pdf.SetFont("Helvetica", "B", 16)
			pdf.Cell(190, 10, title)
			pdf.Ln(8)

			// 导出时间
			pdf.SetFont("Helvetica", "", 9)
			pdf.SetTextColor(128, 128, 128)
			pdf.Cell(190, 6, "导出时间："+time.Now().Format("2006-01-02 15:04:05"))
			pdf.Ln(10)
			pdf.SetTextColor(0, 0, 0)

			// 遍历消息
			for _, m := range msgs {
				if m.Role == "user" {
					// 用户问题
					pdf.SetFont("Helvetica", "B", 11)
					pdf.SetTextColor(14, 124, 134)
					pdf.Cell(190, 8, "问：")
					pdf.Ln(6)
					pdf.SetFont("Helvetica", "", 10)
					pdf.SetTextColor(0, 0, 0)
					pdf.MultiCell(180, 5, m.Content, "", "L", false)
					pdf.Ln(4)
				} else {
					// AI 回答 - 解析 Markdown 并渲染
					pdf.SetFont("Helvetica", "B", 11)
					pdf.SetTextColor(14, 124, 134)
					pdf.Cell(190, 8, "答：")
					pdf.Ln(6)
					pdf.SetFont("Helvetica", "", 10)
					pdf.SetTextColor(0, 0, 0)
					renderMarkdownToPDF(pdf, m.Content)
					pdf.Ln(8)
				}
			}

			// 输出 PDF
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", "attachment; filename=\""+title+".pdf\"")
			w.Header().Set("Cache-Control", "no-cache")
			pdf.Output(w)
			return
		}
		if exportType == "1" || exportType == "json" {
			// 导出会话为 JSON 文件（保留以兼容旧版）
			rec, err := s.db.GetSession(id)
			if err != nil || rec == nil {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			msgs, err := s.db.GetSessionMessages(id)
			if err != nil {
				slog.Error("Getting session messages for export", "id", id, "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to get session messages"})
				return
			}
			// 构建导出数据
			type exportMsg struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}
			exportData := map[string]any{
				"id":         rec.ID,
				"title":      rec.Title,
				"created_at": rec.CreatedAt.Format(time.RFC3339),
				"updated_at": rec.UpdatedAt.Format(time.RFC3339),
				"messages":   msgs,
			}
			data, err := json.MarshalIndent(exportData, "", "  ")
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to marshal session"})
				return
			}
			// 生成下载文件名
			filename := rec.Title
			if filename == "" {
				filename = "对话"
			}
			// 清理文件名
			invalidChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
			for _, c := range invalidChars {
				filename = strings.ReplaceAll(filename, c, "-")
			}
			filename += ".json"

			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(data)
			return
		}
		// 普通获取会话
		rec, err := s.db.GetSession(id)
		if err != nil {
			slog.Error("Getting session", "id", id, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to get session"})
			return
		}
		if rec == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "session not found"})
			return
		}
		msgs, err := s.db.GetSessionMessages(id)
		if err != nil {
			slog.Error("Getting session messages", "id", id, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to get session messages"})
			return
		}
		type m struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		out := make([]m, 0, len(msgs))
		for _, msg := range msgs {
			out = append(out, m{Role: msg.Role, Content: msg.Content})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":       rec.ID,
			"title":    rec.Title,
			"messages": out,
		})
	case http.MethodPut:
		// 重命名会话
		var req struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
			return
		}
		if req.Title == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "title cannot be empty"})
			return
		}
		if err := s.db.UpdateSessionTitle(id, req.Title); err != nil {
			slog.Error("Renaming session", "id", id, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to rename session"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "title": req.Title})
	case http.MethodDelete:
		if err := s.db.DeleteSession(id); err != nil {
			slog.Error("Deleting session", "id", id, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to delete session"})
			return
		}
		// Drop the in-memory copy too, so a stale agent session can't resurrect it.
		s.agent.DeleteSession(id)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminUsers handles admin user management (create list users).
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	// Check admin authentication
	admin := s.getAdminFromRequest(r)
	if admin == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	switch r.Method {
	case http.MethodPost:
		// Create user
		var input auth.AdminCreateUserInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}

		user, err := s.auth.AdminCreateUser(&input, admin)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"id":       user.ID,
			"username": user.Username,
			"nickname": user.Nickname,
			"is_admin": user.IsAdmin,
			"message":  "用户创建成功",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminUser handles single user operations.
func (s *Server) handleAdminUser(w http.ResponseWriter, r *http.Request) {
	// Check admin authentication
	admin := s.getAdminFromRequest(r)
	if admin == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	// Extract user ID from path
	userID := strings.TrimPrefix(r.URL.Path, "/admin/users/")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "用户ID不能为空"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get user
		user, err := s.auth.GetUserByID(userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if user == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "用户不存在"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":         user.ID,
			"username":   user.Username,
			"nickname":   user.Nickname,
			"is_admin":   user.IsAdmin,
			"created_at": user.CreatedAt,
		})

	case http.MethodDelete:
		// Delete user
		if err := s.auth.DeleteUser(userID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"message": "用户删除成功"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// getAdminFromRequest extracts admin user from request (checks API key or token).
func (s *Server) getAdminFromRequest(r *http.Request) *database.User {
	// 1) Basic auth (admin console): base64 user:pass → Login check.
	if u, p, ok := r.BasicAuth(); ok {
		if s.auth == nil {
			return nil
		}
		user, err := s.auth.Login(&auth.LoginInput{Username: u, Password: p})
		if err == nil && user != nil && user.IsAdmin {
			return user
		}
		// Fall through: a non-admin login is still rejected.
		return nil
	}

	// 2) Bearer API key: matches the configured API key → virtual admin.
	apiKey := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

	// If API key matches, return admin user
	if s.cfg.APIKey != "" && subtle.ConstantTimeCompare([]byte(apiKey), []byte(s.cfg.APIKey)) == 1 {
		// For API key auth, return a virtual admin user
		return &database.User{
			ID:       "admin-api",
			Username: "admin",
			IsAdmin:  true,
		}
	}

	// 3) Token-based auth
	if s.auth != nil {
		user, err := s.auth.GetUserByToken(apiKey)
		if err == nil && user != nil && user.IsAdmin {
			return user
		}
	}

	return nil
}

// publicPaths are reachable without auth (and without rate limiting):
// marketing pages, crawler files and the health probe. Everything else —
// chat/session/admin APIs — stays behind the APIKey gate when one is set.
var publicPaths = map[string]struct{}{
	"/health":      {},
	"/":            {},
	"/index.html":  {}, // served by handleLanding
	"/map":         {},
	"/stats":       {},
	"/robots.txt":  {},
	"/sitemap.xml": {},
	"/llms.txt":    {},
}

// withMiddleware adds security + logging middleware to the handler.
// Order: CORS headers → OPTIONS short-circuit → rate limit → auth → logging.
func (s *Server) withMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.applyCORS(w, r)

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Public pages stay open for browsers, health probes and web/AI
		// crawlers even when APIKey is configured; the API stays gated.
		if _, public := publicPaths[r.URL.Path]; !public {
			if !s.limiter.allow(clientIP(r)) {
				slog.Warn("Rate limit exceeded", "ip", clientIP(r), "path", r.URL.Path)
				writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate limit exceeded"})
				return
			}
			if !s.authenticated(r) {
				slog.Warn("Unauthorized request", "ip", clientIP(r), "path", r.URL.Path)
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
				return
			}
		}

		// Logging
		start := time.Now()
		h.ServeHTTP(w, r)
		slog.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start).String(),
		)
	})
}

// applyCORS sets permissive or allowlisted CORS headers.
func (s *Server) applyCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if len(s.cfg.CORSOrigins) == 0 {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		for _, o := range s.cfg.CORSOrigins {
			if o == origin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
				break
			}
		}
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// authenticated checks the Bearer token when APIKey is configured.
// With an empty APIKey auth is disabled and every request passes.
func (s *Server) authenticated(r *http.Request) bool {
	if s.cfg.APIKey == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	got := strings.TrimPrefix(auth, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.APIKey)) == 1
}

// clientIP extracts the caller IP from RemoteAddr ("ip:port").
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimiter is a minimal fixed-window per-IP limiter (no external deps).
type rateLimiter struct {
	mu     sync.Mutex
	limit  int // requests per window per IP; 0 disables
	window time.Duration
	counts map[string]*rateWindow
}

type rateWindow struct {
	start time.Time
	count int
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{
		limit:  limit,
		window: time.Minute,
		counts: make(map[string]*rateWindow),
	}
}

// allow reports whether a request from ip is within the rate limit.
func (rl *rateLimiter) allow(ip string) bool {
	if rl.limit <= 0 {
		return true
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	w, ok := rl.counts[ip]
	if !ok || now.Sub(w.start) >= rl.window {
		rl.counts[ip] = &rateWindow{start: now, count: 1}
		return true
	}
	w.count++
	allowed := w.count <= rl.limit

	// Opportunistically prune expired entries so the map cannot grow unbounded
	// under bursts of distinct source IPs.
	if len(rl.counts) > 256 {
		for k, v := range rl.counts {
			if now.Sub(v.start) >= rl.window {
				delete(rl.counts, k)
			}
		}
	}
	return allowed
}

// handleAdminSync handles POST /admin/sync for file upload sync.
func (s *Server) handleAdminSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check admin authentication
	admin := s.getAdminFromRequest(r)
	if admin == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	// Parse multipart form (10MB max)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("failed to parse form: %v", err),
		})
		return
	}

	// Get uploaded file
	file, handler, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("failed to get file: %v", err),
		})
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Debug("Failed to close uploaded file", "error", err)
		}
	}()

	// Create temp file
	tempFile, err := os.CreateTemp("", "sync-*.json")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to create temp file: %v", err),
		})
		return
	}
	defer func() {
		if err := os.Remove(tempFile.Name()); err != nil {
			slog.Debug("Failed to remove temp file", "path", tempFile.Name(), "error", err)
		}
	}()

	// Copy uploaded file to temp file
	if _, err := io.Copy(tempFile, file); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to save file: %v", err),
		})
		return
	}
	if err := tempFile.Close(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to close temp file: %v", err),
		})
		return
	}

	// Get source parameter
	source := r.FormValue("source")
	if source == "" || source == "all" {
		// "all" (or absent) means sync every dataset; no filename fallback.
		source = ""
	} else if source == "auto" {
		source = strings.TrimSuffix(handler.Filename, ".json")
		source = strings.TrimSuffix(source, ".json")
	}

	// Get full sync parameter
	fullSync := r.FormValue("full") == "true"

	// Initialize sync components
	store, err := knowledge.Load()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to load knowledge: %v", err),
		})
		return
	}

	vecStore, err := knowledge.NewVectorStore(knowledge.VectorStoreConfig{
		Host:       s.cfg.VectorStoreHost,
		Port:       s.cfg.VectorStorePort,
		Collection: s.cfg.VectorCollection,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to connect to vector store: %v", err),
		})
		return
	}
	defer func() {
		if err := vecStore.Close(); err != nil {
			slog.Debug("Failed to close vector store", "error", err)
		}
	}()

	embedder, err := embedding.NewDefault(s.cfg.EmbeddingBaseURL, s.cfg.EmbeddingAPIKey, s.cfg.EmbeddingModel, s.cfg.EmbeddingDimensions)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to init embedding: %v", err),
		})
		return
	}

	syncer := knowledge.NewSyncer(store, vecStore, embedder)

	// Perform sync
	ctx := context.Background()
	cfg := knowledge.SyncConfig{
		Full:      fullSync,
		Source:    source,
		FilePath:  tempFile.Name(),
		BatchSize: 100,
	}

	var status *knowledge.SyncStatus
	if fullSync {
		status, err = syncer.FullSync(ctx, cfg)
	} else {
		status, err = syncer.IncrementalSync(ctx, cfg)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("sync failed: %v", err),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "sync completed",
		"sync":    status,
	})
}

// handleAdminSyncStatus handles GET /admin/sync/status.
func (s *Server) handleAdminSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check admin authentication
	admin := s.getAdminFromRequest(r)
	if admin == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	// Check if vector store is enabled
	if !s.cfg.VectorStoreEnabled {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "disabled",
			"message": "vector store not enabled",
		})
		return
	}

	// Connect to vector store
	vecStore, err := knowledge.NewVectorStore(knowledge.VectorStoreConfig{
		Host:       s.cfg.VectorStoreHost,
		Port:       s.cfg.VectorStorePort,
		Collection: s.cfg.VectorCollection,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to connect to vector store: %v", err),
		})
		return
	}
	defer func() {
		if err := vecStore.Close(); err != nil {
			slog.Debug("Failed to close vector store", "error", err)
		}
	}()

	// Get stats
	ctx := context.Background()
	stats, err := vecStore.GetSyncStats(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to get stats: %v", err),
		})
		return
	}

	total, err := vecStore.Count(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("failed to count points: %v", err),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"total":  total,
		"stats":  stats,
	})
}

// handleAdminUI serves the single-file admin console (embedded HTML).
// The page itself asks for admin credentials and calls the admin APIs with
// Basic auth; no server-side session is involved.
func (s *Server) handleAdminUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, adminUIIndex)
}

// handleAdminKnowledge handles uploading/updating a medical knowledge dataset.
// POST multipart with a "file" part: <dataset>.json or <dataset>.json.gz.
// The file is classified, replaces the existing dataset rows in MariaDB, and
// the in-memory store is refreshed so running queries see the update.
func (s *Server) handleAdminKnowledge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if err := r.ParseMultipartForm(200 << 20); err != nil { // 200MB headroom for big JSON.gz
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("failed to parse form: %v", err)})
		return
	}
	file, handler, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("failed to get file: %v", err)})
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("reading upload: %v", err)})
		return
	}
	if len(raw) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "empty file"})
		return
	}

	ds, n, err := knowledge.IngestUpload(s.cfg.KnowledgeDBDSN(), handler.Filename, raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	// Refresh the in-memory store so the running agent sees the new data.
	knowledge.Reload()

	slog.Info("Admin updated knowledge dataset", "dataset", ds, "rows", n, "file", handler.Filename, "ip", clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": fmt.Sprintf("数据集 %s 已更新（%d 条）", ds, n),
		"dataset": ds,
		"rows":    n,
	})
}

// handleAdminKnowledgeStats reports per-dataset row counts in the knowledge
// store (MariaDB). GET /admin/knowledge/stats
func (s *Server) handleAdminKnowledgeStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	stats, err := knowledge.DatasetStats(s.cfg.KnowledgeDBDSN())
	if err != nil {
		slog.Error("Admin knowledge stats", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to read stats"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": stats})
}

// handleAdminKnowledgeExport exports a knowledge dataset as JSON.
func (s *Server) handleAdminKnowledgeExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	dataset := r.URL.Query().Get("dataset")
	if dataset == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "dataset parameter required"})
		return
	}

	data, err := knowledge.ExportDataset(s.cfg.KnowledgeDBDSN(), dataset)
	if err != nil {
		slog.Error("Admin knowledge export", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", dataset))
	_, _ = w.Write(data)
}

// handleAdminKnowledgeVersions handles knowledge version management.
func (s *Server) handleAdminKnowledgeVersions(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		dataset := r.URL.Query().Get("dataset")
		if dataset == "" {
			// List all datasets with version counts
			datasets, err := s.db.ListAllKnowledgeDatasets()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"datasets": datasets})
			return
		}
		versions, err := s.db.ListKnowledgeVersions(dataset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"versions": versions})

	case http.MethodPost:
		// Record a new version
		var input struct {
			Dataset    string `json:"dataset"`
			EntryCount int    `json:"entry_count"`
			Checksum   string `json:"checksum"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		admin := s.getAdminFromRequest(r)
		// Get latest version
		latest, _ := s.db.GetLatestKnowledgeVersion(input.Dataset)
		newVersion := 1
		if latest != nil {
			newVersion = latest.Version + 1
		}
		record := &database.KnowledgeVersionRecord{
			Dataset:    input.Dataset,
			Version:    newVersion,
			EntryCount: input.EntryCount,
			Checksum:   input.Checksum,
			CreatedBy:  admin.Username,
		}
		if err := s.db.AddKnowledgeVersion(record); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": record})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminSessions handles session management.
func (s *Server) handleAdminSessions(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get pagination params
		limit := 50
		offset := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			fmt.Sscanf(v, "%d", &limit)
		}
		if v := r.URL.Query().Get("offset"); v != "" {
			fmt.Sscanf(v, "%d", &offset)
		}

		recs, err := s.db.ListAllSessions(limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		// Get message counts for each session
		type sessionWithCount struct {
			database.SessionRecord
			MessageCount int `json:"message_count"`
		}
		var sessions []sessionWithCount
		for _, rec := range recs {
			msgs, _ := s.db.GetSessionMessages(rec.ID)
			sessions = append(sessions, sessionWithCount{
				SessionRecord: rec,
				MessageCount:  len(msgs),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})

	case http.MethodDelete:
		// Bulk delete sessions
		var input struct {
			SessionIDs []string `json:"session_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		var deleted int
		for _, id := range input.SessionIDs {
			if err := s.db.DeleteSession(id); err == nil {
				s.agent.DeleteSession(id)
				deleted++
			}
		}
		// Record audit log
		admin := s.getAdminFromRequest(r)
		if admin != nil {
			_ = s.db.AddAuditLog(&database.AuditLogRecord{
				AdminID:      admin.ID,
				AdminUsername: admin.Username,
				Action:       "batch_delete_sessions",
				TargetType:   "session",
				Details:      fmt.Sprintf("deleted %d sessions", deleted),
				IPAddress:    clientIP(r),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "deleted": deleted})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminSessionByID handles single session operations.
func (s *Server) handleAdminSessionByID(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/admin/sessions/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "session id required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get session with messages
		rec, err := s.db.GetSession(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if rec == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "session not found"})
			return
		}
		msgs, err := s.db.GetSessionMessages(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		type msg struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		var messages []msg
		for _, m := range msgs {
			messages = append(messages, msg{Role: m.Role, Content: m.Content})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":       rec.ID,
			"title":    rec.Title,
			"messages": messages,
		})

	case http.MethodDelete:
		if err := s.db.DeleteSession(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		s.agent.DeleteSession(id)
		// Record audit log
		admin := s.getAdminFromRequest(r)
		if admin != nil {
			_ = s.db.AddAuditLog(&database.AuditLogRecord{
				AdminID:      admin.ID,
				AdminUsername: admin.Username,
				Action:       "delete_session",
				TargetType:   "session",
				TargetID:     id,
				IPAddress:    clientIP(r),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminFeedback handles feedback management.
func (s *Server) handleAdminFeedback(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}

	feedback, err := s.db.GetFeedbackWithDetails(limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"feedback": feedback})
}

// handleAdminFeedbackStats handles feedback statistics.
func (s *Server) handleAdminFeedbackStats(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "day"
	}

	stats, err := s.db.GetFeedbackStatsByPeriod(period)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// Get overall stats
	up, down, _ := s.db.GetFeedbackStats()
	writeJSON(w, http.StatusOK, map[string]any{
		"overall": map[string]any{"up": up, "down": down},
		"by_period": stats,
	})
}

// handleAdminAuditLogs handles audit log management.
func (s *Server) handleAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}

	logs, err := s.db.ListAuditLogs(limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	total, _ := s.db.GetAuditLogsCount()
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs, "total": total})
}

// handleAdminConfig handles system configuration.
func (s *Server) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		configs, err := s.db.ListSystemConfigs()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"configs": configs})

	case http.MethodPost:
		var input struct {
			Key         string `json:"key"`
			Value       string `json:"value"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		admin := s.getAdminFromRequest(r)
		if err := s.db.SetSystemConfig(input.Key, input.Value, input.Description, admin.Username); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		// Record audit log
		_ = s.db.AddAuditLog(&database.AuditLogRecord{
			AdminID:      admin.ID,
			AdminUsername: admin.Username,
			Action:       "set_config",
			TargetType:   "config",
			TargetID:     input.Key,
			Details:      input.Value,
			IPAddress:    clientIP(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminConfigByKey handles single config operations.
func (s *Server) handleAdminConfigByKey(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	key := strings.TrimPrefix(r.URL.Path, "/admin/config/")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "config key required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		value, err := s.db.GetSystemConfig(key)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"key": key, "value": value})

	case http.MethodDelete:
		if err := s.db.DeleteSystemConfig(key); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		// Record audit log
		admin := s.getAdminFromRequest(r)
		if admin != nil {
			_ = s.db.AddAuditLog(&database.AuditLogRecord{
				AdminID:      admin.ID,
				AdminUsername: admin.Username,
				Action:       "delete_config",
				TargetType:   "config",
				TargetID:     key,
				IPAddress:    clientIP(r),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminAPIStats handles API usage statistics.
func (s *Server) handleAdminAPIStats(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}

	stats, err := s.db.ListAPIStats(limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": stats})
}

// handleAdminAPIStatsSummary handles API statistics summary.
func (s *Server) handleAdminAPIStatsSummary(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	hours := 24
	if v := r.URL.Query().Get("hours"); v != "" {
		fmt.Sscanf(v, "%d", &hours)
	}

	summary, err := s.db.GetAPIStatsSummary(hours)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// handleAdminAnalytics handles user behavior analysis.
func (s *Server) handleAdminAnalytics(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	stats, err := s.db.GetUserBehaviorStats()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleAdminBatchUsers handles batch user operations.
func (s *Server) handleAdminBatchUsers(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	switch r.Method {
	case http.MethodPost:
		// Batch create users
		var input struct {
			Users []struct {
				Username string `json:"username"`
				Password string `json:"password"`
				Nickname string `json:"nickname"`
				IsAdmin  bool   `json:"is_admin"`
			} `json:"users"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}

		admin := s.getAdminFromRequest(r)
		var created, failed int
		for _, u := range input.Users {
			_, err := s.auth.AdminCreateUser(&auth.AdminCreateUserInput{
				Username: u.Username,
				Password: u.Password,
				Nickname: u.Nickname,
				IsAdmin:  u.IsAdmin,
			}, admin)
			if err == nil {
				created++
			} else {
				failed++
			}
		}
		// Record audit log
		_ = s.db.AddAuditLog(&database.AuditLogRecord{
			AdminID:      admin.ID,
			AdminUsername: admin.Username,
			Action:       "batch_create_users",
			TargetType:   "user",
			Details:      fmt.Sprintf("created %d, failed %d", created, failed),
			IPAddress:    clientIP(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "created": created, "failed": failed})

	case http.MethodDelete:
		// Batch delete users
		var input struct {
			UserIDs []string `json:"user_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		var deleted int
		for _, id := range input.UserIDs {
			if err := s.auth.DeleteUser(id); err == nil {
				deleted++
			}
		}
		// Record audit log
		admin := s.getAdminFromRequest(r)
		if admin != nil {
			_ = s.db.AddAuditLog(&database.AuditLogRecord{
				AdminID:      admin.ID,
				AdminUsername: admin.Username,
				Action:       "batch_delete_users",
				TargetType:   "user",
				Details:      fmt.Sprintf("deleted %d users", deleted),
				IPAddress:    clientIP(r),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "deleted": deleted})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleAdminBatchKnowledge handles batch knowledge operations.
func (s *Server) handleAdminBatchKnowledge(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// Parse multipart form (200MB max for multiple files)
	if err := r.ParseMultipartForm(200 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("failed to parse form: %v", err)})
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no files provided"})
		return
	}

	type result struct {
		Filename string `json:"filename"`
		Dataset  string `json:"dataset,omitempty"`
		Rows     int    `json:"rows,omitempty"`
		Error    string `json:"error,omitempty"`
	}
	var results []result

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			results = append(results, result{Filename: fileHeader.Filename, Error: err.Error()})
			continue
		}

		raw, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			results = append(results, result{Filename: fileHeader.Filename, Error: err.Error()})
			continue
		}

		ds, n, err := knowledge.IngestUpload(s.cfg.KnowledgeDBDSN(), fileHeader.Filename, raw)
		if err != nil {
			results = append(results, result{Filename: fileHeader.Filename, Error: err.Error()})
			continue
		}

		results = append(results, result{Filename: fileHeader.Filename, Dataset: ds, Rows: n})
	}

	// Refresh knowledge store
	knowledge.Reload()

	// Record audit log
	admin := s.getAdminFromRequest(r)
	if admin != nil {
		_ = s.db.AddAuditLog(&database.AuditLogRecord{
			AdminID:      admin.ID,
			AdminUsername: admin.Username,
			Action:       "batch_upload_knowledge",
			TargetType:   "knowledge",
			Details:      fmt.Sprintf("uploaded %d files", len(files)),
			IPAddress:    clientIP(r),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "results": results})
}

// handleAdminExport handles data export.
func (s *Server) handleAdminExport(w http.ResponseWriter, r *http.Request) {
	if s.getAdminFromRequest(r) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "需要管理员权限"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	exportType := r.URL.Query().Get("type")
	if exportType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "type parameter required (sessions, feedback, audit_logs, config)"})
		return
	}

	switch exportType {
	case "sessions":
		sessions, err := s.db.ListAllSessions(10000)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=sessions.json")
		json.NewEncoder(w).Encode(sessions)

	case "feedback":
		feedback, err := s.db.GetFeedbackWithDetails(10000, 0)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=feedback.json")
		json.NewEncoder(w).Encode(feedback)

	case "audit_logs":
		logs, err := s.db.ListAuditLogs(10000, 0)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=audit_logs.json")
		json.NewEncoder(w).Encode(logs)

	case "config":
		configs, err := s.db.ListSystemConfigs()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=config.json")
		json.NewEncoder(w).Encode(configs)

	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown export type"})
	}
}
