package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/embedding"
	"github.com/doctor-agent/internal/knowledge"
	"github.com/doctor-agent/internal/llm"
	"github.com/doctor-agent/internal/prompt"
	"github.com/doctor-agent/internal/rerank"
	"github.com/doctor-agent/internal/safety"
	"github.com/doctor-agent/internal/session"
	"github.com/doctor-agent/internal/tools"
)

// Agent orchestrates the complete medical AI pipeline.
type Agent struct {
	cfg       *config.Config
	provider  llm.LLMProvider
	store     *knowledge.Store
	retriever knowledge.Retriever
	// reranker optionally rescores the final fused candidate list
	// (cross-encoder precision pass); nil = RRF order only.
	reranker knowledge.Reranker
	composer *prompt.Composer
	registry *tools.Registry
	// understandProvider runs the per-message colloquial→clinical query
	// understanding step (cheaper/faster model than the main loop when
	// UNDERSTAND_MODEL is configured; same provider otherwise).
	understandProvider llm.LLMProvider

	router            *tools.Router
	emergencyDetector *safety.EmergencyDetector
	scopeGuard        *safety.ScopeGuard
	postVerifier      *safety.PostVerifier

	sessionsMu   sync.RWMutex
	sessions     map[string]*session.Session
	sessionStore session.Store // optional on-disk or database session store
	sessLocks    sync.Map      // sessionID -> *sync.Mutex, serializes turns per session

	// Session map bounds (see reapIdleSessions); zero values disable the rule.
	sessionIdleTTL time.Duration
	maxSessions    int
	lastSweep      time.Time
}

// New creates a fully initialized Agent.
func New(cfg *config.Config) (*Agent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	store, err := knowledge.Load()
	if err != nil {
		return nil, fmt.Errorf("loading knowledge base: %w", err)
	}
	slog.Info("Knowledge base loaded",
		"medical_entries", len(store.GetAllMedical()),
		"drug_entries", len(store.GetAllDrugs()),
		"emergency_rules", len(store.GetAllEmergencyRules()))

	// Optional external alias dictionary for colloquial→clinical query
	// expansion ("兔唇" → 唇腭裂/唇裂/腭裂). Missing file is fine: the
	// built-in synonym groups still apply.
	if err := knowledge.LoadAliasFile(cfg.AliasMapPath); err != nil {
		slog.Warn("Alias map failed to load; using built-in synonyms only", "path", cfg.AliasMapPath, "error", err)
	}

	// Create LLM provider based on config
	provider, err := createProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating LLM provider: %w", err)
	}
	slog.Info("LLM provider initialized", "provider", provider.Name())

	keywordRetriever := knowledge.NewRetriever(store)
	var retriever knowledge.Retriever = keywordRetriever

	if cfg.VectorStoreEnabled && cfg.EmbeddingEnabled {
		embedder, embedErr := embedding.NewDefault(cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey, cfg.EmbeddingModel, cfg.EmbeddingDimensions)
		if embedErr != nil {
			slog.Warn("Embedding provider unavailable; using keyword-only retrieval (set EMBEDDING_BASE_URL to enable semantic retrieval)", "error", embedErr)
		} else {
			vecStore, vecErr := knowledge.NewVectorStore(knowledge.VectorStoreConfig{
				Host:       cfg.VectorStoreHost,
				Port:       cfg.VectorStorePort,
				Collection: cfg.VectorCollection,
				Dimensions: embedder.Dimensions(),
			})
			if vecErr != nil {
				slog.Warn("Vector store unavailable; using keyword-only retrieval", "error", vecErr)
			} else {
				vectorRetriever := knowledge.NewVectorRetriever(vecStore, embedder, store)
				retriever = knowledge.NewHybridRetriever(keywordRetriever, vectorRetriever, 0.4)
				slog.Info("Hybrid retrieval enabled (keyword + vector)",
					"embedder", embedder.Name(), "collection", cfg.VectorCollection)
			}
		}
	}
	// Optional cross-encoder rerank: a second precision pass over the fused
	// candidate pool (see retrieveWithUnderstanding). Deliberately opt-in —
	// it needs a reachable /rerank service, and a missing one only loses the
	// reordering, never retrieval itself.
	var reranker knowledge.Reranker
	if cfg.RerankEnabled {
		if cfg.RerankBaseURL == "" {
			slog.Warn("RERANK_ENABLED set but RERANK_BASE_URL empty; rerank disabled")
		} else if rp, rerr := rerank.New(cfg.RerankBaseURL, "", cfg.RerankModel); rerr != nil {
			slog.Warn("Rerank provider unavailable; keeping RRF-only ranking", "error", rerr)
		} else {
			reranker = rp
			slog.Info("Cross-encoder rerank enabled", "endpoint", cfg.RerankBaseURL, "model", cfg.RerankModel)
		}
	}
	composer := prompt.NewComposer()
	registry := tools.NewRegistry()
	router := tools.NewRouter()

	// Query-understanding provider: optionally a cheaper/faster
	// OpenAI-compatible model than the main conversation loop.
	understandProvider := provider
	if cfg.UnderstandModel != "" {
		if cfg.LLMProvider == "openai-compat" {
			understandProvider = llm.NewOpenAICompatProvider(
				cfg.OpenAICompatBaseURL, cfg.OpenAICompatAPIKey, cfg.UnderstandModel, "", 4096, 0.1)
			slog.Info("Query understanding uses dedicated model", "model", cfg.UnderstandModel)
		} else {
			slog.Info("UNDERSTAND_MODEL ignored: only openai-compat supports a separate understanding model")
		}
	}
	// Disable thinking mode for query understanding: it's a fast
	// classification/extraction task, and DeepSeek V4's default thinking
	// wastes seconds and can exhaust max_tokens on reasoning alone.
	if p, ok := understandProvider.(*llm.DeepSeekProvider); ok {
		understandProvider = p.WithThinkingDisabled()
	} else if p, ok := understandProvider.(*llm.OpenAICompatProvider); ok {
		understandProvider = p.WithThinkingDisabled()
	}

	// Register unified tools (13 total: 9 action + 2 unified retrieval/lookup
	// + 2 knowledge-graph lookup).
	// Action tools — computation / cross-reference, not replaceable by RAG.
	registry.Register(tools.NewDrugSafetyCheck(store))
	registry.Register(tools.NewGeneticRiskCalculator(store))
	registry.Register(tools.NewFoodRiskAnalyzer(store))
	registry.Register(tools.NewSymptomTriage(store))
	registry.Register(tools.NewDrugInteractionCheckTool(store))
	registry.Register(tools.NewMedicalImageAnalyze(provider))
	registry.Register(tools.NewLabReportAnalyze())
	registry.Register(tools.NewVisitPrep())
	// FDA 标签中文要点 (344 常用药: 禁忌/警告/相互作用/剂量) — 用药安全问答核心
	registry.Register(tools.NewDrugLabelLookup(store))
	// Unified retrieval / lookup — replace ~28 retired specialized tools.
	registry.Register(tools.NewKnowledgeSearch(store, retriever))
	registry.Register(tools.NewExactLookup(store))
	// Knowledge-graph triple lookup (OpenCMKG 354,752 triples, CPubMed-KG 105,328
	// triples) — exact-match entity/relation queries, not replaceable by RAG.
	registry.Register(tools.NewMedicalKGLookup(store))
	registry.Register(tools.NewCPubMedKGLookup(store))

	postVerifier := safety.NewPostVerifier(store.GetReferenceIndex())
	if cfg.JudgeEnabled {
		judge, err := createJudgeProvider(cfg)
		if err != nil {
			return nil, fmt.Errorf("creating judge provider: %w", err)
		}
		slog.Info("Semantic claim verification enabled", "judge", judge.Name())
		postVerifier = safety.NewPostVerifierWithJudge(store.GetReferenceIndex(), judge)
	}

	// Optional session persistence (file or database).
	var sessionStore session.Store
	if cfg.SessionDir != "" {
		fileStore, err := session.NewFileStore(cfg.SessionDir)
		if err != nil {
			return nil, fmt.Errorf("initializing session store: %w", err)
		}
		sessionStore = fileStore
		slog.Info("Session persistence enabled", "type", "file", "dir", cfg.SessionDir)
	}

	return &Agent{
		cfg:                cfg,
		provider:           provider,
		understandProvider: understandProvider,
		store:              store,
		retriever:          retriever,
		reranker:           reranker,
		composer:           composer,
		registry:           registry,
		router:             router,
		emergencyDetector:  safety.NewEmergencyDetector(),
		scopeGuard:         safety.NewScopeGuard(),
		postVerifier:       postVerifier,
		sessions:           make(map[string]*session.Session),
		sessionStore:       sessionStore,
		sessionIdleTTL:     time.Duration(cfg.SessionIdleMinutes) * time.Minute,
		maxSessions:        cfg.MaxActiveSessions,
	}, nil
}

func createProvider(cfg *config.Config) (llm.LLMProvider, error) {
	switch cfg.LLMProvider {
	case "anthropic":
		return llm.NewAnthropicProvider(
			cfg.AnthropicAPIKey,
			cfg.AnthropicModel,
			cfg.MaxTokens,
			cfg.Temperature,
		), nil
	case "deepseek":
		return llm.NewDeepSeekProvider(
			cfg.DeepSeekAPIKey,
			cfg.DeepSeekModel,
			cfg.DeepSeekVisionModel,
			cfg.MaxTokens,
			cfg.Temperature,
		), nil
	case "openai-compat":
		return llm.NewOpenAICompatProvider(
			cfg.OpenAICompatBaseURL,
			cfg.OpenAICompatAPIKey,
			cfg.OpenAICompatModel,
			cfg.OpenAICompatVisionModel,
			cfg.MaxTokens,
			cfg.Temperature,
		), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", cfg.LLMProvider)
	}
}

