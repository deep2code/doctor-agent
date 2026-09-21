package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/knowledge"
	"github.com/doctor-agent/internal/llm"
	"github.com/doctor-agent/internal/prompt"
	"github.com/doctor-agent/internal/safety"
	"github.com/doctor-agent/internal/session"
	"github.com/doctor-agent/internal/tools"
)

// fakeProvider returns preset responses in order; StreamChat forwards the
// per-call deltas (streamed[i] belongs to the i-th LLM call).
type fakeProvider struct {
	responses     []*llm.ChatResponse
	streamed      [][]string // deltas forwarded on each StreamChat call
	chatCalls     int
	captured      [][]llm.Message        // messages seen on each call (for assertions)
	capturedTools [][]llm.ToolDefinition // tools passed on each call
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Chat(_ context.Context, messages []llm.Message, tools []llm.ToolDefinition, _ string) (*llm.ChatResponse, error) {
	f.chatCalls++
	f.captured = append(f.captured, append([]llm.Message(nil), messages...))
	f.capturedTools = append(f.capturedTools, tools)
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
		cfg:                cfg,
		provider:           p,
		understandProvider: p,
		composer:           prompt.NewComposer(),
		registry:           tools.NewRegistry(),
		router:             tools.NewRouter(),
		emergencyDetector:  safety.NewEmergencyDetector(),
		scopeGuard:         safety.NewScopeGuard(),
		postVerifier:       safety.NewPostVerifier(map[string]string{}),
		sessions:           make(map[string]*session.Session),
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
	}, nil)
	if err != nil {
		t.Fatalf("ProcessMessageStream: %v", err)
	}
	if strings.Join(got, "") != "你好世界" {
		t.Errorf("deltas = %v, want [你好 世界]", got)
	}
	if !strings.HasPrefix(resp.Text, "你好世界") {
		t.Errorf("resp.Text = %q, want prefix 你好世界", resp.Text)
	}
	if resp.DisclaimerSent {
		t.Error("disclaimer injection removed; flag should stay false")
	}
	// User + assistant messages recorded once each.
	if msgs := sess.GetMessages(); len(msgs) != 2 {
		t.Errorf("session messages = %d, want 2", len(msgs))
	}
}

// concurrentTool records the peak number of simultaneous Execute calls so
// tests can assert same-batch tool execution really overlaps.
type concurrentTool struct {
	name string
	hold time.Duration
	mu   sync.Mutex
	cur  int
	peak int
}

func (t *concurrentTool) Name() string        { return t.name }
func (t *concurrentTool) Description() string { return "sleeps while tracking concurrency" }
func (t *concurrentTool) Schema() map[string]interface{} {
	return map[string]interface{}{"properties": map[string]interface{}{}}
}
func (t *concurrentTool) Execute(_ context.Context, input map[string]interface{}) (*tools.ToolResult, error) {
	t.mu.Lock()
	t.cur++
	if t.cur > t.peak {
		t.peak = t.cur
	}
	t.mu.Unlock()
	time.Sleep(t.hold)
	t.mu.Lock()
	t.cur--
	t.mu.Unlock()
	return &tools.ToolResult{Success: true, Data: map[string]interface{}{"echo": "ok", "input": input}}, nil
}

func (t *concurrentTool) peakConcurrency() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.peak
}

// TestProcessMessageStreamParallelToolExecution: two tool calls in one
// assistant response must overlap in execution, and the tool-role messages
// fed back to the LLM must still answer the tool calls in original order.
func TestProcessMessageStreamParallelToolExecution(t *testing.T) {
	cfg := testConfig()
	ct := &concurrentTool{name: "slow", hold: 200 * time.Millisecond}
	p := &fakeProvider{
		responses: []*llm.ChatResponse{
			{ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: "slow", Arguments: map[string]any{"i": float64(1)}},
				{ID: "c2", Name: "slow", Arguments: map[string]any{"i": float64(2)}},
			}},
			{Text: "并行执行完成"},
		},
	}
	ag := newTestAgent(cfg, p)
	ag.registry.Register(ct)
	sess := session.New("par1")

	resp, err := ag.ProcessMessageStream(context.Background(), sess, "帮我查两个东西", nil, nil)
	if err != nil {
		t.Fatalf("ProcessMessageStream: %v", err)
	}
	if !strings.HasPrefix(resp.Text, "并行执行完成") {
		t.Errorf("resp.Text = %q", resp.Text)
	}
	if got := ct.peakConcurrency(); got < 2 {
		t.Errorf("同批工具调用应并行执行，实测最大并发 %d", got)
	}
	if len(p.captured) != 2 {
		t.Fatalf("LLM 调用次数 = %d, want 2", len(p.captured))
	}
	var toolIDs []string
	for _, m := range p.captured[1] {
		if m.Role == "tool" {
			toolIDs = append(toolIDs, m.ToolCallID)
		}
	}
	if len(toolIDs) != 2 || toolIDs[0] != "c1" || toolIDs[1] != "c2" {
		t.Errorf("tool 消息必须按调用原序回填，实际: %v", toolIDs)
	}
}

