package tools

import "testing"

// activeToolNames mirrors the registry entries built in
// internal/agent/agent.go New(). Registration alone does not make a tool
// callable: each turn the model only sees the names Router returns, which can
// only come from toolGroups or relationToTools. This gate fails loudly when a
// registered tool is added without a routing home.
var activeToolNames = []string{
	"drug_safety_check",
	"genetic_risk_calculator",
	"food_risk_analyzer",
	"symptom_triage",
	"drug_interaction_check",
	"medical_image_analyze",
	"lab_report_analyze",
	"visit_prep",
	"drug_label_lookup",
	"knowledge_search",
	"exact_lookup",
	"medical_kg_lookup",
	"cpubmed_kg_lookup",
}

func routableToolNames() map[string]bool {
	set := make(map[string]bool)
	for _, names := range toolGroups {
		for _, n := range names {
			set[n] = true
		}
	}
	for _, names := range relationToTools {
		for _, n := range names {
			set[n] = true
		}
	}
	return set
}

func TestEveryActiveToolIsRoutable(t *testing.T) {
	routable := routableToolNames()
	for _, name := range activeToolNames {
		if !routable[name] {
			t.Errorf("tool %q is registered but unreachable: it appears in no toolGroups or relationToTools entry, so Router can never return it", name)
		}
	}
}

// TestRoutableToolsAreAllActive catches the inverse drift: a routing entry
// naming a tool that no longer exists would silently waste one of the 8 slots.
func TestRoutableToolsAreAllActive(t *testing.T) {
	active := make(map[string]bool)
	for _, n := range activeToolNames {
		active[n] = true
	}
	for name := range routableToolNames() {
		if !active[name] {
			t.Errorf("router references %q, which is not an active tool", name)
		}
	}
}

func TestRouterReachesLabelReportAndVisitTools(t *testing.T) {
	r := NewRouter()

	cases := []struct {
		query string
		want  string
	}{
		{"阿莫西林的说明书禁忌是什么", "drug_label_lookup"},
		{"帮我看看这张化验单", "lab_report_analyze"},
		{"血脂报告单偏高", "lab_report_analyze"},
		{"我要去看医生，帮我准备一下", "visit_prep"},
		{"这个病挂什么科，看病要带什么", "visit_prep"},
	}
	for _, c := range cases {
		if got := r.ClassifyMulti(c.query); !contains(got, c.want) {
			t.Errorf("ClassifyMulti(%q) = %v, want it to include %s", c.query, got, c.want)
		}
	}
}

// TestClassifyKGFallsBackToKeywordRoutingWhenStoreMissing pins the store==nil
// path, which is what unit tests and DB-less deployments exercise.
func TestClassifyKGWithoutStoreMatchesClassifyMulti(t *testing.T) {
	r := NewRouter()
	query := "血压高吃什么药"

	k := r.ClassifyKG(query, nil)
	m := r.ClassifyMulti(query)
	if len(k) != len(m) {
		t.Fatalf("ClassifyKG(nil store) = %v, ClassifyMulti = %v", k, m)
	}
	for i := range k {
		if k[i] != m[i] {
			t.Fatalf("ClassifyKG(nil store) = %v, ClassifyMulti = %v", k, m)
		}
	}
}

func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
