package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
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
		resp, err := ag.ProcessMessageStream(ctx, sess, line, func(chunk string) {
			sb.WriteString(chunk)
			fmt.Print(chunk)
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

func runServe(cfg *config.Config) {
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
			os.Setenv(key, val)
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