// TestProcessMessageStreamDuplicateStillSerial: a duplicate (same tool +
// same params) in the same batch is intercepted, not executed twice — the
// parallel path must keep the first-wins dedupe semantics.
func TestProcessMessageStreamDuplicateStillSerial(t *testing.T) {
	cfg := testConfig()
	p := &fakeProvider{
		responses: []*llm.ChatResponse{
			{ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: "echo", Arguments: map[string]any{"a": "1"}},
				{ID: "c2", Name: "echo", Arguments: map[string]any{"a": "1"}},
			}},
			{Text: "去重完成"},
		},
	}
	ag := newTestAgent(cfg, p)
	ag.registry.Register(echoTool{})
	sess := session.New("dup1")

	if _, err := ag.ProcessMessageStream(context.Background(), sess, "重复调用", nil, nil); err != nil {
		t.Fatalf("ProcessMessageStream: %v", err)
	}
	second := p.captured[1]
	var dupNote int
	var toolIDs []string
	for _, m := range second {
		if m.Role == "tool" {
			toolIDs = append(toolIDs, m.ToolCallID)
			if strings.Contains(m.Content, "重复调用") {
				dupNote++
			}
		}
	}
	if len(toolIDs) != 2 || toolIDs[0] != "c1" || toolIDs[1] != "c2" {
		t.Errorf("每个 tool_call 都必须有回填消息且保序，实际: %v", toolIDs)
	}
	if dupNote != 1 {
		t.Errorf("同参重复调用应有 1 条拦截说明，实际 %d", dupNote)
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
	resp, err := ag.ProcessMessageStream(context.Background(), sess, "帮我查一下", func(d string) { deltas = append(deltas, d) }, nil)
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
	// Second LLM call must see: user + assistant(tool_calls) + tool message
	// answering the tool_call_id — OpenAI-compatible endpoints 400 otherwise.
	if len(p.captured) != 2 {
		t.Fatalf("captured calls = %d, want 2", len(p.captured))
	}
	second := p.captured[1]
	if len(second) != 3 {
		t.Fatalf("second call messages = %d, want 3 (user + assistant + tool)", len(second))
	}
	if second[1].Role != "assistant" || len(second[1].ToolCalls) != 1 || second[1].ToolCalls[0].ID != "c1" {
		t.Errorf("second call msg[1] = %+v, want assistant with tool_call c1", second[1])
	}
	if second[2].Role != "tool" || second[2].ToolCallID != "c1" || second[2].Content == "" {
		t.Errorf("second call msg[2] = %+v, want tool message with ToolCallID c1 and content", second[2])
	}
}

