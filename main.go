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
	"github.com/doctor-agent/internal/auth"
	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/database"
	"github.com/doctor-agent/internal/embedding"
	"github.com/doctor-agent/internal/knowledge"
	"github.com/doctor-agent/internal/server"
)

// Build-time version metadata, injected via:
//
//	go build -ldflags "-X main.gitCommit=... -X main.buildTime=..."
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
	// priority: project .env (highest) and global env vars both win.
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
	// 无参数 = 默认启动网页版（用户友好：双击即可用，浏览器打开 localhost:7071）
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
	case "sync-knowledge":
		runSyncKnowledge(cfg)
	case "seed-knowledge":
		// Materialise the gzip knowledge sources into an external SQLite
		// database (knowledge.db). The binary itself embeds no data.
		//   go run . seed-knowledge [--db=knowledge.db] [--src=internal/knowledge/gz]
		runSeedKnowledge()
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
	doctor-agent sync-knowledge     Sync knowledge to vector database
	doctor-agent seed-knowledge     Build knowledge.db from gzip sources
	doctor-agent version            Print version

Environment:
  LLM_PROVIDER                       LLM provider: deepseek (default) | openai-compat
  DEEPSEEK_API_KEY                   DeepSeek API key (required when LLM_PROVIDER=deepseek)
  DEEPSEEK_MODEL                     DeepSeek model (default: deepseek-v4-pro)
  SERVER_HOST                        Server host (default: 0.0.0.0)
  SERVER_PORT                      Server port (default: 7071)
  API_KEY                          Bearer token for /chat endpoints (default: empty = no auth)
  CORS_ORIGINS                     Comma-separated allowed origins (default: * = all)
  RATE_LIMIT                       Max requests per IP per minute (default: 0 = unlimited)
  SESSION_DIR                      Directory for JSON session snapshots (default: empty = in-memory only)
  LOG_LEVEL                        Log level: debug, info, warn, error (default: info)
  POST_VERIFY_SEMANTIC             Semantic claim verification (default: false)
  POST_VERIFY_JUDGE_MODEL          Judge model for verification (default: reuse main model)

Vector Database:
  VECTOR_STORE_ENABLED             Enable vector database (default: true)
  VECTOR_STORE_HOST                Vector store host (default: localhost)
  VECTOR_STORE_PORT                Vector store port (default: 6333)
  VECTOR_COLLECTION                Vector collection name (default: medical_knowledge)

Embedding:
  EMBEDDING_ENABLED                Enable embedding service (default: true)
  EMBEDDING_BASE_URL               Embedding API base URL (empty = in-process local embedder)
  EMBEDDING_API_KEY                Embedding API key (empty = in-process local embedder)
  EMBEDDING_MODEL                  Embedding model (default: text-embedding-v3)