// createJudgeProvider builds a low-temperature LLM provider used for
// claim-support verification. Uses the judge model if configured, otherwise
// reuses the main model at temperature 0 for deterministic verdicts.
func createJudgeProvider(cfg *config.Config) (llm.LLMProvider, error) {
	switch cfg.LLMProvider {
	case "anthropic":
		model := cfg.JudgeModel
		if model == "" {
			model = cfg.AnthropicModel
		}
		return llm.NewAnthropicProvider(cfg.AnthropicAPIKey, model, 2048, 0), nil
	case "deepseek":
		model := cfg.JudgeModel
		if model == "" {
			model = cfg.DeepSeekModel
		}
		// Judge is a deterministic yes/no claim-support check — disable
		// thinking to cut latency and avoid reasoning eating max_tokens.
		return llm.NewDeepSeekProvider(cfg.DeepSeekAPIKey, model, "", 2048, 0).WithThinkingDisabled(), nil
	case "openai-compat":
		model := cfg.JudgeModel
		if model == "" {
			model = cfg.OpenAICompatModel
		}
		return llm.NewOpenAICompatProvider(cfg.OpenAICompatBaseURL, cfg.OpenAICompatAPIKey, model, "", 2048, 0).WithThinkingDisabled(), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", cfg.LLMProvider)
	}
}

// ProcessMessage handles a single user message within a conversation session
// without streaming (equivalent to ProcessMessageStream with nil callbacks).
func (a *Agent) ProcessMessage(ctx context.Context, sess *session.Session, userMessage string) (*Response, error) {
	return a.ProcessMessageStream(ctx, sess, userMessage, nil, nil)
}

// ProcessMessageWithImages handles a user message with attached images.
func (a *Agent) ProcessMessageWithImages(ctx context.Context, sess *session.Session, userMessage string, images []llm.ImageInput) (*Response, error) {
	return a.ProcessMessageStreamWithImages(ctx, sess, userMessage, images, nil, nil)
}

// StepEvent describes one visible step of the agent's pipeline (retrieval,
// tool use, generation, verification). Clients (web UI / CLI) subscribe via
// the onStep callback of ProcessMessageStream to show the user what the agent
// is doing while it works.
type StepEvent struct {
	Type    string `json:"type"` // "emergency" | "refuse" | "retrieve" | "tool_call" | "tool_result" | "generate" | "verify"
	Tool    string `json:"tool,omitempty"`
	Summary string `json:"summary"` // Chinese, human-readable
}

// streamWithRetry wraps provider streaming with a short backoff retry for
// transient provider failures (HTTP 429 rate limits, 5xx, dropped
// connections). Retrying is skipped once any delta has already been emitted
// to the user, because a fresh stream would replay the partial answer.
// cachedPrefix is the byte-stable static section of the system prompt (a
// strict prefix of systemPrompt); when the provider supports prompt caching
// it gets an explicit cache breakpoint, otherwise it is ignored.
func (a *Agent) streamWithRetry(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, cachedPrefix, systemPrompt string, onDelta func(string)) (*llm.ChatResponse, error) {
	var emitted bool
	wrapped := onDelta
	if onDelta != nil {
		wrapped = func(d string) {
			emitted = true
			onDelta(d)
		}
	}
	cacheProvider, canCache := a.provider.(llm.PromptCacheProvider)
	useCache := canCache && cachedPrefix != "" && strings.HasPrefix(systemPrompt, cachedPrefix)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt) * 1500 * time.Millisecond
			slog.Warn("Transient LLM error, retrying", "attempt", attempt, "wait", wait, "error", lastErr)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		var resp *llm.ChatResponse
		var err error
		if useCache {
			resp, err = cacheProvider.StreamChatCached(ctx, messages, tools, cachedPrefix, systemPrompt[len(cachedPrefix):], wrapped)
		} else {
			resp, err = a.provider.StreamChat(ctx, messages, tools, systemPrompt, wrapped)
		}
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if emitted || !isTransientLLMError(err) {
			return nil, fmt.Errorf("LLM error: %w", err)
		}
	}
	return nil, fmt.Errorf("LLM error after retries: %w", lastErr)
}

// isTransientLLMError reports whether the error looks like a temporary
// provider-side failure that a retry can fix.
// queryUnderstanding is the parsed output of the per-message understanding
// step: structured clinical concepts extracted from colloquial patient
// language.
type queryUnderstanding struct {
	Symptoms            []string `json:"symptoms"`
	SuspectedConditions []string `json:"suspected_conditions"`
	SearchQueries       []string `json:"search_queries"`
}

// selfCheckAnswer performs a lightweight rule-based completeness check on the
// final answer. The project's answer structure is: 可能的原因 → 相似情况/
// 常见病例 → 家庭护理 → 何时就医. This function verifies the answer
// contains the core sections and logs a warning when it appears incomplete.
// It does NOT modify the answer — the LLM may legitimately omit a section
// for simple questions (e.g. "布洛芬能退烧吗" doesn't need 何时就医).
// Returns a list of missing section names (empty = all present).
func selfCheckAnswer(userMessage, answer string) []string {
	if len([]rune(answer)) < 30 {
		return []string{"回答过短"}
	}
	sections := []struct {
		name     string
		keywords []string
	}{
		{"可能的原因", []string{"原因", "可能", "常见病因", "引起", "导致"}},
		{"家庭护理", []string{"护理", "休息", "饮食", "多喝水", "观察", "家庭", "建议", "可以"}},
		{"何时就医", []string{"就医", "医院", "医生", "急诊", "及时", "尽快", "严重"}},
	}
	var missing []string
	// Simple factual questions (drug dosage, definition) don't need the full
	// structure — only enforce structure for symptom-style questions.
	isSymptomQuery := false
	for _, kw := range []string{"疼", "痛", "痒", "晕", "恶心", "呕吐", "腹泻", "发烧", "发热", "咳嗽", "不舒服", "难受", "症状", "怎么办", "怎么回事"} {
		if strings.Contains(userMessage, kw) {
			isSymptomQuery = true
			break
		}
	}
	if !isSymptomQuery {
		return nil
	}
	for _, sec := range sections {
		found := false
		for _, kw := range sec.keywords {
			if strings.Contains(answer, kw) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, sec.name)
		}
	}
	return missing
}

// needsClarification reports whether the user message is too vague to answer
// safely. A message is vague when it names only a symptom ("头疼", "肚子疼")
// without duration, severity, accompanying symptoms, or context. In that case
// the agent should ask 2-3 targeted follow-up questions instead of jumping to
// a differential diagnosis. The check is rule-based (no extra LLM call):
// short message + symptom keyword + absence of detail markers = vague.
func needsClarification(userMessage string) bool {
	msg := strings.TrimSpace(userMessage)
	if len([]rune(msg)) > 20 {
		return false
	}
	// Must contain at least one symptom word.
	hasSymptom := false
	for _, kw := range []string{
		"疼", "痛", "痒", "晕", "恶心", "呕吐", "腹泻", "拉肚子", "发烧", "发热",
		"咳嗽", "头痛", "头晕", "胸闷", "气短", "乏力", "疲劳", "失眠", "皮疹",
		"便秘", "便血", "水肿", "黄疸", "鼻塞", "流涕", "咽痛", "尿频", "尿急",
		"不舒服", "难受", "症状",
	} {
		if strings.Contains(msg, kw) {
			hasSymptom = true
			break
		}
	}
	if !hasSymptom {
		return false
	}
	// Detail markers: if any present, the user has provided enough context.
	for _, marker := range []string{
		"天", "小时", "周", "月", "年", "持续", "一直", "反复", "突然", "昨天",
		"今天", "早上", "晚上", "伴随", "还有", "同时", "伴有", "因为", "由于",
		"吃了", "喝了", "用了", "检查", "化验", "医院", "医生", "诊断",
		"岁", "男", "女", "怀孕", "哺乳", "孩子", "宝宝", "老人",
	} {
		if strings.Contains(msg, marker) {
			return false
		}
	}
	return true
}

// buildContextualQuery enriches the current user message with recent
// conversation context for retrieval. Short follow-up messages ("那怎么办",
// "为什么") often refer to a disease/drug discussed in the previous turn;
// prepending the previous user turn's key terms prevents the retriever from
// matching irrelevant entries. Only the immediately preceding user turn is
// used (older turns are unlikely to be the referent), and the original
// userMessage is preserved for the LLM conversation — only the retrieval
// query is enriched.
func (a *Agent) buildContextualQuery(sess *session.Session, userMessage string) string {
	if sess == nil {
		return userMessage
	}
	// Short messages (<= 8 runes) are likely pronominal/follow-up questions.
	if len([]rune(userMessage)) > 8 {
		return userMessage
	}
	history := sess.GetMessages()
	// Find the most recent user message before the current turn.
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" && strings.TrimSpace(history[i].Content) != "" {
			prev := strings.TrimSpace(history[i].Content)
			if prev != userMessage && len([]rune(prev)) > 2 {
				return prev + " " + userMessage
			}
		}
	}
	return userMessage
}

