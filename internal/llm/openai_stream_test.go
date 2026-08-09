package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenAIStreamingChat verifies that StreamChat forwards text deltas,
// accumulates tool-call arguments across chunks, and marks the request as
// streaming. It exercises the shared openAIStreamingChat code path used by
// both DeepSeekProvider and OpenAICompatProvider.
func TestOpenAIStreamingChat(t *testing.T) {
	var gotStream bool
	var gotStreamValue bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotStream = req.Stream

		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		if req.Stream {
			gotStreamValue = true
			// Text chunks split across multiple SSE events.
			writeSSE(t, w, flusher, `{"choices":[{"delta":{"content":"你好"}}]}`)
			writeSSE(t, w, flusher, `{"choices":[{"delta":{"content":"，"}}]}`)
			writeSSE(t, w, flusher, `{"choices":[{"delta":{"content":"世界"}}]}`)
			// Tool call: name in first chunk, arguments in two fragments.
			writeSSE(t, w, flusher, `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"symptom_triage","arguments":"{\"query\":\""}}]}}]}`)
			writeSSE(t, w, flusher, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"发烧\"}"}}]}}]}`)
			// Final chunk often carries finish_reason.
			writeSSE(t, w, flusher, `{"choices":[{"delta":{},"finish_reason":"stop"}]}`)
			writeSSE(t, w, flusher, `data: [DONE]`)
			return
		}

		// Non-streaming path: plain JSON body, no SSE framing.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"一次性回答"}}]}`))
	}))
	defer srv.Close()

	client := &http.Client{}
	messages := []Message{{Role: "user", Content: "测试"}}

	// --- Streaming path ---
	var deltas []string
	resp, err := openAIStreamingChat(context.Background(), client, srv.URL+"/chat/completions",
		"key", "model", 1024, 0.3, messages, []ToolDefinition{
			{Name: "symptom_triage", Description: "triage", Parameters: map[string]any{}, Required: nil},
		}, "system", func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("StreamChat error: %v", err)
	}
	if !gotStream {
		t.Error("expected stream=true in streaming request")
	}
	if !gotStreamValue {
		t.Error("expected request body to have stream=true")
	}
	if got := strings.Join(deltas, ""); got != "你好，世界" {
		t.Errorf("deltas = %q, want %q", got, "你好，世界")
	}
	if resp.Text != "你好，世界" {
		t.Errorf("resp.Text = %q, want %q", resp.Text, "你好，世界")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "symptom_triage" {
		t.Errorf("tool call = %+v, want id=call_1 name=symptom_triage", tc)
	}
	if tc.Arguments["query"] != "发烧" {
		t.Errorf("tool arguments = %v, want query=发烧", tc.Arguments)
	}

	// --- Non-streaming path ---
	gotStream = false
	resp2, err := openAIStreamingChat(context.Background(), client, srv.URL+"/chat/completions",
		"key", "model", 1024, 0.3, messages, nil, "system", nil)
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if gotStream {
		t.Error("expected stream=false in non-streaming request")
	}
	if resp2.Text != "一次性回答" {
		t.Errorf("resp2.Text = %q, want %q", resp2.Text, "一次性回答")
	}
}

func writeSSE(t *testing.T, w http.ResponseWriter, f http.Flusher, data string) {
	t.Helper()
	if _, err := w.Write([]byte("data: " + data + "\n\n")); err != nil {
		t.Fatalf("write sse: %v", err)
	}
	f.Flush()
}