func TestProcessMessageStreamMaxIterationsFallback(t *testing.T) {
	cfg := testConfig()
	// LLM calls tools for 4 iterations; on the 5th (last) iteration tools
	// are stripped, so LLM returns a text answer directly — no fallback needed.
	toolResp := &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: map[string]any{"a": "1"}}}}
	p := &fakeProvider{
		responses: []*llm.ChatResponse{
			toolResp, toolResp, toolResp, toolResp, // 4 tool-call rounds
			{Text: "根据已有信息的最终回答"}, // 5th round (nil tools → text)
		},
		streamed: [][]string{nil, nil, nil, nil, {"根据已有信息的最终回答"}},
	}
	ag := newTestAgent(cfg, p)
	ag.registry.Register(echoTool{})
	sess := session.New("maxiter")

	var deltas []string
	resp, err := ag.ProcessMessageStream(context.Background(), sess, "反复查询", func(d string) { deltas = append(deltas, d) }, nil)
	if err != nil {
		t.Fatalf("ProcessMessageStream should not error on max iterations, got: %v", err)
	}
	if !strings.HasPrefix(resp.Text, "根据已有信息的最终回答") {
		t.Errorf("resp.Text = %q, want fallback text", resp.Text)
	}
	// 4 tool rounds + 1 final text round = 5 LLM calls (no 6th fallback)
	if p.chatCalls != 5 {
		t.Errorf("LLM calls = %d, want 5 (4 tool + 1 final-text)", p.chatCalls)
	}
	// The 5th call (last iteration) must have nil/empty tools
	if len(p.capturedTools) != 5 {
		t.Fatalf("capturedTools = %d entries, want 5", len(p.capturedTools))
	}
	if lastTools := p.capturedTools[4]; len(lastTools) != 0 {
		t.Errorf("last iteration tools = %v, want nil/empty (tools stripped)", lastTools)
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
	}, nil)
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
	ag.sessionStore = fs

	sess := session.New("persist-1")
	if _, err := ag.ProcessMessageStream(context.Background(), sess, "问题", nil, nil); err != nil {
		t.Fatalf("ProcessMessageStream: %v", err)
	}

	restored, err := fs.Load("persist-1")
	if err != nil || restored == nil {
		t.Fatalf("Load after processing: %v, %v", restored, err)
	}
	if msgs := restored.GetMessages(); len(msgs) != 2 {
		t.Errorf("persisted messages = %d, want 2", len(msgs))
	}
	if restored.DisclaimerSent {
		t.Error("disclaimer no longer injected; flag should stay false")
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
	ag.sessionStore = fs

	sess := ag.GetOrCreateSession("restored-1")
	if len(sess.GetMessages()) != 2 {
		t.Fatalf("restored session messages = %d, want 2", len(sess.GetMessages()))
	}

	// Same instance is served from memory on the second call.
	if sess2 := ag.GetOrCreateSession("restored-1"); sess2 != sess {
		t.Error("second GetOrCreateSession returned a different instance")
	}
}

// TestClaimSessionIsolatesAccounts pins the guard in front of /chat,
// /chat/stream and /share, whose conversation id comes from the client.
func TestClaimSessionIsolatesAccounts(t *testing.T) {
	ag := newTestAgent(testConfig(), &fakeProvider{})

	sess, ok := ag.ClaimSession("conv-1", "user-a")
	if !ok || sess == nil {
		t.Fatalf("ClaimSession(user-a) = (%v, %v), want the session", sess, ok)
	}
	if other, ok := ag.ClaimSession("conv-1", "user-b"); ok || other != nil {
		t.Errorf("user-b reached user-a's conversation: (%v, %v)", other, ok)
	}
	if again, ok := ag.ClaimSession("conv-1", "user-a"); !ok || again != sess {
		t.Errorf("user-a re-claim = (%v, %v), want the same instance", again, ok)
	}
	// A different id is a different conversation, not a different owner's lock.
	if _, ok := ag.ClaimSession("conv-2", "user-b"); !ok {
		t.Error("user-b refused its own fresh conversation")
	}
	if o := ag.GetOrCreateSession("conv-1").Owner(); o != "user-a" {
		t.Errorf("owner = %q after user-b's attempt, want user-a", o)
	}
}

// Ownership must survive a restart: the snapshot on disk is what a second
// process knows about who the conversation belongs to.
func TestClaimSessionOwnershipSurvivesRestart(t *testing.T) {
	fs, err := session.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	first := newTestAgent(testConfig(), &fakeProvider{})
	first.sessionStore = fs
	sess, ok := first.ClaimSession("conv-persist", "user-a")
	if !ok {
		t.Fatal("first process could not claim")
	}
	sess.AddUserMessage("问题")
	sess.AddAssistantMessage("回答")
	if err := fs.Save(sess); err != nil {
		t.Fatalf("persist: %v", err)
	}

	second := newTestAgent(testConfig(), &fakeProvider{})
	second.sessionStore = fs
	if s, ok := second.ClaimSession("conv-persist", "user-b"); ok || s != nil {
		t.Errorf("user-b claimed user-a's restored conversation: (%v, %v)", s, ok)
	}
	s, ok := second.ClaimSession("conv-persist", "user-a")
	if !ok {
		t.Fatal("user-a lost its own conversation across restarts")
	}
	if len(s.GetMessages()) != 2 {
		t.Errorf("restored messages = %d, want 2", len(s.GetMessages()))
	}
}

