package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/doctor-agent/internal/config"
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

	emergencyDetector *safety.EmergencyDetector
	scopeGuard        *safety.ScopeGuard
	disclaimerService *safety.DisclaimerService
	postVerifier      *safety.PostVerifier

	sessionsMu sync.RWMutex
	sessions   map[string]*session.Session
	fileStore  *session.FileStore // optional on-disk snapshot store
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

	retriever := knowledge.NewRetriever(store)
	composer := prompt.NewComposer()
	registry := tools.NewRegistry()

	// Register all medical tools
	registry.Register(tools.NewDrugSafetyCheck(store))
	registry.Register(tools.NewGeneticRiskCalculator(store))
	registry.Register(tools.NewFoodRiskAnalyzer(store))
	registry.Register(tools.NewSymptomTriage(store))
	registry.Register(tools.NewReferenceLookup(store))
	registry.Register(tools.NewLiteratureSearch(store))
	registry.Register(tools.NewMSDSearch(store))
	registry.Register(tools.NewVariantLookup(store))
	registry.Register(tools.NewMedlineSearch(store))
	registry.Register(tools.NewDrugLookup(store))
	registry.Register(tools.NewEMLLookup(store))
	registry.Register(tools.NewDrugLabelLookup(store))
	registry.Register(tools.NewNhcSearch(store))
	registry.Register(tools.NewFhsSearch(store))
	registry.Register(tools.NewAapSearch(store))
	registry.Register(tools.NewLabInterpreter())
	registry.Register(tools.NewICD10Lookup(store))
	registry.Register(tools.NewNMPADrugLookup(store))
	registry.Register(tools.NewMedicalKGLookup(store))
	registry.Register(tools.NewDiseaseEncyclopediaLookup(store))
	registry.Register(tools.NewCPubMedKGLookup(store))
	registry.Register(tools.NewHuatuoQALookupTool(store))

	postVerifier := safety.NewPostVerifier(store.GetReferenceIndex())
	if cfg.JudgeEnabled {
		judge, err := createJudgeProvider(cfg)
		if err != nil {
			return nil, fmt.Errorf("creating judge provider: %w", err)
		}
		slog.Info("Semantic claim verification enabled", "judge", judge.Name())
		postVerifier = safety.NewPostVerifierWithJudge(store.GetReferenceIndex(), judge)
	}

	// Optional on-disk session persistence.
	var fileStore *session.FileStore
	if cfg.SessionDir != "" {
		fileStore, err = session.NewFileStore(cfg.SessionDir)
		if err != nil {
			return nil, fmt.Errorf("initializing session store: %w", err)
		}
		slog.Info("Session persistence enabled", "dir", cfg.SessionDir)
	}

	return &Agent{
		cfg:               cfg,
		provider:          provider,
		store:             store,
		retriever:         retriever,
		composer:          composer,
		registry:          registry,
		emergencyDetector: safety.NewEmergencyDetector(),
		scopeGuard:        safety.NewScopeGuard(),
		disclaimerService: safety.NewDisclaimerService(),
		postVerifier:      postVerifier,
		sessions:          make(map[string]*session.Session),
		fileStore:         fileStore,
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

// StepEvent describes one visible step of the agent's pipeline (retrieval,
// tool use, generation, verification). Clients (web UI / CLI) subscribe via
// the onStep callback of ProcessMessageStream to show the user what the agent
// is doing while it works.
type StepEvent struct {
	Type    string `json:"type"` // "emergency" | "refuse" | "retrieve" | "tool_call" | "tool_result" | "generate" | "verify"
	Tool    string `json:"tool,omitempty"`
	Summary string `json:"summary"` // Chinese, human-readable
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

	toolDescs := a.registry.GetToolDescriptions()
	if len(toolDescs) > 0 {
		systemPrompt += "\n" + a.composer.ComposeToolPrompt(toolDescs)
	}

	// Build messages in provider-agnostic format
	messages := a.sessionToMessages(sess)
	messages = append(messages, llm.Message{Role: "user", Content: userMessage})

	toolDefs := a.registry.GetGenericToolDefinitions()

	// Agent loop: call LLM, handle tool use, repeat until final response
	maxIterations := 5
	var toolRefs []tools.CitationRef // tool-returned sources for post-verification
	for i := 0; i < maxIterations; i++ {
		if i == 0 {
			step(StepEvent{Type: "generate", Summary: "正在思考…"})
		} else {
			step(StepEvent{Type: "generate", Summary: "正在根据工具结果组织回答…"})
		}

		resp, err := a.provider.StreamChat(ctx, messages, toolDefs, systemPrompt, onDelta)
		if err != nil {
			return nil, fmt.Errorf("LLM error: %w", err)
		}

		// Check for tool calls
		if len(resp.ToolCalls) > 0 {
			// Build assistant message with text + tool_calls
			assistantMsg := llm.Message{Role: "assistant", Content: resp.Text, ToolCalls: resp.ToolCalls}

			// Execute tools and collect results
			var toolResults strings.Builder
			for _, tc := range resp.ToolCalls {
				slog.Info("Tool use requested", "tool", tc.Name, "id", tc.ID)
				step(StepEvent{Type: "tool_call", Tool: tc.Name, Summary: fmt.Sprintf("调用工具「%s」", tc.Name)})
				result, err := a.registry.Dispatch(ctx, tc.Name, tc.Arguments)
				if err != nil {
					fmt.Fprintf(&toolResults, "[工具 %s 执行错误: %v]\n", tc.Name, err)
					step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具「%s」执行出错：%v", tc.Name, err)})
				} else if !result.Success {
					fmt.Fprintf(&toolResults, "[工具 %s 返回错误: %s]\n", tc.Name, result.Error)
					step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具「%s」返回错误：%s", tc.Name, result.Error)})
				} else {
					resultJSON, _ := json.MarshalIndent(result.Data, "", "  ")
					fmt.Fprintf(&toolResults, "[工具 %s 结果]:\n%s\n", tc.Name, string(resultJSON))
					toolRefs = append(toolRefs, result.Citations...)
					step(StepEvent{Type: "tool_result", Tool: tc.Name, Summary: fmt.Sprintf("工具「%s」返回结果（%d 条引用）", tc.Name, len(result.Citations))})
				}
			}

			messages = append(messages, assistantMsg)
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: fmt.Sprintf("工具执行结果如下。请基于这些结果继续回答用户的问题。\n\n%s", toolResults.String()),
			})
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

	return nil, fmt.Errorf("exceeded maximum tool-use iterations (%d)", maxIterations)
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

	// Try to restore from disk before creating a fresh session.
	if a.fileStore != nil {
		if restored, err := a.fileStore.Load(sessionID); err != nil {
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

// saveSession persists a session snapshot when a file store is configured.
// Snapshotting happens after user/assistant messages are appended.
func (a *Agent) saveSession(sess *session.Session) {
	if a.fileStore == nil {
		return
	}
	if err := a.fileStore.Save(sess); err != nil {
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
