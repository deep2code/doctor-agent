package tools

import (
	"context"
	"testing"

	"github.com/doctor-agent/internal/knowledge"
)

// --- Registry tests ---

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if len(r.ListTools()) != 0 {
		t.Fatalf("expected empty registry, got %d tools", len(r.ListTools()))
	}
}

func TestRegisterAndGetTool(t *testing.T) {
	r := NewRegistry()
	tool := NewLabInterpreter()
	r.Register(tool)

	got, ok := r.GetTool("lab_interpreter")
	if !ok {
		t.Fatal("GetTool returned false for registered tool")
	}
	if got.Name() != "lab_interpreter" {
		t.Fatalf("expected name lab_interpreter, got %s", got.Name())
	}
}

func TestRegisterDuplicateOverwrites(t *testing.T) {
	r := NewRegistry()
	r.Register(NewLabInterpreter())
	r.Register(NewLabInterpreter())

	names := r.ListTools()
	if len(names) != 1 {
		t.Fatalf("expected 1 tool after duplicate register, got %d", len(names))
	}
}

func TestGetToolUnknown(t *testing.T) {
	r := NewRegistry()
	_, ok := r.GetTool("nonexistent")
	if ok {
		t.Fatal("expected false for unknown tool")
	}
}

func TestListToolsOrder(t *testing.T) {
	r := NewRegistry()
	r.Register(NewLabInterpreter())
	r.Register(NewFoodRiskAnalyzer(nil))

	names := r.ListTools()
	if len(names) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(names))
	}
	if names[0] != "lab_interpreter" || names[1] != "food_risk_analyzer" {
		t.Fatalf("unexpected order: %v", names)
	}
}

func TestDispatchUnknownTool(t *testing.T) {
	r := NewRegistry()
	_, err := r.Dispatch(context.Background(), "unknown", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool dispatch")
	}
}