// forceSweep makes the reaper run on every insert instead of once a minute.
func forceSweep(t *testing.T) {
	t.Helper()
	prev := sessionSweepInterval
	sessionSweepInterval = 0
	t.Cleanup(func() { sessionSweepInterval = prev })
}

// backdate pretends a session has been untouched for the given duration.
func backdate(id string, ag *Agent, d time.Duration) {
	ag.sessionsMu.Lock()
	defer ag.sessionsMu.Unlock()
	if s := ag.sessions[id]; s != nil {
		s.UpdatedAt = time.Now().Add(-d)
	}
}

func sessionIDs(ag *Agent) []string {
	ag.sessionsMu.RLock()
	defer ag.sessionsMu.RUnlock()
	out := make([]string, 0, len(ag.sessions))
	for id := range ag.sessions {
		out = append(out, id)
	}
	return out
}

func countSessions(ag *Agent) int {
	ag.sessionsMu.RLock()
	defer ag.sessionsMu.RUnlock()
	return len(ag.sessions)
}

func TestIdleSessionEvictedWhenPersisted(t *testing.T) {
	forceSweep(t)
	cfg := testConfig()
	cfg.SessionIdleMinutes = 1
	cfg.MaxActiveSessions = 0
	ag := newTestAgent(cfg, &fakeProvider{})
	ag.sessionIdleTTL = time.Minute
	fs, err := session.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	ag.sessionStore = fs

	ag.GetOrCreateSession("stale")
	ag.GetOrCreateSession("fresh")
	backdate("stale", ag, time.Hour)

	ag.GetOrCreateSession("trigger")
	if _, ok := ag.sessions["stale"]; ok {
		t.Fatalf("idle session was not evicted: %v", sessionIDs(ag))
	}
	if _, ok := ag.sessions["fresh"]; !ok {
		t.Error("recent session was evicted")
	}
	// The point of the store: eviction must be invisible to the user.
	restored := ag.GetOrCreateSession("stale")
	if restored == nil {
		t.Fatal("evicted session did not come back")
	}
}

func TestSessionCapEvictsLeastRecentlyUsed(t *testing.T) {
	forceSweep(t)
	cfg := testConfig()
	cfg.MaxActiveSessions = 3
	ag := newTestAgent(cfg, &fakeProvider{})
	ag.maxSessions = 3

	for i, id := range []string{"a", "b", "c", "d", "e"} {
		ag.GetOrCreateSession(id)
		// Distinct touch times so the LRU order is deterministic.
		backdate(id, ag, time.Duration(10-i)*time.Minute)
	}
	got := sessionIDs(ag)
	if len(got) != 3 {
		t.Fatalf("sessions after cap = %d (%v), want 3", len(got), got)
	}
	for _, id := range []string{"c", "d", "e"} {
		if _, ok := ag.sessions[id]; !ok {
			t.Errorf("newest session %q was evicted instead of the oldest", id)
		}
	}
}

func TestSessionCapAppliesWithoutStore(t *testing.T) {
	forceSweep(t)
	ag := newTestAgent(testConfig(), &fakeProvider{})
	ag.maxSessions = 2

	for _, id := range []string{"a", "b", "c"} {
		ag.GetOrCreateSession(id)
	}
	if n := countSessions(ag); n != 2 {
		t.Fatalf("sessions = %d, want 2 (cap must hold even with no store)", n)
	}
	if _, ok := ag.sessions["a"]; ok {
		t.Error("oldest session survived the cap")
	}
}