// retrieveWithUnderstanding retrieves knowledge for a user message. The
// verbatim query runs first, and the LLM understanding step is on-demand:
// it only fans out when verbatim recall is weak (fewer than topK relevant
// hits). Each keyword/alias/vector layer already resolves ordinary colloquial
// phrasing without an LLM; the understanding model earns its 2-5s latency on
// queries those layers under-recall — long rambling or heavily ambiguous
// descriptions ("拉肚子" = 感染性腹泻 or 乳糖不耐受 or 秋季腹泻…) where one
// colloquialism fans out to all plausible standard concepts — enumerating,
// not disambiguating; ranking and the generation layer resolve ambiguity
// later with full conversational context. Any failure of the understanding
// step degrades silently to verbatim-only retrieval.
//
// When a reranker is configured, every exit passes through it once: the
// whole fused pool (over-fetched to 2×topK) is rescored by the
// cross-encoder and truncated to topK — the post-RRF precision pass.
func (a *Agent) retrieveWithUnderstanding(ctx context.Context, userMessage string, step func(StepEvent)) []knowledge.RetrievalResult {
	topK := a.cfg.KnowledgeTopK
	if topK <= 0 {
		topK = 5
	}
	// When rerank is active, over-fetch from the retriever legs so the
	// cross-encoder has a real pool to promote from — RRF truncation alone
	// would have already dropped borderline entries.
	fetchK := topK
	if a.reranker != nil {
		fetchK = 2 * topK
	}
	finalize := func(pool []knowledge.RetrievalResult) []knowledge.RetrievalResult {
		return knowledge.RerankCandidates(ctx, a.reranker, userMessage, pool, topK)
	}
	base, err := a.retriever.Retrieve(ctx, userMessage, fetchK)
	if err != nil {
		slog.Warn("Knowledge retrieval failed", "error", err)
		base = nil
	}
	if !a.cfg.QueryUnderstandingEnabled {
		return finalize(base)
	}
	// A full page of hits means the verbatim query already recalled enough
	// relevant material; a second LLM round-trip before the first token
	// would only add marginal recall. Skip it.
	if len(base) >= topK {
		slog.Debug("Query understanding skipped: verbatim recall sufficient", "hits", len(base))
		return finalize(base)
	}

	understood := a.understandQuery(ctx, userMessage)
	if understood == nil {
		return finalize(base)
	}

	queries := understood.SearchQueries
	if len(queries) == 0 && len(understood.SuspectedConditions) > 0 {
		// Prompt asks for search_queries; build them from conditions as a
		// fallback when the model omitted that field.
		joined := strings.Join(understood.Symptoms, " ")
		for _, c := range understood.SuspectedConditions {
			queries = append(queries, strings.TrimSpace(c+" "+joined))
		}
	}
	maxBranches := a.cfg.QueryUnderstandingBranches
	if maxBranches <= 0 {
		maxBranches = 5
	}
	if len(queries) > maxBranches {
		queries = queries[:maxBranches]
	}
	if len(queries) == 0 {
		return finalize(base)
	}

	// Collapse the branch queries' embedding round-trips into one batch
	// call: without this every branch's vector leg would hit the embedding
	// service with its own request (N round-trips, N tokenizer passes on a
	// shared local service) before any Qdrant query can start.
	if pw, ok := a.retriever.(knowledge.QueryPrewarmer); ok {
		pw.PrewarmQueries(queries)
	}

	step(StepEvent{Type: "retrieve", Summary: fmt.Sprintf("口语解析出 %d 条检索式，多路并行检索中", len(queries))})

	type branchResult struct {
		results []knowledge.RetrievalResult
	}
	branches := make(chan branchResult, len(queries))
	for _, q := range queries {
		go func(q string) {
			res, err := a.retriever.Retrieve(ctx, q, fetchK)
			if err != nil {
				slog.Warn("Understanding-branch retrieval failed", "query", q, "error", err)
			}
			branches <- branchResult{results: res}
		}(q)
	}
	paths := make([][]knowledge.RetrievalResult, 0, len(queries))
	for range queries {
		if b := <-branches; len(b.results) > 0 {
			paths = append(paths, b.results)
		}
	}

	merged := mergeRetrievalBranches(base, paths, topK)
	slog.Debug("Knowledge retrieved", "count", len(merged), "branches", len(paths))
	return finalize(merged)
}

// understandQuery runs the LLM understanding step and parses its JSON
// output. Returns nil on any failure — callers fall back to verbatim-only
// retrieval, so an unavailable understanding model must never break search.
func (a *Agent) understandQuery(ctx context.Context, userMessage string) *queryUnderstanding {
	// 8s cap: QU now runs on-demand (only when verbatim recall is thin), so
	// this timeout sits squarely on the first-token path — a slow provider
	// must not stall the answer. On timeout the call degrades silently to
	// verbatim-only retrieval, so a short cap costs recall, never correctness.
	uctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	resp, err := a.understandProvider.Chat(uctx,
		[]llm.Message{{Role: "user", Content: userMessage}},
		nil, prompt.QueryUnderstandingSystem)
	if err != nil {
		slog.Warn("Query understanding failed; using verbatim retrieval only", "error", err)
		return nil
	}
	raw := extractJSONObject(resp.Text)
	if raw == "" {
		slog.Warn("Query understanding produced no JSON object; skipping branches")
		return nil
	}
	var u queryUnderstanding
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		slog.Warn("Query understanding JSON parse failed; skipping branches", "error", err)
		return nil
	}
	return &u
}

// extractJSONObject pulls the outermost {...} span out of an LLM response,
// tolerating markdown fences or stray prose (some providers emit them even
// when told not to).
func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

// mergeRetrievalBranches fuses understanding-branch results with a small RRF
// pass (earlier branches weigh slightly more, mirroring the LLM's confidence
// order), then appends branch-only entries after the verbatim-path results.
// Total is capped at 2×topK so the prompt gains the extra recall without
// drowning in citations.
func mergeRetrievalBranches(base []knowledge.RetrievalResult, paths [][]knowledge.RetrievalResult, topK int) []knowledge.RetrievalResult {
	const (
		k            = 60.0 // RRF constant (Cormack et al., SIGIR 2009)
		branchWeight = 0.5  // branches rank below verbatim-path hits
	)

	scores := make(map[string]float64)
	entries := make(map[string]knowledge.KnowledgeEntry)
	for i, path := range paths {
		w := branchWeight * float64(len(paths)-i) / float64(len(paths))
		for rank, r := range path {
			id := r.Entry.ID
			if _, ok := entries[id]; !ok {
				entries[id] = r.Entry
			}
			scores[id] += w / (k + float64(rank+1))
		}
	}

	out := make([]knowledge.RetrievalResult, 0, len(base)+len(scores))
	seen := make(map[string]bool, len(base)+len(scores))
	for _, r := range base {
		if !seen[r.Entry.ID] {
			seen[r.Entry.ID] = true
			out = append(out, r)
		}
	}

	ids := make([]string, 0, len(scores))
	for id := range scores {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return scores[ids[i]] > scores[ids[j]] })
	for _, id := range ids {
		if len(out) >= 2*topK {
			break
		}
		out = append(out, knowledge.RetrievalResult{Entry: entries[id], Score: scores[id]})
	}
	return out
}

