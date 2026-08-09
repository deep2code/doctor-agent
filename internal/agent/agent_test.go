package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/llm"
	"github.com/doctor-agent/internal/prompt"
	"github.com/doctor-agent/internal/safety"
	"github.com/doctor-agent/internal/session"
	"github.com/doctor-agent/internal/tools"
)

// fakeProvider returns preset responses in order; StreamChat forwards the
// per-call deltas (streamed[i] belongs to the i-th LLM call).
type fakeProvider struct {
	responses []*llm.ChatResponse
	streamed  [][]string // deltas forwarded on each StreamChat call
	chatCalls int
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, _ string) (*llm.ChatResponse, error) {
	f.chatCalls++
	if len(f.responses) == 0 {
		return &llm.ChatResponse{}, nil
	}
	r := f.responses[0]
	f.responses = f.responses[1:]
	return r, nil
}

func (f *fakeProvider) StreamChat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, systemPrompt string, onDelta func(string)) (*llm.ChatResponse, error) {
	if f.chatCalls < len(f.streamed) {
		for _, d := range f.streamed[f.chatCalls] {
			if onDelta != nil {
				onDelta(d)
			}
		}
	}
	return f.Chat(ctx, messages, tools, systemPrompt)
}

// echoTool is a trivial tool used to exercise the agent's tool-use loop.
type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "echoes input" }
func (echoTool) Schema() map[string]interface{} {
	return map[string]interface{}{"properties": map[string]interface{}{}}
}
func (echoTool) Execute(_ context.Context, input map[string]interface{}) (*tools.ToolResult, error) {
	return &tools.ToolResult{Success: true, Data: map[string]interface{}{"echo": "ok", "input": input}}, nil
}

func testConfig() *config.Config {
	return &config.Config{
		EmergencyEnabled:  false,
		ScopeGuardEnabled: false,
		KnowledgeEnabled:  false,
		PostVerifyEnabled: false,
		MaxHistoryTurns:   20,
	}
}

func newTestAgent(cfg *config.Config, p llm.LLMProvider) *Agent {
	ag := &Agent{
		cfg:               cfg,
		provider:          p,
		composer:          prompt.NewComposer(),
		registry:          tools.NewRegistry(),
		emergencyDetector: safety.NewEmergencyDetector(),
		scopeGuard:        safety.NewScopeGuard(),
		disclaimerService: safety.NewDisclaimerService(),
		postVerifier:      safety.NewPostVerifier(map[string]string{}),
		sessions:          make(map[string]*session.Session),
	}
	return ag
}

func TestProcessMessageStreamDeliversDeltas(t *testing.T) {
	cfg := testConfig()
	p := &fakeProvider{
		responses: []*llm.ChatResponse{{Text: "你好世界"}},
		streamed:  [][]string{{"你好", "世界"}},
	}
	ag := newTestAgent(cfg, p)
	sess := session.New("t1")

	var got []string
	resp, err := ag.ProcessMessageStream(context.Background(), sess, "测试问题", func(d string) {
		got = append(got, d)
	})
	if err != nil {
		t.Fatalf("ProcessMessageStream: %v", err)
	}
	if strings.Join(got, "") != "你好世界" {
		t.Errorf("deltas = %v, want [你好 世界]", got)
	}
	if !strings.HasPrefix(resp.Text, "你好世界") {
		t.Errorf("resp.Text = %q, want prefix 你好世界", resp.Text)
	}
	if !resp.DisclaimerSent {
		t.Error("first response should carry the disclaimer")
	}
	// User + assistant messages recorded once each.
	if msgs := sess.GetMessages(); len(msgs) != 2 {
		t.Errorf("session messages = %d, want 2", len(msgs))
	}
}