func TestReaperSkipsMidTurnSession(t *testing.T) {
	forceSweep(t)
	ag := newTestAgent(testConfig(), &fakeProvider{})
	ag.maxSessions = 2
	ag.GetOrCreateSession("busy")
	ag.GetOrCreateSession("other")

	mu := ag.sessionLock("busy")
	mu.Lock()
	defer mu.Unlock()
	backdate("busy", ag, time.Hour)

	// Over the cap now, and "busy" is the least-recently-touched: it must
	// still be there, because dropping it mid-turn would lose the reply.
	ag.GetOrCreateSession("third")
	if _, ok := ag.sessions["busy"]; !ok {
		t.Fatal("evicted a session while its turn was in flight")
	}
	if _, ok := ag.sessions["other"]; ok {
		t.Error("idle session was not evicted instead of the busy one")
	}
}

// fakeRetriever returns a trivial hit so retrieve steps can be exercised.
type fakeRetriever struct{}

func (fakeRetriever) Retrieve(_ context.Context, _ string, _ int) ([]knowledge.RetrievalResult, error) {
	return []knowledge.RetrievalResult{{Score: 0.9}}, nil
}
func (fakeRetriever) RetrieveDrugs(_ context.Context, _ string, _ int) ([]knowledge.DrugRetrievalResult, error) {
	return nil, nil
}
func (fakeRetriever) Name() string { return "fake-retriever" }

func TestProcessMessageStreamEmitsSteps(t *testing.T) {
	cfg := testConfig()
	cfg.KnowledgeEnabled = true // 触发 retrieve 事件
	p := &fakeProvider{
		responses: []*llm.ChatResponse{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: map[string]any{}}}},
			{Text: "最终回答"},
		},
		streamed: [][]string{nil, {"最终回答"}},
	}
	ag := newTestAgent(cfg, p)
	ag.retriever = fakeRetriever{}
	ag.registry.Register(echoTool{})
	sess := session.New("steps-1")

	var steps []StepEvent
	if _, err := ag.ProcessMessageStream(context.Background(), sess, "帮我查", nil, func(ev StepEvent) {
		steps = append(steps, ev)
	}); err != nil {
		t.Fatalf("ProcessMessageStream: %v", err)
	}

	var types []string
	for _, s := range steps {
		types = append(types, s.Type)
	}
	// 预期顺序：retrieve → generate → tool_call → tool_result → generate
	want := []string{"retrieve", "generate", "tool_call", "tool_result", "generate"}
	if strings.Join(types, ",") != strings.Join(want, ",") {
		t.Errorf("step types = %v, want %v", types, want)
	}
	// 摘要为中文、可读
	if len(steps) > 0 && steps[0].Summary == "" {
		t.Error("step summary must not be empty")
	}
	// 工具名随事件携带
	for _, s := range steps {
		if s.Type == "tool_call" && s.Tool != "echo" {
			t.Errorf("tool_call step Tool = %q, want echo", s.Tool)
		}
	}
}

func TestEmergencyStepEmitted(t *testing.T) {
	cfg := testConfig()
	cfg.EmergencyEnabled = true
	ag := newTestAgent(cfg, &fakeProvider{})
	sess := session.New("steps-2")

	var steps []StepEvent
	if _, err := ag.ProcessMessageStream(context.Background(), sess, "我突然胸口剧痛，喘不上气", nil, func(ev StepEvent) {
		steps = append(steps, ev)
	}); err != nil {
		t.Fatalf("ProcessMessageStream: %v", err)
	}
	if len(steps) != 1 || steps[0].Type != "emergency" {
		t.Errorf("steps = %+v, want single emergency step", steps)
	}
}