func isTransientLLMError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"429", "rate limit", "1305", "502", "503", "504", "timeout", "connection reset", "eof"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// sessionLock returns the per-session turn lock, creating it on first use.
// Holding it for the duration of a turn serializes concurrent requests that
// share a session ID, preventing message interleaving and the DisclaimerSent
// data race in ProcessMessageStream.
func (a *Agent) sessionLock(id string) *sync.Mutex {
	m, _ := a.sessLocks.LoadOrStore(id, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// toolOutcome pairs an executed tool call with its successful result so the
// follow-up retrieval step can mine entity names from it.
type toolOutcome struct {
	tc     llm.ToolCall
	result *tools.ToolResult
}

// maxParallelToolExec bounds concurrent tool executions within one batch.
// Tools mostly read mutex-guarded in-memory stores, so the cap only guards
// against a provider emitting an unusually large tool-call list.
const maxParallelToolExec = 4

// executeToolBatch runs one assistant tool-call batch. Dedupe and budget
// bookkeeping happen serially (first-wins), the surviving Dispatch calls run
// in parallel, and the tool-role messages are reassembled in the original
// call order — OpenAI-compatible endpoints reject tool_calls not followed by
// matching tool messages in order, and Anthropic expects the equivalent
// tool_result blocks. Step events stay serial: the SSE writer behind onStep
// is not goroutine-safe.
func (a *Agent) executeToolBatch(ctx context.Context, calls []llm.ToolCall,
	calledTools map[string]int, step func(StepEvent), conversationID string,
) (msgs []llm.Message, refs []tools.CitationRef, successful []toolOutcome, executed int) {
	msgs = make([]llm.Message, len(calls))
	type slot struct {
		idx int
		tc  llm.ToolCall
	}
	toRun := make([]slot, 0, len(calls))
	for idx, tc := range calls {
		// Duplicate detection: skip if same tool + same params
		// was already called in this turn.
		dedupeKey := tc.Name + ":" + tools.ParamsHash(tc.Name, tc.Arguments)
		if calledTools[dedupeKey] >= 1 {
			slog.Warn("Duplicate tool call detected, skipping",
				"tool", tc.Name, "conversation_id", conversationID)
			step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具「%s」重复调用已拦截", tc.Name)})
			msgs[idx] = llm.Message{
				Role: "tool", ToolCallID: tc.ID,
				Content: fmt.Sprintf("[工具 %s 已用相同参数调用过，请勿重复调用。请基于已有结果给出回答。]", tc.Name),
			}
			continue
		}
		calledTools[dedupeKey]++
		executed++
		slog.Info("Tool use requested", "tool", tc.Name, "id", tc.ID)
		step(StepEvent{Type: "tool_call", Tool: tc.Name, Summary: fmt.Sprintf("调用工具「%s」", tc.Name)})
		toRun = append(toRun, slot{idx: idx, tc: tc})
	}

	results := make([]*tools.ToolResult, len(calls))
	errs := make([]error, len(calls))
	sem := make(chan struct{}, maxParallelToolExec)
	var wg sync.WaitGroup
	for _, s := range toRun {
		wg.Add(1)
		go func(s slot) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[s.idx], errs[s.idx] = a.registry.Dispatch(ctx, s.tc.Name, s.tc.Arguments)
		}(s)
	}
	wg.Wait()

	for _, s := range toRun {
		tc := s.tc
		result, err := results[s.idx], errs[s.idx]
		var content string
		switch {
		case err != nil:
			content = fmt.Sprintf("[工具 %s 执行错误: %v]", tc.Name, err)
			step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具「%s」执行出错：%v", tc.Name, err)})
		case !result.Success:
			content = fmt.Sprintf("[工具 %s 返回错误: %s]", tc.Name, result.Error)
			step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具「%s」返回错误：%s", tc.Name, result.Error)})
		default:
			content = compactToolResult(tc.Name, result.Data)
			refs = append(refs, result.Citations...)
			successful = append(successful, toolOutcome{tc: tc, result: result})
			step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具「%s」返回结果（%d 条引用）", tc.Name, len(result.Citations))})
		}
		msgs[s.idx] = llm.Message{Role: "tool", ToolCallID: tc.ID, Content: content}
	}
	return msgs, refs, successful, executed
}

// ProcessMessageStream handles a single user message within a conversation
// session, forwarding every generated text chunk to onDelta (may be nil) as it
// is produced, and every pipeline step to onStep (may be nil). The final text
// is still returned in Response.Text; callers that render onDelta should
// prefer the returned text (post-verification may adjust the final response,
// in which case a small trailing difference is possible).
func (a *Agent) ProcessMessageStream(ctx context.Context, sess *session.Session, userMessage string, onDelta func(string), onStep func(StepEvent)) (*Response, error) {
	step := func(ev StepEvent) {
		if onStep != nil {
			onStep(ev)
		}
	}

	// Serialize turns per session so concurrent requests for the same
	// conversation don't interleave messages or race on DisclaimerSent.
	lock := a.sessionLock(sess.ID)
	lock.Lock()
	defer lock.Unlock()

	// L1: Emergency detection
	if a.cfg.EmergencyEnabled {
		if emerg := a.emergencyDetector.Detect(userMessage); emerg != nil {
			slog.Warn("Emergency detected", "matched", emerg.Matched)
			step(StepEvent{Type: "emergency", Summary: "检测到紧急情况，直接给出急救响应"})
			return &Response{
				Text:           safety.RemoveReferralSentences(safety.EmergencyResponseZH(emerg)),
				IsEmergency:    true,
				DisclaimerSent: true,
			}, nil
		}
	}

	// L2: Scope guard
	if a.cfg.ScopeGuardEnabled {
		if scope := a.scopeGuard.Check(userMessage); !scope.InScope {
			slog.Info("Out-of-scope query rejected", "reason", scope.Reason)
			step(StepEvent{Type: "refuse", Summary: "该问题超出医学咨询范围，拒绝回答并引导"})
			return &Response{
				Text:           scope.Redirect,
				IsOutOfScope:   true,
				DisclaimerSent: true,
			}, nil
		}
	}

	// Knowledge retrieval
	var retrieved []knowledge.RetrievalResult
	if a.cfg.KnowledgeEnabled {
		retrievalQuery := a.buildContextualQuery(sess, userMessage)
		retrieved = a.retrieveWithUnderstanding(ctx, retrievalQuery, step)
		slog.Debug("Knowledge retrieved", "count", len(retrieved))
		if len(retrieved) > 0 {
			step(StepEvent{Type: "retrieve", Summary: fmt.Sprintf("检索知识库，命中 %d 条相关条目", len(retrieved))})
		} else {
			step(StepEvent{Type: "retrieve", Summary: "知识库未检索到相关条目，将如实告知并引导"})
		}
	}

	// Build system prompt: static layer prefix (cacheable) + dynamic sections.
	patientCtx := a.buildPatientContextString(sess)
	staticPrompt := a.composer.ComposeStaticPrefix()
	systemPrompt := staticPrompt + a.composer.ComposeDynamicSections(retrieved, patientCtx)

	// When retrieval found nothing, constrain the model to steer instead of
	// improvising medical content from its own memory (hallucination guard).
	if a.cfg.KnowledgeEnabled && len(retrieved) == 0 {
		systemPrompt += "\n\n" + prompt.NoKnowledgeGuidance
	}

	// Route: select only relevant tools based on query classification.
	// This reduces the LLM's decision space from 35 to <=10 tools.
	var selectedToolNames []string
	if a.router != nil {
		selectedToolNames = a.router.ClassifyKG(userMessage, a.store)
	}
	slog.Debug("Tool routing (KG-guided)", "query", userMessage, "selected", selectedToolNames)

	toolDescs := a.registry.GetToolDescriptionsByNames(selectedToolNames)
	if len(toolDescs) > 0 {
		systemPrompt += "\n" + a.composer.ComposeToolPrompt(toolDescs)
	}

	// Clarification guidance: when the user names only a symptom without
	// duration/severity/context, ask targeted follow-ups instead of jumping
	// to a differential diagnosis.
	if needsClarification(userMessage) {
		systemPrompt += "\n\n## 信息不足时的澄清指引\n\n用户的问题只提到了症状，缺少持续时间、严重程度、伴随症状等关键信息。请先提出 2-3 个有针对性的追问问题（例如：症状持续多久了？是持续性还是阵发性？有没有伴随其他不适？），帮助用户补充信息后再给出分析。不要直接给出诊断或治疗建议。"
		step(StepEvent{Type: "retrieve", Summary: "问题信息较简略，将先引导用户补充关键细节"})
	}

	// Build messages in provider-agnostic format
	messages := a.sessionToMessages(sess)
	messages = append(messages, llm.Message{Role: "user", Content: userMessage})

	toolDefs := a.registry.GetGenericToolDefinitionsByNames(selectedToolNames)

	// Agent loop: call LLM, handle tool use, repeat until final response
	maxIterations := a.cfg.MaxToolIterations
	if maxIterations <= 0 {
		maxIterations = 5
	}
	var toolRefs []tools.CitationRef // tool-returned sources for post-verification
	var totalUsage llm.TokenUsage    // accumulated token usage across all LLM calls

	// Duplicate tool call detection and tool call budget.
	calledTools := make(map[string]int) // "toolName:paramsHash" -> count
	toolCallCount := 0
	maxToolCalls := a.cfg.MaxToolCalls
	if maxToolCalls <= 0 {
		maxToolCalls = 5
	}
	toolBudgetExceeded := false
	for i := 0; i < maxIterations; i++ {
		if i == 0 {
			step(StepEvent{Type: "generate", Summary: "正在思考…"})
		} else {
			step(StepEvent{Type: "generate", Summary: "正在根据工具结果组织回答…"})
		}

		// Last iteration or tool budget exceeded: strip tools so the
		// LLM must produce a text answer instead of calling more tools.
		iterTools := toolDefs
		iterPrompt := systemPrompt
		if i == maxIterations-1 || toolBudgetExceeded {
			iterTools = nil
			iterPrompt = systemPrompt + "\n\n你已经调用了多次工具。请基于已获取的工具返回信息，给出最终的完整回答，不要再调用任何工具。"
		}

		resp, err := a.streamWithRetry(ctx, messages, iterTools, staticPrompt, iterPrompt, onDelta)
		if err != nil {
			// Context cancellation (client disconnected, request timeout) is
			// not a system error — log at WARN and return a distinguishable
			// error so the server can skip the generic "internal error" event.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				slog.Warn("LLM stream aborted: context canceled or deadline exceeded",
					"error", err,
					"conversation_id", sess.ID,
					"iteration", i,
					"max_iterations", maxIterations,
				)
				return nil, fmt.Errorf("context canceled: %w", err)
			}
			slog.Error("LLM StreamChat failed",
				"error", err,
				"conversation_id", sess.ID,
				"iteration", i,
				"max_iterations", maxIterations,
			)
			return nil, err
		}
		totalUsage = totalUsage.Add(resp.Usage)

		// Guard against empty LLM responses (no text, no tool calls). This
		// can happen when the model truncates output at zero content tokens
		// or a thinking-mode model emits only reasoning_content. Retry on
		// non-final iterations; on the final iteration produce a safe
		// fallback answer so the user never sees a blank response and the
		// session history is never polluted with an empty assistant message
		// (which would cause HTTP 400 on the next turn).
		if resp.Text == "" && len(resp.ToolCalls) == 0 {
			slog.Warn("LLM returned empty response (no text, no tool calls); retrying",
				"conversation_id", sess.ID, "iteration", i, "max_iterations", maxIterations)
			if i < maxIterations-1 && !toolBudgetExceeded {
				continue
			}
			resp.Text = "抱歉，我刚才的思考没有产生有效回答。请您换一种方式描述问题，或补充更多细节（如症状持续时间、伴随症状、既往病史等），我会重新为您分析。"
		}

		// Check for tool calls
		if len(resp.ToolCalls) > 0 {
			// Ensure every tool call has a unique ID. Some OpenAI-compatible
			// providers (e.g. Zhipu glm in streaming mode) omit the tool_call
			// id field or send it only in the first delta chunk which may be
			// split across SSE lines. When id is empty, the tool result
			// message's tool_call_id is omitted (omitempty), causing the API
			// to report "insufficient tool messages following tool_calls".
			missingIDs := 0
			for idx := range resp.ToolCalls {
				if resp.ToolCalls[idx].ID == "" {
					missingIDs++
					resp.ToolCalls[idx].ID = fmt.Sprintf("call_%d_%d", i, idx)
				}
			}
			if missingIDs > 0 {
				slog.Warn("Tool call IDs were missing from LLM response, generated fallback IDs",
					"conversation_id", sess.ID,
					"iteration", i,
					"total_calls", len(resp.ToolCalls),
					"missing_ids", missingIDs,
				)
			}
			// Build assistant message with text + tool_calls + reasoning
			assistantMsg := llm.Message{
				Role:             "assistant",
				Content:          resp.Text,
				ReasoningContent: resp.ReasoningContent,
				ToolCalls:        resp.ToolCalls,
			}

			// Execute tools in parallel (results reassembled in call order —
			// see executeToolBatch).
			toolMsgs, batchRefs, successful, batchExecuted := a.executeToolBatch(ctx, resp.ToolCalls, calledTools, step, sess.ID)
			toolRefs = append(toolRefs, batchRefs...)
			toolCallCount += batchExecuted

			// Check tool budget after this batch of tool calls.
			if toolCallCount >= maxToolCalls {
				toolBudgetExceeded = true
			}

			messages = append(messages, assistantMsg)
			messages = append(messages, toolMsgs...)

			// Iterative retrieval: after tool results reveal concrete
			// entities (disease/drug names), automatically pull structured
			// knowledge about those entities so the final answer has
			// treatment/prevention/citations without an extra LLM round.
			for _, so := range successful {
				a.maybeFollowupRetrieve(ctx, so.tc, so.result, &retrieved, &messages)
			}
			continue
		}

		// No tool calls → final response
		responseText := resp.Text

		// L3: Post-generation verification
		if a.cfg.PostVerifyEnabled {
			step(StepEvent{Type: "verify", Summary: "正在校验回答的引用与安全性…"})
			// Map flat citation numbers [N] to their sources for verification.
			// Tool-returned literature (PMID/DOI) is registered alongside so
			// [PMID]-style references resolve instead of being flagged.
			sources := knowledge.BuildCitedSources(retrieved)
			for _, ref := range toolRefs {
				text := fmt.Sprintf("文献: %s", ref.Title)
				if ref.Year > 0 {
					text += fmt.Sprintf(" (%d)", ref.Year)
				}
				knowledge.AddToolSource(sources, ref.Title, ref.DOI, ref.PMID, ref.Year, ref.Level, text)
			}
			verifyResult := a.postVerifier.Verify(ctx, responseText, sources)
			if !verifyResult.Passed {
				slog.Warn("Response post-verification failed",
					"warnings", verifyResult.Warnings,
					"unsupported", verifyResult.UnsupportedClaims)
				// 核查结果仅记录日志，不再把"内容质量核查"块追加进回答，
				// 避免影响用户判断 (2026-09-08)。
				_ = verifyResult.CorrectedResponse
			}
		}

		// L3.5: Self-check answer completeness (rule-based, log-only).
		if missing := selfCheckAnswer(userMessage, responseText); len(missing) > 0 {
			slog.Warn("Answer self-check: possibly incomplete",
				"conversation_id", sess.ID, "missing", missing)
		}

		// L4: Disclaimer injection removed from answers (2026-09-06).
		disclaimerSent := false

		// Update session atomically: user + assistant together so a crash
		// between the two can never leave an orphaned user message (which
		// would cause two consecutive user messages on the next turn).
		sess.AddUserMessage(userMessage)
		sess.AddAssistantMessage(responseText)
		a.saveSession(sess)
		sess.TrimHistory(a.cfg.MaxHistoryTurns)

		return &Response{
			Text:           safety.RemoveReferralSentences(responseText),
			DisclaimerSent: disclaimerSent,
			Usage:          totalUsage,
			CostUSD:        llm.CostUSD(a.providerModel(), totalUsage),
			CostCNY:        llm.CostUSD(a.providerModel(), totalUsage) * llm.USDToCNY,
			Model:          a.providerModel(),
		}, nil
	}

	// Max iterations exceeded — force a final text response without tools
	// so the LLM must summarize what it found instead of erroring out.
	slog.Warn("Agent exceeded maximum tool-use iterations, forcing final response without tools",
		"conversation_id", sess.ID,
		"max_iterations", maxIterations,
	)
	step(StepEvent{Type: "generate", Summary: "正在根据已有信息组织最终回答…"})
	finalResp, err := a.streamWithRetry(ctx, messages, nil, staticPrompt,
		systemPrompt+"\n\n你已经调用了多次工具，请基于已获取的工具返回信息，给出最终的完整回答，不要再调用任何工具。",
		onDelta)
	if err != nil {
		slog.Error("Final LLM call after max iterations failed",
			"error", err,
			"conversation_id", sess.ID,
		)
		return nil, fmt.Errorf("LLM final response: %w", err)
	}
	responseText := finalResp.Text
	totalUsage = totalUsage.Add(finalResp.Usage)
	if responseText == "" {
		slog.Warn("Final LLM call after max iterations returned empty text; using fallback",
			"conversation_id", sess.ID)
		responseText = "抱歉，多次尝试后仍未能生成有效回答。建议您简化问题或分步骤咨询，我会尽力为您解答。"
	}

	// L3: Post-generation verification
	if a.cfg.PostVerifyEnabled {
		sources := knowledge.BuildCitedSources(retrieved)
		for _, ref := range toolRefs {
			text := fmt.Sprintf("文献: %s", ref.Title)
			if ref.Year > 0 {
				text += fmt.Sprintf(" (%d)", ref.Year)
			}
			knowledge.AddToolSource(sources, ref.Title, ref.DOI, ref.PMID, ref.Year, ref.Level, text)
		}
		verifyResult := a.postVerifier.Verify(ctx, responseText, sources)
		if !verifyResult.Passed {
			slog.Warn("Response post-verification failed",
				"warnings", verifyResult.Warnings,
				"unsupported", verifyResult.UnsupportedClaims)
			// 核查结果仅记录日志，不再把"内容质量核查"块追加进回答，
			// 避免影响用户判断 (2026-09-08)。
			_ = verifyResult.CorrectedResponse
		}
	}

	// L3.5: Self-check answer completeness.
	if missing := selfCheckAnswer(userMessage, responseText); len(missing) > 0 {
		slog.Warn("Answer self-check: possibly incomplete (max-iter path)",
			"conversation_id", sess.ID, "missing", missing)
	}

	// L4: Disclaimer injection removed from answers (2026-09-06).
	disclaimerSent := false

	// Update session atomically (user + assistant together).
	sess.AddUserMessage(userMessage)
	sess.AddAssistantMessage(responseText)
	a.saveSession(sess)
	sess.TrimHistory(a.cfg.MaxHistoryTurns)

	return &Response{
		Text:           safety.RemoveReferralSentences(responseText),
		DisclaimerSent: disclaimerSent,
		Usage:          totalUsage,
		CostUSD:        llm.CostUSD(a.providerModel(), totalUsage),
		CostCNY:        llm.CostUSD(a.providerModel(), totalUsage) * llm.USDToCNY,
		Model:          a.providerModel(),
	}, nil
}

