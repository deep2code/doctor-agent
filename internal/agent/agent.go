package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/embedding"
	"github.com/doctor-agent/internal/knowledge"
	"github.com/doctor-agent/internal/llm"
	"github.com/doctor-agent/internal/prompt"
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
	composer  *prompt.Composer
	registry  *tools.Registry

	router            *tools.Router
	emergencyDetector *safety.EmergencyDetector
	scopeGuard        *safety.ScopeGuard
	disclaimerService *safety.DisclaimerService
	postVerifier      *safety.PostVerifier

	sessionsMu   sync.RWMutex
	sessions     map[string]*session.Session
	sessionStore session.Store // optional on-disk or database session store
	sessLocks    sync.Map      // sessionID -> *sync.Mutex, serializes turns per session
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
	composer := prompt.NewComposer()
	registry := tools.NewRegistry()
	router := tools.NewRouter()

	// Register unified tools (8 total: 6 action + 2 unified retrieval/lookup).
	// Action tools — computation / cross-reference, not replaceable by RAG.
	registry.Register(tools.NewDrugSafetyCheck(store))
	registry.Register(tools.NewGeneticRiskCalculator(store))
	registry.Register(tools.NewFoodRiskAnalyzer(store))
	registry.Register(tools.NewSymptomTriage(store))
	registry.Register(tools.NewDrugInteractionCheckTool(store))
	registry.Register(tools.NewMedicalImageAnalyze(provider))
	// Unified retrieval / lookup — replace ~28 retired specialized tools.
	registry.Register(tools.NewKnowledgeSearch(store, retriever))
	registry.Register(tools.NewExactLookup(store))

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
		cfg:               cfg,
		provider:          provider,
		store:             store,
		retriever:         retriever,
		composer:          composer,
		registry:          registry,
		router:            router,
		emergencyDetector: safety.NewEmergencyDetector(),
		scopeGuard:        safety.NewScopeGuard(),
		disclaimerService: safety.NewDisclaimerService(),
		postVerifier:      postVerifier,
		sessions:          make(map[string]*session.Session),
		sessionStore:      sessionStore,
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
			cfg.MaxTokens,
			cfg.Temperature,
		), nil
	case "openai-compat":
		return llm.NewOpenAICompatProvider(
			cfg.OpenAICompatBaseURL,
			cfg.OpenAICompatAPIKey,
			cfg.OpenAICompatModel,
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
		return llm.NewDeepSeekProvider(cfg.DeepSeekAPIKey, model, 2048, 0), nil
	case "openai-compat":
		model := cfg.JudgeModel
		if model == "" {
			model = cfg.OpenAICompatModel
		}
		return llm.NewOpenAICompatProvider(cfg.OpenAICompatBaseURL, cfg.OpenAICompatAPIKey, model, 2048, 0), nil
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

// streamWithRetry wraps provider.StreamChat with a short backoff retry for
// transient provider failures (HTTP 429 rate limits, 5xx, dropped
// connections). Retrying is skipped once any delta has already been emitted
// to the user, because a fresh stream would replay the partial answer.
func (a *Agent) streamWithRetry(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, systemPrompt string, onDelta func(string)) (*llm.ChatResponse, error) {
	var emitted bool
	wrapped := onDelta
	if onDelta != nil {
		wrapped = func(d string) {
			emitted = true
			onDelta(d)
		}
	}
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
		resp, err := a.provider.StreamChat(ctx, messages, tools, systemPrompt, wrapped)
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

// ProcessMessageStream handles a single user message within a conversation
// session, forwarding every generated text chunk to onDelta (may be nil) as it
// is produced, and every pipeline step to onStep (may be nil). The final text
// is still returned in Response.Text; callers that render onDelta should
// prefer the returned text (post-verification may adjust the final response,
// in which case a small trailing difference is possible).

// sessionLock returns the per-session turn lock, creating it on first use.
// Holding it for the duration of a turn serializes concurrent requests that
// share a session ID, preventing message interleaving and the DisclaimerSent
// data race in ProcessMessageStream.
func (a *Agent) sessionLock(id string) *sync.Mutex {
	m, _ := a.sessLocks.LoadOrStore(id, &sync.Mutex{})
	return m.(*sync.Mutex)
}

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
				Text:           safety.EmergencyResponseZH(emerg),
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
		retrieved, _ = a.retriever.Retrieve(ctx, userMessage, a.cfg.KnowledgeTopK)
		slog.Debug("Knowledge retrieved", "count", len(retrieved))
		if len(retrieved) > 0 {
			step(StepEvent{Type: "retrieve", Summary: fmt.Sprintf("检索知识库，命中 %d 条相关条目", len(retrieved))})
		} else {
			step(StepEvent{Type: "retrieve", Summary: "知识库未检索到相关条目，将如实告知并引导"})
		}
	}

	// Build system prompt
	patientCtx := a.buildPatientContextString(sess)
	systemPrompt := a.composer.ComposeSystemPrompt(retrieved, patientCtx)

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

	// Duplicate tool call detection and tool call budget.
	calledTools := make(map[string]int) // "toolName:paramsHash" -> count
	toolCallCount := 0
	maxToolCalls := 3 // budget: after 3 successful calls, force text answer
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

		resp, err := a.streamWithRetry(ctx, messages, iterTools, iterPrompt, onDelta)
		if err != nil {
			slog.Error("LLM StreamChat failed",
				"error", err,
				"conversation_id", sess.ID,
				"iteration", i,
				"max_iterations", maxIterations,
			)
			return nil, err
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

			// Execute tools; each result becomes a tool-role message that
			// answers its tool_call_id. OpenAI-compatible endpoints reject
			// tool_calls not followed by matching tool messages, and
			// Anthropic expects the equivalent tool_result blocks.
			var toolMsgs []llm.Message
			for _, tc := range resp.ToolCalls {
				// Duplicate detection: skip if same tool + same params
				// was already called in this turn.
				dedupeKey := tc.Name + ":" + tools.ParamsHash(tc.Name, tc.Arguments)
				if calledTools[dedupeKey] >= 1 {
					slog.Warn("Duplicate tool call detected, skipping",
						"tool", tc.Name, "conversation_id", sess.ID, "iteration", i)
					step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具「%s」重复调用已拦截", tc.Name)})
					toolMsgs = append(toolMsgs, llm.Message{
						Role: "tool", ToolCallID: tc.ID,
						Content: fmt.Sprintf("[工具 %s 已用相同参数调用过，请勿重复调用。请基于已有结果给出回答。]", tc.Name),
					})
					continue
				}
				calledTools[dedupeKey]++
				toolCallCount++

				slog.Info("Tool use requested", "tool", tc.Name, "id", tc.ID)
				step(StepEvent{Type: "tool_call", Tool: tc.Name, Summary: fmt.Sprintf("调用工具「%s」", tc.Name)})
				result, err := a.registry.Dispatch(ctx, tc.Name, tc.Arguments)
				var content string
				if err != nil {
					content = fmt.Sprintf("[工具 %s 执行错误: %v]", tc.Name, err)
					step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具「%s」执行出错：%v", tc.Name, err)})
				} else if !result.Success {
					content = fmt.Sprintf("[工具 %s 返回错误: %s]", tc.Name, result.Error)
					step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具「%s」返回错误：%s", tc.Name, result.Error)})
				} else {
					resultJSON, _ := json.MarshalIndent(result.Data, "", "  ")
					content = string(resultJSON)
					toolRefs = append(toolRefs, result.Citations...)
					step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具「%s」返回结果（%d 条引用）", tc.Name, len(result.Citations))})
				}
				toolMsgs = append(toolMsgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: content})
			}

			// Check tool budget after this batch of tool calls.
			if toolCallCount >= maxToolCalls {
				toolBudgetExceeded = true
			}

			messages = append(messages, assistantMsg)
			messages = append(messages, toolMsgs...)
			continue
		}

		// No tool calls → final response
		responseText := resp.Text

		// Update session
		sess.AddUserMessage(userMessage)
		a.saveSession(sess)

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
				if verifyResult.CorrectedResponse != "" {
					responseText = verifyResult.CorrectedResponse
				}
			}
		}

		// L4: Apply disclaimer
		disclaimerSent := false
		if !sess.DisclaimerSent {
			responseText = a.disclaimerService.Apply(sess.ID, responseText)
			sess.DisclaimerSent = true
			disclaimerSent = true
		}

		sess.AddAssistantMessage(responseText)
		a.saveSession(sess)
		sess.TrimHistory(a.cfg.MaxHistoryTurns)

		return &Response{
			Text:           responseText,
			DisclaimerSent: disclaimerSent,
		}, nil
	}

	// Max iterations exceeded — force a final text response without tools
	// so the LLM must summarize what it found instead of erroring out.
	slog.Warn("Agent exceeded maximum tool-use iterations, forcing final response without tools",
		"conversation_id", sess.ID,
		"max_iterations", maxIterations,
	)
	step(StepEvent{Type: "generate", Summary: "正在根据已有信息组织最终回答…"})
	finalResp, err := a.provider.StreamChat(ctx, messages, nil,
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

	// Update session
	sess.AddUserMessage(userMessage)
	a.saveSession(sess)

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
			if verifyResult.CorrectedResponse != "" {
				responseText = verifyResult.CorrectedResponse
			}
		}
	}

	// L4: Apply disclaimer
	disclaimerSent := false
	if !sess.DisclaimerSent {
		responseText = a.disclaimerService.Apply(sess.ID, responseText)
		sess.DisclaimerSent = true
		disclaimerSent = true
	}

	sess.AddAssistantMessage(responseText)
	a.saveSession(sess)
	sess.TrimHistory(a.cfg.MaxHistoryTurns)

	return &Response{
		Text:           responseText,
		DisclaimerSent: disclaimerSent,
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
				Text:           safety.EmergencyResponseZH(emerg),
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
		retrieved, _ = a.retriever.Retrieve(ctx, userMessage, a.cfg.KnowledgeTopK)
		slog.Debug("Knowledge retrieved", "count", len(retrieved))
		if len(retrieved) > 0 {
			step(StepEvent{Type: "retrieve", Summary: fmt.Sprintf("检索知识库，命中 %d 条相关条目", len(retrieved))})
		} else {
			step(StepEvent{Type: "retrieve", Summary: "知识库未检索到相关条目，将如实告知并引导"})
		}
	}

	// Build system prompt
	patientCtx := a.buildPatientContextString(sess)
	systemPrompt := a.composer.ComposeSystemPrompt(retrieved, patientCtx)

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

	// Duplicate tool call detection and tool call budget.
	calledTools := make(map[string]int) // "toolName:paramsHash" -> count
	toolCallCount := 0
	maxToolCalls := 3 // budget: after 3 successful calls, force text answer
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
		if onDelta != nil {
			llmResp, llmErr = a.streamWithRetry(ctx, messages, iterTools, iterPrompt, onDelta)
		} else {
			llmResp, llmErr = a.provider.Chat(ctx, messages, iterTools, iterPrompt)
		}
		if llmErr != nil {
			slog.Error("LLM call failed",
				"error", llmErr,
				"conversation_id", sess.ID,
				"iteration", i,
				"max_iterations", maxIterations,
				"has_images", len(images) > 0,
			)
			return nil, fmt.Errorf("LLM call: %w", llmErr)
		}

		// No tool calls → final answer
		if len(llmResp.ToolCalls) == 0 {
			responseText := llmResp.Text

			// L3: Citation post-verification
			if a.postVerifier != nil {
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
					if verifyResult.CorrectedResponse != "" {
						responseText = verifyResult.CorrectedResponse
					}
				}
			}

			// L4: Apply disclaimer
			disclaimerSent := false
			if !sess.DisclaimerSent {
				responseText = a.disclaimerService.Apply(sess.ID, responseText)
				sess.DisclaimerSent = true
				disclaimerSent = true
			}

			sess.AddAssistantMessage(responseText)
			a.saveSession(sess)
			sess.TrimHistory(a.cfg.MaxHistoryTurns)

			return &Response{
				Text:           responseText,
				DisclaimerSent: disclaimerSent,
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

		// Each result becomes a tool-role message answering its tool_call_id
		// (required by OpenAI-compatible endpoints; Anthropic gets the
		// equivalent tool_result blocks).
		var toolMsgs []llm.Message
		for _, tc := range llmResp.ToolCalls {
			// Duplicate detection: skip if same tool + same params
			// was already called in this turn.
			dedupeKey := tc.Name + ":" + tools.ParamsHash(tc.Name, tc.Arguments)
			if calledTools[dedupeKey] >= 1 {
				slog.Warn("Duplicate tool call detected, skipping",
					"tool", tc.Name, "conversation_id", sess.ID, "iteration", i)
				step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具 %s 重复调用已拦截", tc.Name)})
				toolMsgs = append(toolMsgs, llm.Message{
					Role: "tool", ToolCallID: tc.ID,
					Content: fmt.Sprintf("[工具 %s 已用相同参数调用过，请勿重复调用。请基于已有结果给出回答。]", tc.Name),
				})
				continue
			}
			calledTools[dedupeKey]++
			toolCallCount++

			step(StepEvent{Type: "tool_call", Tool: tc.Name, Summary: fmt.Sprintf("调用工具 %s", tc.Name)})
			toolResult, err := a.registry.Dispatch(ctx, tc.Name, tc.Arguments)
			var content string
			if err != nil {
				content = fmt.Sprintf("[工具 %s 执行错误: %v]", tc.Name, err)
				step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具 %s 执行出错: %v", tc.Name, err)})
			} else {
				toolRefs = append(toolRefs, toolResult.Citations...)
				step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具 %s 返回结果", tc.Name)})
				if toolResult.Success {
					resultJSON, _ := json.MarshalIndent(toolResult.Data, "", "  ")
					content = string(resultJSON)
				} else {
					content = fmt.Sprintf("[工具 %s 返回错误: %s]", tc.Name, toolResult.Error)
				}
			}
			toolMsgs = append(toolMsgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: content})
		}

		// Check tool budget after this batch of tool calls.
		if toolCallCount >= maxToolCalls {
			toolBudgetExceeded = true
		}

		messages = append(messages, toolMsgs...)
	}

	// Max iterations exceeded — force a final text response without tools
	// so the LLM must summarize what it found instead of erroring out.
	slog.Warn("Agent exceeded maximum tool-use iterations, forcing final response without tools",
		"conversation_id", sess.ID,
		"max_iterations", maxIterations,
		"has_images", len(images) > 0,
	)
	step(StepEvent{Type: "generate", Summary: "正在根据已有信息组织最终回答…"})
	finalResp, err := a.provider.StreamChat(ctx, messages, nil,
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

	// L3: Citation post-verification
	if a.postVerifier != nil {
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
			if verifyResult.CorrectedResponse != "" {
				responseText = verifyResult.CorrectedResponse
			}
		}
	}

	// L4: Apply disclaimer
	disclaimerSent := false
	if !sess.DisclaimerSent {
		responseText = a.disclaimerService.Apply(sess.ID, responseText)
		sess.DisclaimerSent = true
		disclaimerSent = true
	}

	sess.AddAssistantMessage(responseText)
	a.saveSession(sess)
	sess.TrimHistory(a.cfg.MaxHistoryTurns)

	return &Response{
		Text:           responseText,
		DisclaimerSent: disclaimerSent,
	}, nil
}

// sessionToMessages returns the session history in provider-agnostic form.
// Sessions now store llm.Message directly, so no conversion is needed.
func (a *Agent) sessionToMessages(sess *session.Session) []llm.Message {
	return sess.GetMessages()
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
			a.sessions[sessionID] = restored
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
	a.sessionsMu.Unlock()
	return sess
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
	})
}

// Response wraps the agent's output to the user.
type Response struct {
	Text           string `json:"text"`
	IsEmergency    bool   `json:"is_emergency"`
	IsOutOfScope   bool   `json:"is_out_of_scope"`
	QualityWarning string `json:"quality_warning,omitempty"`
	DisclaimerSent bool   `json:"disclaimer_sent"`
}