// TestToolLoopCarriesToolCallsToNextTurn guards against the Zhipu/OpenAI 400
// "Invalid assistant message: content or tool_calls must be set": the
// assistant tool-use message sent on the next round must carry ToolCalls.
func TestToolLoopCarriesToolCallsToNextTurn(t *testing.T) {
	cfg := testConfig()
	p := &fakeProvider{
		responses: []*llm.ChatResponse{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: map[string]any{"a": "1"}}}},
			{Text: "最终回答"},
		},
		streamed: [][]string{nil, {"最终回答"}},
	}
	ag := newTestAgent(cfg, p)
	ag.registry.Register(echoTool{})
	sess := session.New("toolcalls-1")

	if _, err := ag.ProcessMessageStream(context.Background(), sess, "帮我查", nil, nil); err != nil {
		t.Fatalf("ProcessMessageStream: %v", err)
	}

	if len(p.captured) != 2 {
		t.Fatalf("LLM calls = %d, want 2", len(p.captured))
	}
	// 第二轮中应有一条 assistant 消息携带 ToolCalls（在工具结果 user 消息之前）
	second := p.captured[1]
	var assistant *llm.Message
	for i := range second {
		if second[i].Role == "assistant" && len(second[i].ToolCalls) > 0 {
			assistant = &second[i]
			break
		}
	}
	if assistant == nil {
		t.Fatal("second round has no assistant message with ToolCalls")
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].Name != "echo" {
		t.Errorf("assistant ToolCalls = %+v, want [echo]", assistant.ToolCalls)
	}
}

// cacheFake wraps fakeProvider and implements llm.PromptCacheProvider,
// recording the prefix/rest split on each cached call.
type cacheFake struct {
	*fakeProvider
	prefixes []string
	rests    []string
}

func (c *cacheFake) StreamChatCached(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, prefix, rest string, onDelta func(string)) (*llm.ChatResponse, error) {
	c.prefixes = append(c.prefixes, prefix)
	c.rests = append(c.rests, rest)
	return c.StreamChat(ctx, messages, tools, prefix+rest, onDelta)
}

func TestStreamWithRetryRoutesToPromptCache(t *testing.T) {
	cfg := testConfig()
	p := &cacheFake{fakeProvider: &fakeProvider{
		responses: []*llm.ChatResponse{{Text: "最终回答"}},
	}}
	ag := newTestAgent(cfg, p)
	static := ag.composer.ComposeStaticPrefix()
	full := static + "## 动态部分"

	resp, err := ag.streamWithRetry(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, nil, static, full, nil)
	if err != nil {
		t.Fatalf("streamWithRetry: %v", err)
	}
	if resp.Text != "最终回答" {
		t.Errorf("resp.Text = %q", resp.Text)
	}
	if len(p.prefixes) != 1 || p.prefixes[0] != static {
		t.Errorf("cached calls = %v, want the static prefix exactly once", p.prefixes)
	}
	if p.rests[0] != "## 动态部分" {
		t.Errorf("rest = %q, want 动态部分 only", p.rests[0])
	}

	// Empty prefix must fall back to the plain StreamChat path.
	p2 := &cacheFake{fakeProvider: &fakeProvider{responses: []*llm.ChatResponse{{Text: "x"}}}}
	if _, err := newTestAgent(cfg, p2).streamWithRetry(context.Background(), nil, nil, "", "only-dynamic", nil); err != nil {
		t.Fatalf("streamWithRetry fallback: %v", err)
	}
	if len(p2.prefixes) != 0 || p2.chatCalls != 1 {
		t.Errorf("empty prefix should use plain StreamChat, got prefixes=%v chatCalls=%d", p2.prefixes, p2.chatCalls)
	}
}

// TestProcessMessageStreamCachesStaticPrefix: the full pipeline routes every
// LLM call through the cached path with the byte-stable layer prefix.
func TestProcessMessageStreamCachesStaticPrefix(t *testing.T) {
	cfg := testConfig()
	p := &cacheFake{fakeProvider: &fakeProvider{
		responses: []*llm.ChatResponse{{Text: "你好世界"}},
		streamed:  [][]string{{"你好世界"}},
	}}
	ag := newTestAgent(cfg, p)
	sess := session.New("cache-e2e")

	if _, err := ag.ProcessMessageStream(context.Background(), sess, "测试问题", nil, nil); err != nil {
		t.Fatalf("ProcessMessageStream: %v", err)
	}
	static := ag.composer.ComposeStaticPrefix()
	if len(p.prefixes) == 0 {
		t.Fatal("expected cached streaming calls")
	}
	for i, pre := range p.prefixes {
		if pre != static {
			t.Errorf("call %d: prefix differs from static layers", i)
		}
	}
	for i, r := range p.rests {
		if strings.Contains(r, "DUAL-VERSION OUTPUT") {
			t.Errorf("call %d: dynamic rest still contains static layer content", i)
		}
	}
}