func TestDispatchKnownTool(t *testing.T) {
	r := NewRegistry()
	r.Register(NewLabInterpreter())
	result, err := r.Dispatch(context.Background(), "lab_interpreter", map[string]any{
		"test_name": "MCV",
		"value":     75.0,
		"unit":      "fL",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
}

func TestGetToolDefinitions(t *testing.T) {
	r := NewRegistry()
	r.Register(NewLabInterpreter())
	defs := r.GetToolDefinitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
}

func TestGetGenericToolDefinitions(t *testing.T) {
	r := NewRegistry()
	r.Register(NewLabInterpreter())
	defs := r.GetGenericToolDefinitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Name != "lab_interpreter" {
		t.Fatalf("expected name lab_interpreter, got %s", defs[0].Name)
	}
}

func TestGetToolDescriptions(t *testing.T) {
	r := NewRegistry()
	r.Register(NewLabInterpreter())
	descs := r.GetToolDescriptions()
	if len(descs) != 1 {
		t.Fatalf("expected 1 description, got %d", len(descs))
	}
}

// --- Tool interface compliance (all 16 tools) ---

var allToolConstructors = []struct {
	name string
	fn   func() Tool
}{
	{"drug_safety_check", func() Tool { return NewDrugSafetyCheck(nil) }},
	{"genetic_risk_calculator", func() Tool { return NewGeneticRiskCalculator(nil) }},
	{"food_risk_analyzer", func() Tool { return NewFoodRiskAnalyzer(nil) }},
	{"symptom_triage", func() Tool { return NewSymptomTriage(nil) }},
	{"reference_lookup", func() Tool { return NewReferenceLookup(nil) }},
	{"literature_search", func() Tool { return NewLiteratureSearch(nil) }},
	{"msd_search", func() Tool { return NewMSDSearch(nil) }},
	{"variant_lookup", func() Tool { return NewVariantLookup(nil) }},
	{"medline_search", func() Tool { return NewMedlineSearch(nil) }},
	{"drug_lookup", func() Tool { return NewDrugLookup(nil) }},
	{"eml_lookup", func() Tool { return NewEMLLookup(nil) }},
	{"drug_label_lookup", func() Tool { return NewDrugLabelLookup(nil) }},
	{"nhc_search", func() Tool { return NewNhcSearch(nil) }},
	{"fhs_search", func() Tool { return NewFhsSearch(nil) }},
	{"aap_search", func() Tool { return NewAapSearch(nil) }},
	{"lab_interpreter", func() Tool { return NewLabInterpreter() }},
	{"icd10_lookup", func() Tool { return NewICD10Lookup(nil) }},
	{"nmpa_drug_lookup", func() Tool { return NewNMPADrugLookup(nil) }},
	{"medical_kg_lookup", func() Tool { return NewMedicalKGLookup(nil) }},
	{"disease_encyclopedia_lookup", func() Tool { return NewDiseaseEncyclopediaLookup(nil) }},
	{"cpubmed_kg_lookup", func() Tool { return NewCPubMedKGLookup(nil) }},
}

func TestToolInterfaceCompliance(t *testing.T) {
	for _, tc := range allToolConstructors {
		t.Run(tc.name, func(t *testing.T) {
			tool := tc.fn()
			if tool.Name() == "" {
				t.Error("Name() returned empty")
			}
			if tool.Name() != tc.name {
				t.Errorf("Name() = %q, want %q", tool.Name(), tc.name)
			}
			if tool.Description() == "" {
				t.Error("Description() returned empty")
			}
			schema := tool.Schema()
			if schema == nil {
				t.Error("Schema() returned nil")
			}
			if schema["type"] != "object" {
				t.Errorf("Schema type = %v, want object", schema["type"])
			}
			if _, ok := schema["properties"]; !ok {
				t.Error("Schema missing properties")
			}
			if _, ok := schema["required"]; !ok {
				t.Error("Schema missing required")
			}
		})
	}
}

// --- LabInterpreter Execute tests ---

func TestLabInterpreterMCVLow(t *testing.T) {
	tool := NewLabInterpreter()
	result, err := tool.Execute(context.Background(), map[string]any{
		"test_name": "MCV",
		"value":     72.0,
		"unit":      "fL",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if result.Data["test_name"] != "MCV" {
		t.Errorf("test_name = %v, want MCV", result.Data["test_name"])
	}
}

func TestLabInterpreterMCVLowSouthern(t *testing.T) {
	tool := NewLabInterpreter()
	result, err := tool.Execute(context.Background(), map[string]any{
		"test_name":           "MCV",
		"value":               72.0,
		"unit":                "fL",
		"is_southern_chinese": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	interp, _ := result.Data["interpretation"].(string)
	if interp == "" {
		t.Error("expected interpretation for southern Chinese MCV")
	}
}

func TestLabInterpreterMCVNormal(t *testing.T) {
	tool := NewLabInterpreter()
	result, err := tool.Execute(context.Background(), map[string]any{
		"test_name": "MCV",
		"value":     90.0,
		"unit":      "fL",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
}

func TestLabInterpreterMissingValue(t *testing.T) {
	tool := NewLabInterpreter()
	result, err := tool.Execute(context.Background(), map[string]any{
		"test_name": "MCV",
		"unit":      "fL",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for missing value")
	}
}

func TestLabInterpreterHbLow(t *testing.T) {
	tool := NewLabInterpreter()
	result, err := tool.Execute(context.Background(), map[string]any{
		"test_name": "Hb",
		"value":     85.0,
		"unit":      "g/L",
		"sex":       "male",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
}

func TestLabInterpreterG6PD(t *testing.T) {
	tool := NewLabInterpreter()
	result, err := tool.Execute(context.Background(), map[string]any{
		"test_name": "G6PD",
		"value":     30.0,
		"unit":      "%",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
}

func TestLabInterpreterHbA1cDiabetic(t *testing.T) {
	tool := NewLabInterpreter()
	result, err := tool.Execute(context.Background(), map[string]any{
		"test_name": "HbA1c",
		"value":     7.2,
		"unit":      "%",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
}

func TestLabInterpreterUnknown(t *testing.T) {
	tool := NewLabInterpreter()
	result, err := tool.Execute(context.Background(), map[string]any{
		"test_name": "XYZ_UNKNOWN",
		"value":     100.0,
		"unit":      "U/L",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
}

// --- FoodRiskAnalyzer Execute tests ---

func TestFoodRiskFavaBean(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Skipf("knowledge store not available: %v", err)
	}
	tool := NewFoodRiskAnalyzer(store)
	result, err := tool.Execute(context.Background(), map[string]any{
		"food_name": "蚕豆",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	cat, _ := result.Data["risk_category"].(string)
	if cat == "" {
		t.Error("expected risk_category in result")
	}
}

func TestFoodRiskSaltedFish(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Skipf("knowledge store not available: %v", err)
	}
	tool := NewFoodRiskAnalyzer(store)
	result, err := tool.Execute(context.Background(), map[string]any{
		"food_name": "咸鱼",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
}

func TestFoodRiskMissingParam(t *testing.T) {
	tool := NewFoodRiskAnalyzer(nil)
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for missing food_name")
	}
}

func TestFoodRiskGeneral(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Skipf("knowledge store not available: %v", err)
	}
	tool := NewFoodRiskAnalyzer(store)
	result, err := tool.Execute(context.Background(), map[string]any{
		"food_name": "苹果",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
}

// --- SymptomTriage Execute tests ---

func TestSymptomTriageEmergency(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Skipf("knowledge store not available: %v", err)
	}
	tool := NewSymptomTriage(store)
	result, err := tool.Execute(context.Background(), map[string]any{
		"chief_complaint": "胸痛",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	level, _ := result.Data["triage_level"].(string)
	if level == "" {
		t.Error("expected triage_level in result")
	}
}

func TestSymptomTriageRoutine(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Skipf("knowledge store not available: %v", err)
	}
	tool := NewSymptomTriage(store)
	result, err := tool.Execute(context.Background(), map[string]any{
		"chief_complaint": "轻微头痛",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
}

func TestSymptomTriageMissingComplaint(t *testing.T) {
	tool := NewSymptomTriage(nil)
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for missing chief_complaint")
	}
}

// --- DrugSafetyCheck Execute tests ---

func TestDrugSafetyCheckKnown(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Skipf("knowledge store not available: %v", err)
	}
	tool := NewDrugSafetyCheck(store)
	result, err := tool.Execute(context.Background(), map[string]any{
		"drug_name": "阿司匹林",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
}

func TestDrugSafetyCheckMissing(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Skipf("knowledge store not available: %v", err)
	}
	tool := NewDrugSafetyCheck(store)
	result, err := tool.Execute(context.Background(), map[string]any{
		"drug_name": "UNKNOWN_DRUG_12345",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success (unknown drug), got error: %s", result.Error)
	}
}

func TestDrugSafetyCheckMissingParam(t *testing.T) {
	store, err := knowledge.Load()
	if err != nil {
		t.Skipf("knowledge store not available: %v", err)
	}
	tool := NewDrugSafetyCheck(store)
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for missing drug_name")
	}
}