Sync Command:
  --full, -f                       Full sync (rebuild all vectors)
  --source, -s <source>            Sync specific source (medical, drugs, literature, etc.)
  --file <path>                    Sync specific JSON file
  --batch-size, -b <size>          Batch size for embedding (default: 100)`)
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

	fmt.Println("   正在启动… 请用浏览器打开:  http://localhost:7071")
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

	// Ensure application database exists, then initialize it.
	if err := cfg.EnsureAppDB(); err != nil {
		slog.Error("Failed to ensure application database", "error", err)
		os.Exit(1)
	}

	// Initialize database
	db, err := database.New(database.Config{DSN: cfg.AppDBDSN()})
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Warn("Failed to close database", "error", err)
		}
	}()

	// Initialize auth service
	authSvc := auth.NewService(db)

	// Create initial admin if no users exist
	createInitialAdmin(db, authSvc, cfg)

	ag, err := agent.New(cfg)
	if err != nil {
		slog.Error("Failed to initialize agent", "error", err)
		return
	}

	srv := server.New(cfg, ag, authSvc)

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
		return
	}
}

// createInitialAdmin creates an admin user if no users exist.
func createInitialAdmin(db *database.DB, authSvc *auth.Service, cfg *config.Config) {
	// Check if any users exist
	user, err := db.GetUserByUsername("admin")
	if err != nil {
		slog.Error("Failed to check for admin user", "error", err)
		return
	}

	if user != nil {
		// Admin already exists
		return
	}

	// Create initial admin user
	adminPassword := cfg.AdminPassword
	if adminPassword == "" {
		adminPassword = "admin123" // Default password
	}

	input := &auth.AdminCreateUserInput{
		Username: "admin",
		Password: adminPassword,
		Nickname: "管理员",
		IsAdmin:  true,
	}

	// Create a virtual admin for the creation process
	virtualAdmin := &database.User{
		ID:       "system",
		Username: "system",
		IsAdmin:  true,
	}

	_, err = authSvc.AdminCreateUser(input, virtualAdmin)
	if err != nil {
		slog.Error("Failed to create initial admin user", "error", err)
		return
	}

	slog.Info("Initial admin user created", "username", "admin")
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

func runSeedKnowledge() {
	dbPath := config.Load().KnowledgeDBDSN()
	gzDir := "internal/knowledge/gz"
	for _, a := range os.Args[2:] {
		if v, ok := strings.CutPrefix(a, "--db="); ok {
			dbPath = v
		}
		if v, ok := strings.CutPrefix(a, "--src="); ok {
			gzDir = v
		}
	}
	fmt.Printf("🌱 从 %s 构建知识库 %s ...\n", gzDir, dbPath)
	if err := knowledge.Seed(dbPath, gzDir); err != nil {
		fmt.Fprintf(os.Stderr, "❌ seed 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ 知识库已生成: %s\n", dbPath)
}

func runSyncKnowledge(cfg *config.Config) {
	fmt.Println("🔄 知识库同步到向量数据库")
	fmt.Println()

	// Check if vector store is enabled
	if !cfg.VectorStoreEnabled {
		fmt.Fprintf(os.Stderr, "❌ 向量数据库未启用，请设置 VECTOR_STORE_ENABLED=true\n")
		os.Exit(1)
	}

	// Check if embedding is enabled
	if !cfg.EmbeddingEnabled {
		fmt.Fprintf(os.Stderr, "❌ 嵌入服务未启用，请设置 EMBEDDING_ENABLED=true\n")
		os.Exit(1)
	}

	// Load knowledge base
	fmt.Println("📚 加载知识库...")
	store, err := knowledge.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 知识库加载失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ 知识库加载成功: %d 医学条目, %d 药品条目\n",
		len(store.GetAllMedical()), len(store.GetAllDrugs()))

	// Initialize vector store
	fmt.Println("🗄️  初始化向量数据库...")
	vecStore, err := knowledge.NewVectorStore(knowledge.VectorStoreConfig{
		Host:       cfg.VectorStoreHost,
		Port:       cfg.VectorStorePort,
		Collection: cfg.VectorCollection,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 向量数据库连接失败: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := vecStore.Close(); err != nil {
			fmt.Printf("Warning: failed to close vector store: %v\n", err)
		}
	}()

	// Initialize embedding provider (defaults to the in-process local provider
	// when no remote credentials are configured, so sync works fully offline).
	fmt.Println("🔗 初始化嵌入服务...")
	embedder, err := embedding.NewDefault(cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey, cfg.EmbeddingModel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 嵌入服务初始化失败: %v\n", err)
		return
	}

	// Create syncer
	syncer := knowledge.NewSyncer(store, vecStore, embedder)

	// Parse flags
	fullSync := false
	source := ""
	filePath := ""
	batchSize := 100

	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--full", "-f":
			fullSync = true
		case "--source", "-s":
			if i+1 < len(os.Args) {
				i++
				source = os.Args[i]
			}
		case "--file":
			if i+1 < len(os.Args) {
				i++
				filePath = os.Args[i]
			}
		case "--batch-size", "-b":
			if i+1 < len(os.Args) {
				i++
				if _, err := fmt.Sscanf(os.Args[i], "%d", &batchSize); err != nil {
					fmt.Fprintf(os.Stderr, "⚠️  无效的 batch-size: %s, 使用默认值 100\n", os.Args[i])
					batchSize = 100
				}
			}
		}
	}

	// Perform sync
	fmt.Println()
	if fullSync {
		fmt.Println("🔄 执行全量同步...")
	} else {
		fmt.Println("🔄 执行增量同步...")
	}

	ctx := context.Background()
	cfgSync := knowledge.SyncConfig{
		Full:      fullSync,
		Source:    source,
		FilePath:  filePath,
		BatchSize: batchSize,
	}

	var syncStatus *knowledge.SyncStatus
	if fullSync {
		syncStatus, err = syncer.FullSync(ctx, cfgSync)
	} else {
		syncStatus, err = syncer.IncrementalSync(ctx, cfgSync)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 同步失败: %v\n", err)
		return
	}

	// Print results
	fmt.Println()
	fmt.Println("━━━ 同步完成 ━━━")
	fmt.Printf("✅ 同步时间: %s\n", syncStatus.LastSync.Format("2006-01-02 15:04:05"))
	fmt.Printf("✅ 同步记录: %d / %d\n", syncStatus.SyncedRecords, syncStatus.TotalRecords)

	if len(syncStatus.Errors) > 0 {
		fmt.Printf("⚠️  错误数量: %d\n", len(syncStatus.Errors))
		for _, e := range syncStatus.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	// Show vector store stats
	fmt.Println()
	fmt.Println("━━━ 向量库统计 ━━━")
	stats, err := vecStore.GetSyncStats(ctx)
	if err != nil {
		fmt.Printf("⚠️  获取统计失败: %v\n", err)
	} else {
		for source, count := range stats {
			fmt.Printf("  %s: %d 条\n", source, count)
		}
		total, _ := vecStore.Count(ctx)
		fmt.Printf("  总计: %d 条\n", total)
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
// environment with the LOWEST priority: the project .env (overriding) and
// real environment variables always win (we only set keys that are empty).
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

// loadDotenv reads a .env file and sets environment variables, OVERRIDING any
// values already present in the process environment.
// Priority: project .env (current dir) > user home ~/.env > global env vars
// > ~/.doctor-agent/config.env. Only ONE .env is used (file-level fallback:
// if ./.env exists it wins; otherwise ~/.env is tried). Returns nil when
// neither file exists.
func loadDotenv() error {
	data, err := os.ReadFile(".env")
	if err != nil && os.IsNotExist(err) {
		// Fall back to the user's home directory.
		if home, herr := os.UserHomeDir(); herr == nil {
			data, err = os.ReadFile(filepath.Join(home, ".env"))
			if err != nil && os.IsNotExist(err) {
				return nil
			}
		} else {
			return nil
		}
	}
	if err != nil {
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
		// Highest priority: always apply, even if already set globally.
		_ = os.Setenv(key, val)
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