// ProcessMessageStreamWithImages handles a user message with attached images.
func (a *Agent) ProcessMessageStreamWithImages(ctx context.Context, sess *session.Session, userMessage string, images []llm.ImageInput, onDelta func(string), onStep func(StepEvent)) (*Response, error) {
	step := func(ev StepEvent) {
		if onStep != nil {
			onStep(ev)
		}
	}

	// Serialize turns per session (same rationale as ProcessMessageStream).
	lock := a.sessionLock(sess.ID)
	lock.Lock()
	defer lock.Unlock()

	// L1: Emergency detection (skip for image messages - images may contain medical reports)
	if len(images) == 0 && a.cfg.EmergencyEnabled {
		if emerg := a.emergencyDetector.Detect(userMessage); emerg != nil {
			slog.Warn("Emergency detected", "matched", emerg.Matched)
			step(StepEvent{Type: "emergency", Summary: "检测到紧急情况，直接给出急救响应"})
			return &Response{
				Text:           safety.RemoveReferralSentences(safety.EmergencyResponseZH(emerg)),
				IsEmergency:    true,
				DisclaimerSent: true,
			}, nil
		}
	}

	// L2: Scope guard
	if a.cfg.ScopeGuardEnabled {
		if scope := a.scopeGuard.Check(userMessage); !scope.InScope {
			slog.Info("Out-of-scope query rejected", "reason", scope.Reason)
			step(StepEvent{Type: "refuse", Summary: "该问题超出医学咨询范围，拒绝回答并引导"})
			return &Response{
				Text:           scope.Redirect,
				IsOutOfScope:   true,
				DisclaimerSent: true,
			}, nil
		}
	}

	// Knowledge retrieval
	var retrieved []knowledge.RetrievalResult
	if a.cfg.KnowledgeEnabled {
		retrievalQuery := a.buildContextualQuery(sess, userMessage)
		retrieved = a.retrieveWithUnderstanding(ctx, retrievalQuery, step)
		slog.Debug("Knowledge retrieved", "count", len(retrieved), "has_images", len(images) > 0)
		if len(retrieved) > 0 {
			step(StepEvent{Type: "retrieve", Summary: fmt.Sprintf("检索知识库，命中 %d 条相关条目", len(retrieved))})
		} else {
			step(StepEvent{Type: "retrieve", Summary: "知识库未检索到相关条目，将如实告知并引导"})
		}
	}

	// Build system prompt: static layer prefix (cacheable) + dynamic sections.
	patientCtx := a.buildPatientContextString(sess)
	staticPrompt := a.composer.ComposeStaticPrefix()
	systemPrompt := staticPrompt + a.composer.ComposeDynamicSections(retrieved, patientCtx)

	// When retrieval found nothing, constrain the model to steer instead of
	// improvising medical content from its own memory (hallucination guard).
	if a.cfg.KnowledgeEnabled && len(retrieved) == 0 {
		systemPrompt += "\n\n" + prompt.NoKnowledgeGuidance
	}

	// Route: select only relevant tools via KG-guided classification.
	var selectedToolNames []string
	if a.router != nil {
		selectedToolNames = a.router.ClassifyKG(userMessage, a.store)
	}
	// When images are attached, always include the image analysis tool.
	if len(images) > 0 {
		hasImageTool := false
		for _, n := range selectedToolNames {
			if n == "medical_image_analyze" {
				hasImageTool = true
				break
			}
		}
		if !hasImageTool {
			selectedToolNames = append(selectedToolNames, "medical_image_analyze")
		}
	}
	slog.Debug("Tool routing", "query", userMessage, "selected", selectedToolNames, "has_images", len(images) > 0)

	toolDescs := a.registry.GetToolDescriptionsByNames(selectedToolNames)
	if len(toolDescs) > 0 {
		systemPrompt += "\n" + a.composer.ComposeToolPrompt(toolDescs)
	}

	// Clarification guidance (same logic as ProcessMessageStream).
	if len(images) == 0 && needsClarification(userMessage) {
		systemPrompt += "\n\n## 信息不足时的澄清指引\n\n用户的问题只提到了症状，缺少持续时间、严重程度、伴随症状等关键信息。请先提出 2-3 个有针对性的追问问题，帮助用户补充信息后再给出分析。不要直接给出诊断或治疗建议。"
		step(StepEvent{Type: "retrieve", Summary: "问题信息较简略，将先引导用户补充关键细节"})
	}

	// Build messages in provider-agnostic format with images
	messages := a.sessionToMessages(sess)

	// Create user message with images
	userMsg := llm.Message{Role: "user", Content: userMessage}
	if len(images) > 0 {
		// Add text part
		userMsg.Parts = append(userMsg.Parts, llm.ContentPart{
			Type: "text",
			Text: userMessage,
		})
		// Add image parts
		for _, img := range images {
			userMsg.Parts = append(userMsg.Parts, llm.ContentPart{
				Type:  "image",
				Image: &img,
			})
		}
		// Clear Content since we're using Parts
		userMsg.Content = ""
	}
	messages = append(messages, userMsg)

	toolDefs := a.registry.GetGenericToolDefinitionsByNames(selectedToolNames)

	// Agent loop: call LLM, handle tool use, repeat until final response
	maxIterations := a.cfg.MaxToolIterations
	if maxIterations <= 0 {
		maxIterations = 5
	}
	var toolRefs []tools.CitationRef // tool-returned sources for post-verification
	var totalUsage llm.TokenUsage    // accumulated token usage across all LLM calls

	// Duplicate tool call detection and tool call budget.
	calledTools := make(map[string]int) // "toolName:paramsHash" -> count
	toolCallCount := 0
	maxToolCalls := a.cfg.MaxToolCalls
	if maxToolCalls <= 0 {
		maxToolCalls = 5
	}
	toolBudgetExceeded := false
	for i := 0; i < maxIterations; i++ {
		if i == 0 {
			step(StepEvent{Type: "generate", Summary: "正在思考…"})
		} else {
			step(StepEvent{Type: "generate", Summary: "正在根据工具结果组织回答…"})
		}

		// Last iteration or tool budget exceeded: strip tools so the
		// LLM must produce a text answer instead of calling more tools.
		iterTools := toolDefs
		iterPrompt := systemPrompt
		if i == maxIterations-1 || toolBudgetExceeded {
			iterTools = nil
			iterPrompt = systemPrompt + "\n\n你已经调用了多次工具。请基于已获取的工具返回信息，给出最终的完整回答，不要再调用任何工具。"
		}

		var llmResp *llm.ChatResponse
		var llmErr error
		// Always route through streamWithRetry so transient errors (429,
		// 5xx, dropped connections) are retried regardless of whether the
		// caller wants streaming deltas. When onDelta is nil the provider
		// call runs in non-streaming mode internally.
		llmResp, llmErr = a.streamWithRetry(ctx, messages, iterTools, staticPrompt, iterPrompt, onDelta)
		if llmErr != nil {
			if errors.Is(llmErr, context.Canceled) || errors.Is(llmErr, context.DeadlineExceeded) {
				slog.Warn("LLM stream aborted: context canceled or deadline exceeded",
					"error", llmErr,
					"conversation_id", sess.ID,
					"iteration", i,
					"max_iterations", maxIterations,
					"has_images", len(images) > 0,
				)
				return nil, fmt.Errorf("context canceled: %w", llmErr)
			}
			slog.Error("LLM call failed",
				"error", llmErr,
				"conversation_id", sess.ID,
				"iteration", i,
				"max_iterations", maxIterations,
				"has_images", len(images) > 0,
			)
			return nil, fmt.Errorf("LLM call: %w", llmErr)
		}
		totalUsage = totalUsage.Add(llmResp.Usage)

		// Guard against empty LLM responses (same logic as
		// ProcessMessageStream — see comment there).
		if llmResp.Text == "" && len(llmResp.ToolCalls) == 0 {
			slog.Warn("LLM returned empty response (no text, no tool calls); retrying",
				"conversation_id", sess.ID, "iteration", i,
				"max_iterations", maxIterations, "has_images", len(images) > 0)
			if i < maxIterations-1 && !toolBudgetExceeded {
				continue
			}
			llmResp.Text = "抱歉，我刚才的思考没有产生有效回答。请您换一种方式描述问题，或补充更多细节（如症状持续时间、伴随症状、既往病史等），我会重新为您分析。"
		}

		// No tool calls → final answer
		if len(llmResp.ToolCalls) == 0 {
			responseText := llmResp.Text

			// L3: Citation post-verification
			if a.cfg.PostVerifyEnabled && a.postVerifier != nil {
				sources := knowledge.BuildCitedSources(retrieved)
				// Also register tool-returned citation refs
				for _, ref := range toolRefs {
					text := ref.Title
					if ref.DOI != "" {
						text += " DOI:" + ref.DOI
					}
					if ref.Year > 0 {
						text += fmt.Sprintf(" (%d)", ref.Year)
					}
					knowledge.AddToolSource(sources, ref.Title, ref.DOI, ref.PMID, ref.Year, ref.Level, text)
				}
				verifyResult := a.postVerifier.Verify(ctx, responseText, sources)
				if !verifyResult.Passed {
					slog.Warn("Response post-verification failed",
						"warnings", verifyResult.Warnings,
						"unsupported", verifyResult.UnsupportedClaims)
					// 与 ProcessMessageStream 一致：核查结果仅记录日志，
					// 不改动回答文本 (2026-09-08 决策)。
				}
			}

			// L3.5: Self-check answer completeness.
			if missing := selfCheckAnswer(userMessage, responseText); len(missing) > 0 {
				slog.Warn("Answer self-check: possibly incomplete (images path)",
					"conversation_id", sess.ID, "missing", missing)
			}

			// L4: Disclaimer injection removed from answers (2026-09-06).
			disclaimerSent := false

			// Update session atomically (user + assistant together).
			// NOTE: AddUserMessage was missing here previously — image
			// conversations never persisted the user turn, causing history
			// to contain only assistant messages on subsequent turns.
			sess.AddUserMessage(userMessage)
			sess.AddAssistantMessage(responseText)
			a.saveSession(sess)
			sess.TrimHistory(a.cfg.MaxHistoryTurns)

			return &Response{
				Text:           safety.RemoveReferralSentences(responseText),
				DisclaimerSent: disclaimerSent,
				Usage:          totalUsage,
				CostUSD:        llm.CostUSD(a.providerModel(), totalUsage),
				CostCNY:        llm.CostUSD(a.providerModel(), totalUsage) * llm.USDToCNY,
				Model:          a.providerModel(),
			}, nil
		}

		// Execute tool calls and build continuation messages
		// Ensure every tool call has a unique ID (same fix as
		// ProcessMessageStream — see comment there).
		missingIDs := 0
		for idx := range llmResp.ToolCalls {
			if llmResp.ToolCalls[idx].ID == "" {
				missingIDs++
				llmResp.ToolCalls[idx].ID = fmt.Sprintf("call_%d_%d", i, idx)
			}
		}
		if missingIDs > 0 {
			slog.Warn("Tool call IDs were missing from LLM response, generated fallback IDs",
				"conversation_id", sess.ID,
				"iteration", i,
				"total_calls", len(llmResp.ToolCalls),
				"missing_ids", missingIDs,
				"has_images", len(images) > 0,
			)
		}
		messages = append(messages, llm.Message{
			Role:             "assistant",
			Content:          llmResp.Text,
			ReasoningContent: llmResp.ReasoningContent,
			ToolCalls:        llmResp.ToolCalls,
		})

		// Execute tools in parallel (results reassembled in call order —
		// see executeToolBatch).
		toolMsgs, batchRefs, successful, batchExecuted := a.executeToolBatch(ctx, llmResp.ToolCalls, calledTools, step, sess.ID)
		toolRefs = append(toolRefs, batchRefs...)
		toolCallCount += batchExecuted

		// Check tool budget after this batch of tool calls.
		if toolCallCount >= maxToolCalls {
			toolBudgetExceeded = true
		}

		messages = append(messages, toolMsgs...)

		// Iterative retrieval: auto-pull structured knowledge for entities
		// revealed by tool results (same logic as ProcessMessageStream).
		for _, so := range successful {
			a.maybeFollowupRetrieve(ctx, so.tc, so.result, &retrieved, &messages)
		}
	}

	// Max iterations exceeded — force a final text response without tools
	// so the LLM must summarize what it found instead of erroring out.
	slog.Warn("Agent exceeded maximum tool-use iterations, forcing final response without tools",
		"conversation_id", sess.ID,
		"max_iterations", maxIterations,
		"has_images", len(images) > 0,
	)
	step(StepEvent{Type: "generate", Summary: "正在根据已有信息组织最终回答…"})
	finalResp, err := a.streamWithRetry(ctx, messages, nil, staticPrompt,
		systemPrompt+"\n\n你已经调用了多次工具，请基于已获取的工具返回信息，给出最终的完整回答，不要再调用任何工具。",
		onDelta)
	if err != nil {
		slog.Error("Final LLM call after max iterations failed",
			"error", err,
			"conversation_id", sess.ID,
			"has_images", len(images) > 0,
		)
		return nil, fmt.Errorf("LLM final response: %w", err)
	}
	responseText := finalResp.Text
	totalUsage = totalUsage.Add(finalResp.Usage)
	if responseText == "" {
		slog.Warn("Final LLM call after max iterations returned empty text; using fallback",
			"conversation_id", sess.ID, "has_images", len(images) > 0)
		responseText = "抱歉，多次尝试后仍未能生成有效回答。建议您简化问题或分步骤咨询，我会尽力为您解答。"
	}

	// L3: Citation post-verification
	if a.cfg.PostVerifyEnabled && a.postVerifier != nil {
		sources := knowledge.BuildCitedSources(retrieved)
		for _, ref := range toolRefs {
			text := ref.Title
			if ref.DOI != "" {
				text += " DOI:" + ref.DOI
			}
			if ref.Year > 0 {
				text += fmt.Sprintf(" (%d)", ref.Year)
			}
			knowledge.AddToolSource(sources, ref.Title, ref.DOI, ref.PMID, ref.Year, ref.Level, text)
		}
		verifyResult := a.postVerifier.Verify(ctx, responseText, sources)
		if !verifyResult.Passed {
			slog.Warn("Response post-verification failed",
				"warnings", verifyResult.Warnings,
				"unsupported", verifyResult.UnsupportedClaims)
			// 与 ProcessMessageStream 一致：核查结果仅记录日志，
			// 不改动回答文本 (2026-09-08 决策)。
		}
	}

	// L3.5: Self-check answer completeness.
	if missing := selfCheckAnswer(userMessage, responseText); len(missing) > 0 {
		slog.Warn("Answer self-check: possibly incomplete (images max-iter path)",
			"conversation_id", sess.ID, "missing", missing)
	}

	// L4: Disclaimer injection removed from answers (2026-09-06).
	disclaimerSent := false

	// Update session atomically (user + assistant together).
	sess.AddUserMessage(userMessage)
	sess.AddAssistantMessage(responseText)
	a.saveSession(sess)
	sess.TrimHistory(a.cfg.MaxHistoryTurns)

	return &Response{
		Text:           safety.RemoveReferralSentences(responseText),
		DisclaimerSent: disclaimerSent,
		Usage:          totalUsage,
		CostUSD:        llm.CostUSD(a.providerModel(), totalUsage),
		CostCNY:        llm.CostUSD(a.providerModel(), totalUsage) * llm.USDToCNY,
		Model:          a.providerModel(),
	}, nil
}

