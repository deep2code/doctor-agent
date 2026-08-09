package knowledge

// --- Existing types (unchanged) ---

// Citation represents a single academic/clinical reference.
type Citation struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Journal string `json:"journal"`
	Year    int    `json:"year"`
	DOI     string `json:"doi,omitempty"`
	PMID    string `json:"pmid,omitempty"`
	Level   string `json:"level"`
	URL     string `json:"url,omitempty"`
}

// PrevalenceEntry holds regional prevalence data for a condition.
type PrevalenceEntry struct {
	Rate       float64 `json:"rate"`
	Population string  `json:"population,omitempty"`
	SampleSize int     `json:"sample_size,omitempty"`
}

// DiagnosticCriteria defines how a condition is diagnosed.
type DiagnosticCriteria struct {
	LabTests         []string          `json:"lab_tests,omitempty"`
	Imaging          []string          `json:"imaging,omitempty"`
	Thresholds       map[string]string `json:"thresholds,omitempty"`
	ClinicalFeatures []string          `json:"clinical_features,omitempty"`
	GoldStandard     string            `json:"gold_standard,omitempty"`
}

// TreatmentOption defines an evidence-based treatment approach.
type TreatmentOption struct {
	Method        string `json:"method"`
	Indication    string `json:"indication"`
	EvidenceLevel string `json:"evidence_level"`
	CitationRef   string `json:"citation_ref"`
	Notes         string `json:"notes,omitempty"`
}

// KnowledgeEntry is the top-level structure for a medical knowledge item.
type KnowledgeEntry struct {
	ID                     string              `json:"id"`
	ConditionZH            string              `json:"condition_zh"`
	ConditionEN            string              `json:"condition_en"`
	Category               string              `json:"category"`
	ICD10                  string              `json:"icd10,omitempty"`
	Regions                []string            `json:"regions"`
	Prevalence             map[string]PrevalenceEntry `json:"prevalence,omitempty"`
	Diagnosis              *DiagnosticCriteria `json:"diagnosis,omitempty"`
	Treatment              []TreatmentOption   `json:"treatment,omitempty"`
	DifferentialDiagnosis  []string            `json:"differential_diagnosis,omitempty"`
	RiskFactors            []string            `json:"risk_factors,omitempty"`
	Complications          []string            `json:"complications,omitempty"`
	Prevention             []string            `json:"prevention,omitempty"`
	WhenToSeekCare         []string            `json:"when_to_seek_care,omitempty"`
	ClinicalExamples       []ClinicalExample   `json:"clinical_examples,omitempty"`
	Citations              []Citation          `json:"citations"`
	Keywords               []string            `json:"keywords"`
}

// ClinicalExample provides a real-world case illustration.
type ClinicalExample struct {
	Description string `json:"description"`
	KeyFindings string `json:"key_findings"`
	Outcome     string `json:"outcome"`
	CitationRef string `json:"citation_ref,omitempty"`
}

// DrugEntry represents a drug with G6PD safety information.
type DrugEntry struct {
	ID            string     `json:"id"`
	GenericNameEN string     `json:"generic_name_en"`
	GenericNameZH string     `json:"generic_name_zh"`
	TradeNames    []string   `json:"trade_names,omitempty"`
	DrugClass     string     `json:"drug_class"`
	G6PDSafety    string     `json:"g6pd_safety"`
	RiskLevel     string     `json:"risk_level"`
	Mechanism     string     `json:"mechanism,omitempty"`
	Alternatives  []string   `json:"alternatives,omitempty"`
	Citations     []Citation `json:"citations"`
	Keywords      []string   `json:"keywords"`
}

// EmergencyRule defines a triage decision rule.
type EmergencyRule struct {
	ID         string     `json:"id"`
	Condition  string     `json:"condition"`
	Keywords   []string   `json:"keywords"`
	KeywordsZH []string   `json:"keywords_zh"`
	Level      string     `json:"level"`
	Action     string     `json:"action"`
	ActionZH   string     `json:"action_zh"`
	Citations  []Citation `json:"citations,omitempty"`
}

