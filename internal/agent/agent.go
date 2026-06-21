package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

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

	sessions map[string]*session.Session
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
	registry.Register(tools.NewLabInterpreter())

	postVerifier := safety.NewPostVerifier(store.GetReferenceIndex())

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
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", cfg.LLMProvider)
	}
}

// ProcessMessage handles a single user message within a conversation session.
func (a *Agent) ProcessMessage(ctx context.Context, sess *session.Session, userMessage string) (*Response, error) {
	// L1: Emergency detection
	if a.cfg.EmergencyEnabled {
		if emerg := a.emergencyDetector.Detect(userMessage); emerg != nil {
			slog.Warn("Emergency detected", "matched", emerg.Matched)
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
	}

	// Build system prompt
	patientCtx := a.buildPatientContextString(sess)
	systemPrompt := a.composer.ComposeSystemPrompt(retrieved, patientCtx)
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
	for i := 0; i < maxIterations; i++ {
		resp, err := a.provider.Chat(ctx, messages, toolDefs, systemPrompt)
		if err != nil {
			return nil, fmt.Errorf("LLM error: %w", err)
		}

		// Check for tool calls
		if len(resp.ToolCalls) > 0 {
			// Build assistant message with text + tool_calls
			assistantMsg := llm.Message{Role: "assistant", Content: resp.Text}

			// Execute tools and collect results
			var toolResults strings.Builder
			for _, tc := range resp.ToolCalls {
				slog.Info("Tool use requested", "tool", tc.Name, "id", tc.ID)
				result, err := a.registry.Dispatch(ctx, tc.Name, tc.Arguments)
				if err != nil {
					toolResults.WriteString(fmt.Sprintf("[工具 %s 执行错误: %v]\n", tc.Name, err))
				} else if !result.Success {
					toolResults.WriteString(fmt.Sprintf("[工具 %s 返回错误: %s]\n", tc.Name, result.Error))
				} else {
					resultJSON, _ := json.MarshalIndent(result.Data, "", "  ")
					toolResults.WriteString(fmt.Sprintf("[工具 %s 结果]:\n%s\n", tc.Name, string(resultJSON)))
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

		// L3: Post-generation verification
		if a.cfg.PostVerifyEnabled {
			verifyResult := a.postVerifier.Verify(responseText)
			if !verifyResult.Passed {
				slog.Warn("Response post-verification failed", "warnings", verifyResult.Warnings)
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
		sess.TrimHistory(a.cfg.MaxHistoryTurns)

		return &Response{
			Text:           responseText,
			DisclaimerSent: disclaimerSent,
		}, nil
	}

	return nil, fmt.Errorf("exceeded maximum tool-use iterations (%d)", maxIterations)
}

// sessionToMessages converts session history to provider-agnostic messages.
func (a *Agent) sessionToMessages(sess *session.Session) []llm.Message {
	anthropicMsgs := sess.GetMessages()
	msgs := make([]llm.Message, 0, len(anthropicMsgs))

	for _, m := range anthropicMsgs {
		// Extract the role and text content from Anthropic-specific message params
		msg := llm.Message{}
		switch {
		case m.Role == "user":
			msg.Role = "user"
		case m.Role == "assistant":
			msg.Role = "assistant"
		default:
			continue
		}

		// Extract text from content blocks (simplified: concatenate all text)
		if m.Content != nil {
			// Content is a complex union type; extract text heuristically
			msg.Content = extractTextContent(m)
		}

		if msg.Content != "" {
			msgs = append(msgs, msg)
		}
	}

	return msgs
}

// extractTextContent extracts text from an Anthropic message's content union.
// This handles the common cases; for full fidelity, use the SDK's AsAny() methods.
func extractTextContent(m anthropic.MessageParam) string {
	// The Content field is []ContentBlockParamUnion, but it's stored as a
	// complex union type. For simplicity, we serialize and extract text.
	// In practice, messages built by our session manager always use
	// NewTextBlock, making this straightforward.
	data, err := json.Marshal(m.Content)
	if err != nil {
		return ""
	}

	// Simple heuristic: find "text" fields in the JSON
	var contentBlocks []map[string]any
	if err := json.Unmarshal(data, &contentBlocks); err != nil {
		return ""
	}

	var texts []string
	for _, block := range contentBlocks {
		if t, ok := block["text"].(string); ok && t != "" {
			texts = append(texts, t)
		}
	}

	return strings.Join(texts, "\n")
}

// GetOrCreateSession returns an existing session or creates a new one.
func (a *Agent) GetOrCreateSession(sessionID string) *session.Session {
	if sess, ok := a.sessions[sessionID]; ok {
		return sess
	}
	sess := session.New(sessionID)
	a.sessions[sessionID] = sess
	return sess
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
