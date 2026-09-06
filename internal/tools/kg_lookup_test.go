package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/doctor-agent/internal/knowledge"
)

// KG tools hit the lazily-loaded OpenCMKG / CPubMed-KG datasets; skip when no
// knowledge database is reachable (same convention as body_part_lookup_test).

func TestMedicalKGLookup(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Skipf("knowledge store not available: %v", err)
	}
	tool := NewMedicalKGLookup(store)
	ctx := context.Background()

	res, err := tool.Execute(ctx, map[string]any{"entity": "高血压"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute not successful: %v", res.Error)
	}
	if got := res.Data["result_count"].(int); got == 0 {
		t.Fatal("expected triples for 高血压, got 0")
	}

	// Relation filter narrows results and all relations match.
	res, err = tool.Execute(ctx, map[string]any{"entity": "高血压", "relation": "disease_has_symptom"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if n := res.Data["result_count"].(int); n == 0 {
		t.Fatal("expected symptom triples for 高血压, got 0")
	} else {
		byRelation, _ := json.Marshal(res.Data["by_relation"])
		if !strings.Contains(string(byRelation), "disease_has_symptom") {
			t.Fatalf("relation filter ignored: %s", byRelation)
		}
	}

	// Unknown entity yields a friendly empty result, not an error.
	res, err = tool.Execute(ctx, map[string]any{"entity": "不存在的实体xyz"})
	if err != nil || !res.Success {
		t.Fatalf("unknown entity should be graceful: err=%v res=%v", err, res)
	}
}

func TestCPubMedKGLookup(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Skipf("knowledge store not available: %v", err)
	}
	tool := NewCPubMedKGLookup(store)
	ctx := context.Background()

	res, err := tool.Execute(ctx, map[string]any{"disease": "高血压"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute not successful: %v", res.Error)
	}
	if got := res.Data["result_count"].(int); got == 0 {
		t.Fatal("expected triples for 高血压, got 0")
	}
	rels, _ := json.Marshal(res.Data["relations"])
	if !strings.Contains(string(rels), "药物治疗") && !strings.Contains(string(rels), "临床表现") {
		t.Fatalf("expected core CPubMed relations, got %s", rels)
	}
}

func TestRouterIncludesKGTools(t *testing.T) {
	r := NewRouter()

	for _, q := range []string{
		"高血压有什么并发症", // 并发症 → CatDisease
		"糖尿病治疗",     // 治疗 → CatDisease
		"孩子发烧症状",    // 症状 → CatSymptom
	} {
		tools := r.ClassifyMulti(q)
		joined := strings.Join(tools, ",")
		if !strings.Contains(joined, "medical_kg_lookup") && !strings.Contains(joined, "cpubmed_kg_lookup") {
			t.Errorf("query %q routed without KG tools: [%s]", q, joined)
		}
	}
}