// FoodRiskEntry defines food-related health risks.
type FoodRiskEntry struct {
	ID                 string     `json:"id"`
	FoodNameZH         string     `json:"food_name_zh"`
	FoodNameEN         string     `json:"food_name_en"`
	RiskCategory       string     `json:"risk_category"`
	RiskDetail         string     `json:"risk_detail"`
	AffectedPopulation string     `json:"affected_population,omitempty"`
	Severity           string     `json:"severity"`
	Citations          []Citation `json:"citations"`
	Keywords           []string   `json:"keywords"`
}

// LabTestReference defines normal ranges and interpretation.
type LabTestReference struct {
	ID           string            `json:"id"`
	TestNameZH   string            `json:"test_name_zh"`
	TestNameEN   string            `json:"test_name_en"`
	Category     string            `json:"category"`
	NormalRanges map[string]string `json:"normal_ranges"`
	Units        string            `json:"units"`
	HighMeaning  string            `json:"high_meaning,omitempty"`
	LowMeaning   string            `json:"low_meaning,omitempty"`
	CriticalHigh float64           `json:"critical_high,omitempty"`
	CriticalLow  float64           `json:"critical_low,omitempty"`
	Citations    []Citation        `json:"citations"`
	Keywords     []string          `json:"keywords"`
}

// RetrievalResult wraps a knowledge entry with retrieval metadata.
type RetrievalResult struct {
	Entry           KnowledgeEntry `json:"entry"`
	Score           float64        `json:"score"`
	MatchedKeywords []string       `json:"matched_keywords"`
}

// DrugRetrievalResult wraps a drug entry retrieval.
type DrugRetrievalResult struct {
	Entry           DrugEntry `json:"entry"`
	Score           float64   `json:"score"`
	MatchedKeywords []string  `json:"matched_keywords"`
}

// FoodRetrievalResult wraps a food risk entry retrieval.
type FoodRetrievalResult struct {
	Entry           FoodRiskEntry `json:"entry"`
	Score           float64       `json:"score"`
	MatchedKeywords []string      `json:"matched_keywords"`
}

// --- New types for RAG document ingestion ---

// Document represents an unstructured or semi-structured medical document
// that can be chunked and ingested into the vector store.
type Document struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Content     string            `json:"content"`
	SourceType  string            `json:"source_type"`  // pubmed_abstract, guideline, textbook, research_article
	DOI         string            `json:"doi,omitempty"`
	PMID        string            `json:"pmid,omitempty"`
	Journal     string            `json:"journal,omitempty"`
	Year        int               `json:"year,omitempty"`
	EvidenceLevel string          `json:"evidence_level,omitempty"`
	Keywords    []string          `json:"keywords,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Chunk represents a single retrievable segment of a document.
type Chunk struct {
	ID           string   `json:"id"`
	DocumentID   string   `json:"document_id"`
	Content      string   `json:"content"`
	ChunkIndex   int      `json:"chunk_index"`
	TotalChunks  int      `json:"total_chunks"`
	Keywords     []string `json:"keywords,omitempty"`
}

// Evidence levels (GRADE and guideline).
const (
	EvidenceNationalGuideline      = "national_guideline"
	EvidenceInternationalGuideline = "international_guideline"
	EvidenceMetaAnalysis           = "meta_analysis"
	EvidenceRCT                    = "rct"
	EvidenceCohort                 = "cohort"
	EvidenceCaseControl            = "case_control"
	EvidenceCaseReport             = "case_report"
	EvidenceExpertOpinion          = "expert_opinion"
)

// Triage levels.
const (
	TriageEmergency = "emergency"
	TriageUrgent    = "urgent"
	TriageRoutine   = "routine"
	TriageSelfCare  = "self_care"
)

// G6PD safety levels.
const (
	G6PDSafe    = "safe"
	G6PDUnsafe  = "unsafe"
	G6PDCaution = "caution"
	G6PDUnknown = "unknown"
)

// Risk levels.
const (
	RiskHigh     = "high"
	RiskModerate = "moderate"
	RiskLow      = "low"
	RiskNone     = "none"
)