// toolResultHardCap bounds the byte length of a serialized tool result
// injected into conversation history.
const toolResultHardCap = 4000

// compressionLimits is one rung of the generic field-aware compaction
// ladder: max container items, max string runes, max recursion depth.
type compressionLimits struct {
	maxItems int
	maxStr   int
	maxDepth int
}

// compactionLadder tries progressively tighter compressions; the first
// rung whose output fits toolResultHardCap wins, so payloads keep as much
// detail as the budget allows instead of being cut at an arbitrary byte.
var compactionLadder = []compressionLimits{
	{maxItems: 6, maxStr: 240, maxDepth: 4},
	{maxItems: 3, maxStr: 120, maxDepth: 3},
	{maxItems: 2, maxStr: 60, maxDepth: 2},
	{maxItems: 2, maxStr: 40, maxDepth: 1},
}

// compactToolResult reduces the size of a tool result before it is injected
// into the conversation history. Large JSON payloads (disease_encyclopedia
// returns 24 fields; knowledge_search can return 10 full entries) quickly
// consume the context window and crowd out the actual answer. Compaction is
// field-aware and always emits valid JSON: a knowledge_search-specific
// pruning policy first, then a generic ladder that caps string lengths,
// list lengths and nesting depth, and finally a top-level field summary if
// even the tightest rung does not fit. The old behavior of slicing the
// serialized bytes at a hard cap is gone — it could cut mid-JSON and split
// multibyte CJK characters. data is never mutated (maybeFollowupRetrieve
// consumes it after this call).
func compactToolResult(toolName string, data map[string]any) string {
	raw, _ := json.MarshalIndent(data, "", "  ")
	if len(raw) <= toolResultHardCap {
		return string(raw)
	}
	// Normalize through a JSON round-trip: tools store typed slices
	// ([]map[string]any, []Citation, []string) under "results", which a Go
	// []any type switch cannot see — the old knowledge_search policy was
	// silently dead for real payloads because of this. After unmarshal every
	// container is map[string]any/[]any, and the caller's map stays intact.
	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return scalarSummary(data)
	}

	base := normalized
	// Dataset-aware pruning for knowledge_search: keep top 3 results with
	// only the fields the LLM actually needs to answer.
	if toolName == "knowledge_search" {
		if pruned, ok := pruneKnowledgeSearch(normalized); ok {
			out, err := json.MarshalIndent(pruned, "", "  ")
			if err == nil && len(out) <= toolResultHardCap {
				return string(out)
			}
			base = pruned
		}
	}
	for _, lim := range compactionLadder {
		if s, ok := compressToSize(base, lim); ok {
			return s
		}
	}
	return scalarSummary(data)
}

