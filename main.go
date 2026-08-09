package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/doctor-agent/internal/agent"
	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/knowledge"
	"github.com/doctor-agent/internal/server"
)

// Build-time version metadata, injected via:
//   go build -ldflags "-X main.gitCommit=... -X main.buildTime=..."
var (
	gitCommit = "unknown"
	buildTime = "unknown"
)

func main() {
	// Load .env file if present (silently ignore if not found)
	if err := loadDotenv(); err != nil {
		slog.Debug("No .env file loaded", "error", err)
	}
	// Load the user-level config (~/.doctor-agent/config.env) — lowest
	// priority, so real environment variables and .env always win.
	if err := loadUserConfig(); err != nil {
		slog.Debug("No user config loaded", "error", err)
	}

	cfg := config.Load()

	// Setup structured logging
	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})))

	// Parse subcommand
	// 无参数 = 默认启动网页版（用户友好：双击即可用，浏览器打开 localhost:8080）
	if len(os.Args) < 2 {
		startWebUI(cfg)
		return
	}

	cmd := os.Args[1]
	switch cmd {
	case "chat":
		runChat(cfg)
	case "serve":
		runServe(cfg)
	case "verify-knowledge":
		// Optional flag: go run . verify-knowledge -urls  (online URL liveness check)
		checkURLs := len(os.Args) > 2 && os.Args[2] == "-urls"
		runVerifyKnowledge(checkURLs)
	case "version":
		fmt.Printf("doctor-agent v1.0.0 (commit %s, built %s)\n", gitCommit, buildTime)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Doctor Agent — 循证医学AI助手 (Evidence-Based Medical AI Assistant)

Usage:
  doctor-agent chat               Interactive CLI chat mode
  doctor-agent serve              Start HTTP API server
  doctor-agent verify-knowledge   Validate knowledge base files
  doctor-agent verify-knowledge -urls   Also probe citation URLs (online)
  doctor-agent version            Print version

Environment:
  LLM_PROVIDER                       LLM provider: deepseek (default) | openai-compat
  DEEPSEEK_API_KEY                   DeepSeek API key (required when LLM_PROVIDER=deepseek)
  DEEPSEEK_MODEL                     DeepSeek model (default: deepseek-v4-pro)
  SERVER_HOST                        Server host (default: 0.0.0.0)
  SERVER_PORT                      Server port (default: 8080)
  API_KEY                          Bearer token for /chat endpoints (default: empty = no auth)
  CORS_ORIGINS                     Comma-separated allowed origins (default: * = all)
  RATE_LIMIT                       Max requests per IP per minute (default: 0 = unlimited)
  SESSION_DIR                      Directory for JSON session snapshots (default: empty = in-memory only)
  LOG_LEVEL                        Log level: debug, info, warn, error (default: info)
  POST_VERIFY_SEMANTIC             Semantic claim verification (default: false)
  POST_VERIFY_JUDGE_MODEL          Judge model for verification (default: reuse main model)`)
}

func runChat(cfg *config.Config) {
	fmt.Println("🔬 Doctor Agent — 循证医学AI助手 (Evidence-Based Medical AI Assistant)")
	fmt.Println("   专注全中国人群 · 纯循证医学 · 每条回答有据可查")
	fmt.Println("   输入 'quit' 或 'exit' 退出 | 输入 'help' 查看帮助")
	fmt.Println()

	// 首次运行引导：缺 API Key 时，交互式配置一次并保存，之后免配置。
	if err := cfg.Validate(); err != nil {
		if setupErr := setupAPIKey(); setupErr != nil {
			slog.Error("API Key 配置失败", "error", setupErr)
			os.Exit(1)
		}
		_ = loadUserConfig() // 把刚保存的配置读进环境变量
		cfg = config.Load()
	}

	ag, err := agent.New(cfg)
	if err != nil {
		slog.Error("Failed to initialize agent", "error", err)
		os.Exit(1)
	}

	sess := ag.GetOrCreateSession("cli-session")

	fmt.Println("✅ 知识库加载完成，您可以开始咨询。")
	fmt.Println()

	// Simple readline-like loop
	scanner := newLineScanner()
	for {
		fmt.Print("👤 您: ")
		line, ok := scanner()
		if !ok {
			break
		}
		if line == "" {
			continue
		}

		switch line {
		case "quit", "exit", "q":
			fmt.Println("👋 再见！")
			return
		case "help", "h":
			printChatHelp()
			continue
		case "clear":
			sess.Clear()
			fmt.Println("🧹 对话历史已清除。")
			continue
		}

		ctx := context.Background()
		fmt.Println()
		fmt.Println("🤖 医生智能体:")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		var sb strings.Builder
		stepIcons := map[string]string{
			"emergency": "🚨", "refuse": "🚫", "retrieve": "🔍",
			"tool_call": "🛠️", "tool_result": "✅", "generate": "✍️", "verify": "🛡️",
		}
		resp, err := ag.ProcessMessageStream(ctx, sess, line,
			func(chunk string) {
				sb.WriteString(chunk)
				fmt.Print(chunk)
			},
			func(ev agent.StepEvent) {
				icon := stepIcons[ev.Type]
				if icon == "" {
					icon = "·"
				}
				fmt.Printf("\n  \x1b[90m%s %s\x1b[0m\n", icon, ev.Summary)
			})
		fmt.Println()
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ 错误: %v\n", err)
			continue
		}
		// Non-streaming paths (emergency / scope guard) produce no deltas;
		// fall back to the returned text so the response is still shown.
		if sb.Len() == 0 && resp != nil && resp.Text != "" {
			fmt.Println(resp.Text)
		}
	}
}

func printChatHelp() {
	fmt.Println(`
命令:
  quit / exit / q    退出程序
  help / h           显示此帮助
  @region <地区>     设置所在地区（如 @region guangdong）
  @g6pd <状态>       设置G6PD状态（如 @g6pd deficient）
  @thal <状态>       设置地贫携带状态（如 @thal alpha）
  clear              清除对话历史

Chat simply by typing your medical question. The agent will:
1. Screen for medical emergencies
2. Retrieve relevant evidence-based knowledge
3. Provide structured clinical analysis with citations
4. Add disclaimers and safety warnings`)
}

// startWebUI is the default entry for double-clicked binaries: it runs the
// HTTP server (with embedded web chat UI) after a one-time key setup.
func startWebUI(cfg *config.Config) {
	fmt.Println("🩺 Doctor Agent — 循证医学AI助手（网页版）")
	fmt.Println()

	if err := cfg.Validate(); err != nil {
		if setupErr := setupAPIKey(); setupErr != nil {
			slog.Error("API Key 配置失败", "error", setupErr)
			os.Exit(1)
		}
		_ = loadUserConfig()
		cfg = config.Load()
	}

	fmt.Println("   正在启动… 请用浏览器打开:  http://localhost:8080")
	fmt.Println("   按 Ctrl+C 退出")
	fmt.Println()
	runServe(cfg)
}

func runServe(cfg *config.Config) {
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 配置不完整: %v\n", err)
		fmt.Fprintln(os.Stderr, "请先运行一次 `doctor-agent chat` 完成交互配置（只需一次），")
		fmt.Fprintln(os.Stderr, "或编辑 ~/.doctor-agent/config.env，或设置环境变量。")
		os.Exit(1)
	}

	ag, err := agent.New(cfg)
	if err != nil {
		slog.Error("Failed to initialize agent", "error", err)
		os.Exit(1)
	}

	srv := server.New(cfg, ag)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		slog.Info("Shutting down...")
		if err := srv.Shutdown(context.Background()); err != nil {
			slog.Warn("Graceful shutdown error", "error", err)
		}
	}()

	if err := srv.Start(); err != nil {
		slog.Error("Server error", "error", err)
		os.Exit(1)
	}
}

func runVerifyKnowledge(checkURLs bool) {
	store, err := knowledge.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 知识库加载失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ 知识库加载成功")
	fmt.Println()
	fmt.Println("━━━ 知识库完整性校验 ━━━")
	fmt.Println()

	report := knowledge.VerifyData(store)
	fmt.Print(knowledge.ReportText(report))

	if checkURLs {
		fmt.Println()
		fmt.Println("━━━ URL 可达性检查 ━━━")
		fmt.Println("正在探测引用 URL（每个超时 10s，并发 8）...")
		issues := knowledge.CheckURLLiveness(store, 10*time.Second, 8)
		if len(issues) == 0 {
			fmt.Println("✅ 所有引用 URL 均可达")
		} else {
			fmt.Printf("⚠️  %d 个 URL 不可达:\n", len(issues))
			for _, it := range issues {
				fmt.Printf("  - [%s] %s → %v\n", it.ID, it.URL, it.Err)
			}
		}
	}

	if len(report.Errors) > 0 || len(report.EntryIDIssues) > 0 || len(report.CitationIssues) > 0 {
		fmt.Println()
		fmt.Println("⚠️  校验发现错误，请修复后再发布。")
		os.Exit(1)
	}
	if len(report.Warnings) > 0 {
		fmt.Println()
		fmt.Println("⚠️  校验存在警告（不影响可用性），建议完善数据。")
	}
}

// userConfigPath returns the user-level config file path
// (~/.doctor-agent/config.env), where the first-run setup stores the API key.
func userConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".doctor-agent", "config.env")
}

// loadUserConfig reads ~/.doctor-agent/config.env and applies it to the
// environment with the LOWEST priority: real environment variables and the
// project .env always win (we only set keys that are still empty).
func loadUserConfig() error {
	path := userConfigPath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	return nil
}

// setupAPIKey is the first-run interactive setup: it asks the user for an API
// key (recommending the free 智谱 glm-4-flash option), persists it to
// ~/.doctor-agent/config.env and never asks again.
func setupAPIKey() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("🔑 首次使用：配置一个 API Key（只需这一次，之后免配置）")
	fmt.Println()
	fmt.Println("推荐：智谱 glm-4-flash —— 免费、国内直连")
	fmt.Println("      获取: https://open.bigmodel.cn 注册 → 控制台 → API Keys")
	fmt.Println("备选：DeepSeek —— https://platform.deepseek.com")
	fmt.Println("      豆包(火山方舟) —— https://console.volcengine.com/ark 开通")
	fmt.Println()
	fmt.Print("选择模型 [1=智谱 glm-4-flash(推荐), 2=DeepSeek, 3=豆包]（默认 1）: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	fmt.Print("请粘贴你的 API Key（粘贴后回车）: ")
	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("API Key 不能为空")
	}

	var content string
	switch {
	case strings.HasPrefix(choice, "3"):
		// 豆包（火山方舟）— OpenAI 兼容端点，model 可直接用模型名
		content = "# doctor-agent user config (created by first-run setup)\n" +
			"LLM_PROVIDER=openai-compat\n" +
			"OPENAI_COMPAT_BASE_URL=https://ark.cn-beijing.volces.com/api/v3\n" +
			"OPENAI_COMPAT_API_KEY=" + key + "\n" +
			"OPENAI_COMPAT_MODEL=doubao-seed-2-1-pro-260628\n"
	case strings.HasPrefix(choice, "2"):
		content = "# doctor-agent user config (created by first-run setup)\n" +
			"LLM_PROVIDER=deepseek\n" +
			"DEEPSEEK_API_KEY=" + key + "\n"
	default:
		content = "# doctor-agent user config (created by first-run setup)\n" +
			"LLM_PROVIDER=openai-compat\n" +
			"OPENAI_COMPAT_BASE_URL=https://open.bigmodel.cn/api/paas/v4\n" +
			"OPENAI_COMPAT_API_KEY=" + key + "\n" +
			"OPENAI_COMPAT_MODEL=glm-4-flash\n"
	}

	path := userConfigPath()
	if path == "" {
		return fmt.Errorf("无法确定用户主目录")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建配置目录: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("保存配置: %w", err)
	}

	fmt.Printf("✅ 已保存到 %s，之后无需再配置。\n", path)
	fmt.Println()
	return nil
}

// loadDotenv reads a .env file in the current directory and sets environment
// variables. Returns nil if the file does not exist.
func loadDotenv() error {
	data, err := os.ReadFile(".env")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Only set if not already present in the environment
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	return nil
}

// newLineScanner returns a function that reads one line from stdin.
// Simple implementation without external dependencies.
func newLineScanner() func() (string, bool) {
	buf := make([]byte, 0, 4096)
	return func() (string, bool) {
		buf = buf[:0]
		for {
			b := make([]byte, 1)
			n, err := os.Stdin.Read(b)
			if err != nil || n == 0 {
				return "", false
			}
			if b[0] == '\n' {
				return string(buf), true
			}
			buf = append(buf, b[0])
		}
	}
}