func TestProcessMessageStreamToolLoop(t *testing.T) {
	cfg := testConfig()
	p := &fakeProvider{
		responses: []*llm.ChatResponse{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: map[string]any{"a": "1"}}}},
			{Text: "工具执行完毕后的最终回答"},
		},
		streamed: [][]string{nil, {"工具执行完毕后的最终回答"}},
	}
	ag := newTestAgent(cfg, p)
	ag.registry.Register(echoTool{})
	sess := session.New("t2")

	var deltas []string
	resp, err := ag.ProcessMessageStream(context.Background(), sess, "帮我查一下", func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("ProcessMessageStream: %v", err)
	}
	if !strings.HasPrefix(resp.Text, "工具执行完毕后的最终回答") {
		t.Errorf("resp.Text = %q, want prefix 工具执行完毕后的最终回答", resp.Text)
	}
	if p.chatCalls != 2 {
		t.Errorf("LLM calls = %d, want 2 (tool round + final round)", p.chatCalls)
	}
	if len(deltas) != 1 || deltas[0] != "工具执行完毕后的最终回答" {
		t.Errorf("deltas = %v", deltas)
	}
	if msgs := sess.GetMessages(); len(msgs) != 2 {
		t.Errorf("session messages = %d, want 2", len(msgs))
	}
}

func TestEmergencyBypassesLLM(t *testing.T) {
	cfg := testConfig()
	cfg.EmergencyEnabled = true
	p := &fakeProvider{}
	ag := newTestAgent(cfg, p)
	sess := session.New("t3")

	var deltas []string
	resp, err := ag.ProcessMessageStream(context.Background(), sess, "我突然胸口剧痛，喘不上气", func(d string) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("ProcessMessageStream: %v", err)
	}
	if !resp.IsEmergency {
		t.Error("expected emergency response")
	}
	if p.chatCalls != 0 {
		t.Errorf("LLM should not be called on emergency, got %d calls", p.chatCalls)
	}
	if len(deltas) != 0 {
		t.Errorf("emergency must not stream deltas, got %v", deltas)
	}
	if len(sess.GetMessages()) != 0 {
		t.Error("emergency response must not be stored in session")
	}
}

func TestSessionPersistenceDuringProcessing(t *testing.T) {
	cfg := testConfig()
	cfg.SessionDir = t.TempDir()

	p := &fakeProvider{
		responses: []*llm.ChatResponse{{Text: "持久化回答"}},
		streamed:  [][]string{{"持久化回答"}},
	}
	ag := newTestAgent(cfg, p)

	fs, err := session.NewFileStore(cfg.SessionDir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	ag.fileStore = fs

	sess := session.New("persist-1")
	if _, err := ag.ProcessMessageStream(context.Background(), sess, "问题", nil); err != nil {
		t.Fatalf("ProcessMessageStream: %v", err)
	}

	restored, err := fs.Load("persist-1")
	if err != nil || restored == nil {
		t.Fatalf("Load after processing: %v, %v", restored, err)
	}
	if msgs := restored.GetMessages(); len(msgs) != 2 {
		t.Errorf("persisted messages = %d, want 2", len(msgs))
	}
	if !restored.DisclaimerSent {
		t.Error("disclaimer flag not persisted")
	}
}

func TestGetOrCreateSessionRestoresFromDisk(t *testing.T) {
	cfg := testConfig()
	cfg.SessionDir = t.TempDir()

	fs, _ := session.NewFileStore(cfg.SessionDir)
	// Pre-seed a session file.
	seed := session.New("restored-1")
	seed.AddUserMessage("历史问题")
	seed.AddAssistantMessage("历史回答")
	if err := fs.Save(seed); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	ag := newTestAgent(cfg, &fakeProvider{})
	ag.fileStore = fs

	sess := ag.GetOrCreateSession("restored-1")
	if len(sess.GetMessages()) != 2 {
		t.Fatalf("restored session messages = %d, want 2", len(sess.GetMessages()))
	}

	// Same instance is served from memory on the second call.
	if sess2 := ag.GetOrCreateSession("restored-1"); sess2 != sess {
		t.Error("second GetOrCreateSession returned a different instance")
	}
}