// pruneKnowledgeSearch applies the top-3 + field-whitelist policy to a
// normalized knowledge_search payload. ok=false when it does not apply
// (different shape, or ≤3 results already).
func pruneKnowledgeSearch(data map[string]any) (map[string]any, bool) {
	results, ok := data["results"].([]any)
	if !ok || len(results) <= 3 {
		return nil, false
	}
	pruned := make(map[string]any, len(data))
	for k, v := range data {
		if k != "results" {
			pruned[k] = v
		}
	}
	kept := make([]any, 0, 3)
	for _, item := range results[:3] {
		if m, ok := item.(map[string]any); ok {
			thin := make(map[string]any)
			for _, key := range []string{"condition_zh", "name_zh", "title", "treatment", "prevention", "risk_factors", "complications", "citations", "relevance"} {
				if v, ok := m[key]; ok {
					thin[key] = v
				}
			}
			kept = append(kept, thin)
		} else {
			kept = append(kept, item)
		}
	}
	pruned["results"] = kept
	pruned["_note"] = fmt.Sprintf("结果已压缩：原 %d 条保留前 3 条并精简字段", len(results))
	return pruned, true
}

// compressToSize runs one ladder rung and accepts it only if the result
// fits the budget.
func compressToSize(v any, lim compressionLimits) (string, bool) {
	out, err := json.MarshalIndent(compressValue(v, lim, 0), "", "  ")
	if err != nil || len(out) > toolResultHardCap {
		return "", false
	}
	return string(out), true
}

// compressValue recursively caps strings to lim.maxStr runes, lists to
// lim.maxItems entries (with an omission note), and stops recursing at
// lim.maxDepth — values deeper than that are serialized and rune-capped as
// a single string, which bounds both size and stack depth. Always returns
// JSON-safe values.
func compressValue(v any, lim compressionLimits, depth int) any {
	switch t := v.(type) {
	case string:
		return truncateRunes(t, lim.maxStr)
	case []any:
		if depth >= lim.maxDepth {
			return truncateRunes(jsonLike(t), lim.maxStr)
		}
		if len(t) <= lim.maxItems {
			kept := make([]any, len(t))
			for i, item := range t {
				kept[i] = compressValue(item, lim, depth+1)
			}
			return kept
		}
		kept := make([]any, 0, lim.maxItems+1)
		for _, item := range t[:lim.maxItems] {
			kept = append(kept, compressValue(item, lim, depth+1))
		}
		kept = append(kept, fmt.Sprintf("（其余 %d 项已省略）", len(t)-lim.maxItems))
		return kept
	case map[string]any:
		if depth >= lim.maxDepth {
			return truncateRunes(jsonLike(t), lim.maxStr)
		}
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = compressValue(val, lim, depth+1)
		}
		return out
	default:
		return v
	}
}

