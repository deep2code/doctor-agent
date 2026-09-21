package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRerankTEIArrayShape: bare TEI response [{index, relevance_score}...],
// deliberately out of order — scores must land back at their doc index.
func TestRerankTEIArrayShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rerank" {
			t.Errorf("path = %q, want /rerank", r.URL.Path)
		}
		var req rerankRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Texts) != 3 || req.Query != "宝宝拉肚子" {
			t.Errorf("request = %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"index":2,"relevance_score":0.9},{"index":0,"relevance_score":0.1},{"index":1,"relevance_score":0.5}]`))
	}))
	defer srv.Close()

	p, err := New(srv.URL, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	scores, err := p.Rerank(context.Background(), "宝宝拉肚子", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	want := []float64{0.1, 0.5, 0.9}
	for i := range want {
		if scores[i] != want[i] {
			t.Errorf("scores[%d] = %v, want %v", i, scores[i], want[i])
		}
	}
}

// TestRerankResultsWrapperShape: vLLM/Cohere-style {"results":[...]} parses
// identically.
func TestRerankResultsWrapperShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"index":0,"relevance_score":0.7},{"index":1,"relevance_score":0.3}]}`))
	}))
	defer srv.Close()

	p, _ := New(srv.URL, "", "m")
	scores, err := p.Rerank(context.Background(), "q", []string{"a", "b"})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if scores[0] != 0.7 || scores[1] != 0.3 {
		t.Errorf("scores = %v", scores)
	}
}

// TestRerankErrors: non-200 and unusable payloads surface errors (the caller
// degrades to RRF order); empty doc list is a no-op, not a request.
func TestRerankErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rerankRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Query {
		case "mode=status":
			http.Error(w, "overloaded", http.StatusServiceUnavailable)
		case "mode=garbage":
			_, _ = w.Write([]byte(`not json`))
		default:
			// index outside the doc range → no usable scores
			_, _ = w.Write([]byte(`[{"index":99,"relevance_score":1}]`))
		}
	}))
	defer srv.Close()
	p, _ := New(srv.URL, "", "")

	if _, err := p.Rerank(context.Background(), "q", []string{"a"}); err == nil {
		t.Error("empty-index response should error")
	}
	if _, err := p.Rerank(context.Background(), "mode=status", []string{"a", "b"}); err == nil {
		t.Error("non-200 should error")
	}
	if _, err := p.Rerank(context.Background(), "mode=garbage", []string{"a", "b", "c"}); err == nil {
		t.Error("unparseable body should error")
	}

	if scores, err := p.Rerank(context.Background(), "q", nil); err != nil || scores != nil {
		t.Errorf("empty docs = (%v, %v), want (nil, nil) without a request", scores, err)
	}
}

func TestNewRequiresBaseURL(t *testing.T) {
	if _, err := New("", "", ""); err == nil {
		t.Error("empty baseURL must error")
	}
}
