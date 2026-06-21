package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/doctor-agent/internal/agent"
	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/knowledge"
	"github.com/doctor-agent/internal/server"
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
		runVerifyKnowledge()
	case "version":
		fmt.Println("doctor-agent v1.0.0")
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
  doctor-agent version            Print version

Environment:
  ANTHROPIC_API_KEY               Anthropic API key (required)
  ANTHROPIC_MODEL                  Model name (default: claude-sonnet-4-20250514)
  SERVER_HOST                      Server host (default: 0.0.0.0)
  SERVER_PORT                      Server port (default: 8080)
  LOG_LEVEL                        Log level: debug, info, warn, error (default: info)`)
}

func runChat(cfg *config.Config) {
	fmt.Println("🔬 Doctor Agent — 循证医学AI助手 (Evidence-Based Medical AI Assistant)")
	fmt.Println("   专注于中国南方人群 · 纯循证医学 · 每条回答有据可查")
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
		}

		ctx := context.Background()
		resp, err := ag.ProcessMessage(ctx, sess, line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ 错误: %v\n", err)
			continue
		}

		fmt.Println()
		fmt.Println("🤖 医生智能体:")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println(resp.Text)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
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
		srv.Shutdown(context.Background())
	}()

	if err := srv.Start(); err != nil {
		slog.Error("Server error", "error", err)
		os.Exit(1)
	}
}

func runVerifyKnowledge() {
	store, err := knowledge.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 知识库加载失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ 知识库加载成功")
	fmt.Printf("   - 医学知识条目: %d\n", len(store.GetAllMedical()))
	fmt.Printf("   - 药物条目: %d\n", len(store.GetAllDrugs()))
	fmt.Printf("   - 紧急分诊规则: %d\n", len(store.GetAllEmergencyRules()))
	fmt.Printf("   - 引用索引条目: %d\n", len(store.GetReferenceIndex()))

	// Validate citations
	medicalEntries := store.GetAllMedical()
	totalCitations := 0
	entriesWithNoCitations := 0
	for _, entry := range medicalEntries {
		if len(entry.Citations) == 0 {
			fmt.Printf("   ⚠️  条目 '%s' 缺少引用文献\n", entry.ConditionZH)
			entriesWithNoCitations++
		}
		totalCitations += len(entry.Citations)
	}
	fmt.Printf("   - 总引用文献数: %d\n", totalCitations)
	if entriesWithNoCitations > 0 {
		fmt.Printf("   ⚠️  有 %d 个条目缺少引用文献\n", entriesWithNoCitations)
	} else {
		fmt.Println("   ✅ 所有医学条目均包含引用文献")
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