// scalarSummary is the last-resort tier: a flat overview of the top-level
// fields. It is always valid JSON and always small.
func scalarSummary(data map[string]any) string {
	sum := make(map[string]any, len(data)+1)
	for k, v := range data {
		switch t := v.(type) {
		case string:
			sum[k] = truncateRunes(t, 80)
		case []any:
			sum[k] = fmt.Sprintf("（%d 项，内容已省略）", len(t))
		case map[string]any:
			sum[k] = fmt.Sprintf("（对象，%d 个字段，内容已省略）", len(t))
		default:
			b, err := json.Marshal(v)
			if err != nil || len(b) > 120 {
				sum[k] = truncateRunes(fmt.Sprintf("%v", v), 80)
			} else {
				sum[k] = v
			}
		}
	}
	sum["_note"] = "结果过大，仅保留字段概览；如需完整内容可重新调用该工具"
	out, err := json.MarshalIndent(sum, "", "  ")
	if err != nil || len(out) > toolResultHardCap {
		return `{"_note":"结果过大，内容已省略；可重新调用该工具获取完整数据"}`
	}
	return string(out)
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func jsonLike(v any) string {
	if b, err := json.Marshal(v); err == nil {
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}

// maybeFollowupRetrieve performs an automatic secondary retrieval after a
// tool call returns. When the tool result reveals a concrete entity (disease
// name, drug name), that entity is used as a fresh retrieval query so the
// agent gains structured knowledge (treatment, prevention, complications)
// about the entity without the LLM having to explicitly call knowledge_search
// again. Results are injected as an extra context message and merged into the
// retrieved slice for citation post-verification.
//
// This closes the "retrieve → tool → answer" loop: e.g. user asks "一侧头痛
// 伴恶心", symptom_triage returns "偏头痛", then followup retrieval pulls
// the migraine KnowledgeEntry (treatment/prevention/citations) into context.
func (a *Agent) maybeFollowupRetrieve(ctx context.Context, tc llm.ToolCall, result *tools.ToolResult, retrieved *[]knowledge.RetrievalResult, messages *[]llm.Message) {
	if result == nil || !result.Success || result.Data == nil {
		return
	}
	entities := extractEntitiesFromToolResult(tc.Name, tc.Arguments, result.Data)
	if len(entities) == 0 {
		return
	}
	// Cap followup queries to avoid runaway retrieval in one turn.
	if len(entities) > 2 {
		entities = entities[:2]
	}
	var entityQueries []string
	for _, ent := range entities {
		entityQueries = append(entityQueries, ent+" 治疗 预防 并发症")
	}
	if pw, ok := a.retriever.(knowledge.QueryPrewarmer); ok {
		pw.PrewarmQueries(entityQueries)
	}
	var added []knowledge.RetrievalResult
	for _, q := range entityQueries {
		res, err := a.retriever.Retrieve(ctx, q, 3)
		if err != nil {
			slog.Debug("Followup retrieval failed", "query", q, "error", err)
			continue
		}
		added = append(added, res...)
	}
	if len(added) == 0 {
		return
	}
	// De-dup against already-retrieved entries.
	seen := make(map[string]bool, len(*retrieved))
	for _, r := range *retrieved {
		seen[r.Entry.ID] = true
	}
	var fresh []knowledge.RetrievalResult
	for _, r := range added {
		if !seen[r.Entry.ID] {
			seen[r.Entry.ID] = true
			fresh = append(fresh, r)
		}
	}
	if len(fresh) == 0 {
		return
	}
	// Number the supplement [K+1..] continuing the merged list (initial
	// retrieval occupies [1..K]). Restarting at [1] would make the same
	// number point to two different sources: the model would cite the
	// supplement as [1] while post-verification resolves [1] against the
	// merged BuildCitedSources map, i.e. the wrong paper.
	offset := knowledge.FlatCitationCount(*retrieved)
	*retrieved = append(*retrieved, fresh...)
	// Format as a compact context injection message.
	var sb strings.Builder
	sb.WriteString("【补充检索结果】根据工具返回的实体，自动检索到以下循证医学知识，请在回答中参考：\n\n")
	formatter := knowledge.NewCitationFormatter()
	sb.WriteString(formatter.BuildCitationMapOffset(fresh, offset))
	*messages = append(*messages, llm.Message{Role: "user", Content: sb.String()})
	slog.Debug("Followup retrieval injected", "entities", entities, "fresh_entries", len(fresh))
}

// extractEntitiesFromToolResult pulls concrete medical entities (disease
// names, drug names) from a tool call's arguments and result data. It handles
// the unified tools' common output shapes: knowledge_search returns
// condition_zh / results[].condition_zh; exact_lookup returns name_zh /
// disease; symptom_triage returns likely_conditions; medical_kg_lookup
// returns head/tail disease nodes.
func extractEntitiesFromToolResult(toolName string, args map[string]any, data map[string]any) []string {
	var entities []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || len([]rune(s)) < 2 {
			return
		}
		for _, e := range entities {
			if e == s {
				return
			}
		}
		entities = append(entities, s)
	}
	// 1. From arguments: knowledge_search query is itself a retrieval-worthy term.
	if q, ok := args["query"].(string); ok && toolName == "knowledge_search" {
		add(q)
	}
	// 2. From result data: common fields across unified tools.
	for _, key := range []string{"condition_zh", "name_zh", "disease", "condition", "drug_name", "generic_name_zh"} {
		if v, ok := data[key].(string); ok {
			add(v)
		}
	}
	// 3. results[] array (knowledge_search / exact_lookup / disease_encyclopedia).
	if results, ok := data["results"].([]any); ok {
		for i, item := range results {
			if i >= 3 {
				break
			}
			if m, ok := item.(map[string]any); ok {
				for _, key := range []string{"condition_zh", "name_zh", "name", "disease", "title"} {
					if v, ok := m[key].(string); ok {
						add(v)
					}
				}
			}
		}
	}
	// 4. symptom_triage likely_conditions.
	if conds, ok := data["likely_conditions"].([]any); ok {
		for _, c := range conds {
			if s, ok := c.(string); ok {
				add(s)
			}
		}
	}
	// 5. medical_kg_lookup head/tail disease nodes.
	for _, key := range []string{"head", "tail"} {
		if v, ok := data[key].(string); ok {
			add(v)
		}
	}
	return entities
}

// sessionToMessages returns the session history in provider-agnostic form.
// Sessions now store llm.Message directly, so no conversion is needed.
//
// Historical messages are sanitised before being fed back to the LLM:
// persisted history must only carry Role + Content. Runtime-only fields
// (ReasoningContent, ToolCalls, ToolCallID, Parts) must never survive into
// the next turn — they are provider-specific (e.g. DeepSeek's
// reasoning_content) and can cause OpenAI-compatible endpoints to reject
// the request with HTTP 400 when the model is switched. Messages with
// unexpected roles (e.g. "tool" leaked into persisted history) are dropped
// entirely: a tool message without its preceding assistant tool_call is
// always invalid.
func (a *Agent) sessionToMessages(sess *session.Session) []llm.Message {
	raw := sess.GetMessages()
	msgs := make([]llm.Message, 0, len(raw))
	// Track orphaned tool messages: when an empty assistant is dropped, the
	// tool messages that followed it (up to the next user/assistant) must
	// also be dropped — a tool message without its preceding assistant
	// tool_call is invalid for every provider.
	droppingToolMsgs := false
	for i, m := range raw {
		if m.Role != "user" && m.Role != "assistant" && m.Role != "tool" {
			slog.Warn("Dropping historical message with unexpected role",
				"session_id", sess.ID, "index", i, "role", m.Role)
			continue
		}
		// Drop empty assistant messages (no text, no tool calls) — they cause
		// HTTP 400 from OpenAI-compatible endpoints and are meaningless for
		// Anthropic too.
		if m.Role == "assistant" && strings.TrimSpace(m.Content) == "" && len(m.ToolCalls) == 0 {
			slog.Warn("Dropping empty assistant message from session history",
				"session_id", sess.ID, "index", i,
				"has_reasoning", m.ReasoningContent != "")
			droppingToolMsgs = true
			continue
		}
		if m.Role == "assistant" || m.Role == "user" {
			droppingToolMsgs = false
		}
		if droppingToolMsgs && m.Role == "tool" {
			slog.Warn("Dropping orphaned tool message after empty assistant",
				"session_id", sess.ID, "index", i)
			continue
		}
		if m.ReasoningContent != "" || len(m.ToolCalls) > 0 || m.ToolCallID != "" || len(m.Parts) > 0 {
			slog.Warn("Sanitising historical message: removing runtime-only fields before LLM request",
				"session_id", sess.ID,
				"index", i,
				"role", m.Role,
				"had_reasoning", m.ReasoningContent != "",
				"had_tool_calls", len(m.ToolCalls),
				"had_tool_call_id", m.ToolCallID != "",
				"had_parts", len(m.Parts))
		}
		m.ReasoningContent = ""
		m.ToolCalls = nil
		m.ToolCallID = ""
		m.Parts = nil
		msgs = append(msgs, m)
	}
	return msgs
}

// GetOrCreateSession returns an existing session or creates a new one.
// When a file store is configured, sessions not in memory are first restored
// from disk (so conversations survive restarts).
func (a *Agent) GetOrCreateSession(sessionID string) *session.Session {
	a.sessionsMu.RLock()
	sess, ok := a.sessions[sessionID]
	a.sessionsMu.RUnlock()
	if ok {
		return sess
	}

	// Try to restore from store before creating a fresh session.
	if a.sessionStore != nil {
		if restored, err := a.sessionStore.Load(sessionID); err != nil {
			slog.Warn("Failed to restore session", "id", sessionID, "error", err)
		} else if restored != nil {
			a.sessionsMu.Lock()
			// Re-check under the write lock: a concurrent request for the
			// same ID may already have restored/created it while we were
			// loading. Overwriting would hand two requests different
			// *Session objects for one conversation, so their histories
			// would silently diverge and clobber each other on save.
			if existing, ok := a.sessions[sessionID]; ok {
				a.sessionsMu.Unlock()
				return existing
			}
			a.sessions[sessionID] = restored
			a.lastSweep = a.reapLocked(a.lastSweep)
			a.sessionsMu.Unlock()
			return restored
		}
	}

	sess = session.New(sessionID)
	a.sessionsMu.Lock()
	if existing, ok := a.sessions[sessionID]; ok {
		a.sessionsMu.Unlock()
		return existing
	}
	a.sessions[sessionID] = sess
	a.lastSweep = a.reapLocked(a.lastSweep)
	a.sessionsMu.Unlock()
	return sess
}

// ClaimSession is GetOrCreateSession plus the ownership check every
// user-facing endpoint needs: it returns false when the conversation already
// belongs to somebody else, so a guessed conversation id can never be read or
// written by another account. An unowned (pre-login / anonymous) conversation
// is bound to owner here, which is what lets a chat started while logged out
// keep its history after the user signs in.
func (a *Agent) ClaimSession(sessionID, owner string) (*session.Session, bool) {
	sess := a.GetOrCreateSession(sessionID)
	if sess == nil || !sess.ClaimOwner(owner) {
		return nil, false
	}
	return sess, true
}

// sessionSweepInterval throttles the memory-bound scan: it walks every live
// session, so it runs at most once a minute from the request path rather than
// on every message. It is a var so tests can force a sweep.
var sessionSweepInterval = time.Minute

// reapLocked drops sessions that idle out (only when a store can bring them
// back) and then enforces the size ceiling, least-recently-touched first. The
// map used to be append-only: the web UI mints a fresh conversation id per
// visitor, so one long-lived server accumulated every conversation's full
// history until it was OOM-killed — worse for everyone than one user having to
// reopen a very old chat. Caller must hold sessionsMu for writing; it returns
// the timestamp to keep as the next sweep deadline.
func (a *Agent) reapLocked(last time.Time) time.Time {
	now := time.Now()
	if !last.IsZero() && now.Sub(last) < sessionSweepInterval {
		return last
	}
	type touched struct {
		id   string
		idle time.Duration
	}
	live := make([]touched, 0, len(a.sessions))
	for id, sess := range a.sessions {
		live = append(live, touched{id, now.Sub(sess.LastTouched())})
	}

	// Idle eviction first, and only when the conversation is restorable.
	if a.sessionStore != nil {
		for _, e := range live {
			if a.sessionIdleTTL > 0 && e.idle > a.sessionIdleTTL {
				a.evictLocked(e.id)
			}
		}
	}
	if a.maxSessions > 0 {
		sort.Slice(live, func(i, j int) bool { return live[i].idle > live[j].idle })
		for _, e := range live {
			if len(a.sessions) <= a.maxSessions {
				break
			}
			if _, ok := a.sessions[e.id]; !ok {
				continue
			}
			a.evictLocked(e.id) // refuses to evict a session mid-turn
		}
	}
	if dropped := len(live) - len(a.sessions); dropped > 0 {
		slog.Info("Session cache trimmed", "dropped", dropped, "live", len(a.sessions),
			"idle_ttl", a.sessionIdleTTL.String(), "cap", a.maxSessions)
	}
	return now
}

// evictLocked removes one session from the cache and its turn lock. It refuses
// to evict a session whose lock is held — a turn is in flight, and dropping it
// mid-response would have the next request restore a stale snapshot and clobber
// the reply on save.
func (a *Agent) evictLocked(id string) bool {
	if v, ok := a.sessLocks.Load(id); ok {
		mu := v.(*sync.Mutex)
		if !mu.TryLock() {
			return false
		}
		mu.Unlock()
	}
	delete(a.sessions, id)
	a.sessLocks.Delete(id)
	return true
}

// SetSessionStore enables session persistence (file or database) after
// construction. Used by the HTTP server to persist sessions to MariaDB even
// when no SESSION_DIR is configured.
func (a *Agent) SetSessionStore(store session.Store) {
	a.sessionStore = store
	if store != nil {
		slog.Info("Session persistence enabled", "type", "database")
	}
}

// DeleteSession removes a session from memory (and the configured store).
// Used by the HTTP layer when a conversation is deleted via the session API.
func (a *Agent) DeleteSession(sessionID string) {
	a.sessionsMu.Lock()
	delete(a.sessions, sessionID)
	a.sessionsMu.Unlock()
	// Drop the per-session turn lock too: entries are only ever added, so
	// without this cleanup attacker-controlled conversation IDs would grow
	// the map without bound.
	a.sessLocks.Delete(sessionID)
	if a.sessionStore != nil {
		if err := a.sessionStore.Delete(sessionID); err != nil {
			slog.Warn("Failed to delete session from store", "id", sessionID, "error", err)
		}
	}
}

// saveSession persists a session snapshot when a store is configured.
// Snapshotting happens after user/assistant messages are appended.
func (a *Agent) saveSession(sess *session.Session) {
	if a.sessionStore == nil {
		return
	}
	if err := a.sessionStore.Save(sess); err != nil {
		slog.Warn("Failed to persist session", "id", sess.ID, "error", err)
	}
}

func (a *Agent) buildPatientContextString(sess *session.Session) string {
	pc := sess.GetPatientContext()
	if pc == nil {
		return ""
	}
	return prompt.BuildPatientContext(&prompt.PatientContextSummary{
		Region:           pc.Region,
		G6PDStatus:       pc.G6PDStatus,
		ThalassemiaTrait: pc.ThalassemiaTrait,
		KnownConditions:  pc.KnownConditions,
		KnownAllergies:   pc.KnownAllergies,
		Medications:      pc.CurrentMedications,
		ProfileSummary:   pc.ProfileSummary,
	})
}

// Response wraps the agent's output to the user.
type Response struct {
	Text           string `json:"text"`
	IsEmergency    bool   `json:"is_emergency"`
	IsOutOfScope   bool   `json:"is_out_of_scope"`
	QualityWarning string `json:"quality_warning,omitempty"`
	DisclaimerSent bool   `json:"disclaimer_sent"`
	// Usage/Cost: token consumption and estimated cost of generating this
	// answer (summed across all LLM calls in the turn). Cost is 0 when the
	// model has no known list price.
	Usage   llm.TokenUsage `json:"usage,omitempty"`
	CostUSD float64        `json:"cost_usd,omitempty"`
	CostCNY float64        `json:"cost_cny,omitempty"`
	Model   string         `json:"model,omitempty"`
}

// providerModel returns the underlying model identifier when the provider
// exposes it (optional interface — fakes in tests don't).
func (a *Agent) providerModel() string {
	if nm, ok := a.provider.(interface{ Model() string }); ok {
		return nm.Model()
	}
	return ""
}
