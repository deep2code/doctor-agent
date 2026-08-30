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

// HealthMyth represents a common health misconception and its correction.
type HealthMyth struct {
	ID           string   `json:"id"`
	Category     string   `json:"category"`
	Myth         string   `json:"myth"`
	Reality      string   `json:"reality"`
	Harm         string   `json:"harm"`
	CorrectAction string  `json:"correct_action"`
	Keywords     []string `json:"keywords"`
}

// HealthMythSet wraps a collection of health myths.
type HealthMythSet struct {
	Myths []HealthMyth `json:"myths"`
}

// BodyPartTriage maps a human body region (as shown on the interactive body
// map) to common conditions, red-flag symptoms, suggested departments and
// home-care advice. It powers body_part_lookup so users who can only point to
// where it hurts still get evidence-based guidance.
type BodyPartTriage struct {
	ID          string     `json:"id"`
	PartKey     string     `json:"part_key"`
	PartZH      string     `json:"part_zh"`
	Aliases     []string   `json:"aliases,omitempty"`
	Side        string     `json:"side"` // front | back
	Conditions  []string   `json:"conditions,omitempty"`
	RedFlags    string     `json:"red_flags,omitempty"`
	Departments []string   `json:"departments,omitempty"`
	SelfCare    []string   `json:"self_care,omitempty"`
	Citations   []Citation `json:"citations,omitempty"`
}

// EssentialMedicine represents a drug on the China National Essential
// Medicines List (国家基本药物目录), focused on patient-facing information.
type EssentialMedicine struct {
	ID               string   `json:"id"`
	NameZH           string   `json:"name_zh"`
	NameEN           string   `json:"name_en"`
	Category         string   `json:"category"`
	ATC              string   `json:"atc,omitempty"`
	InsuranceClass   string   `json:"insurance_class"`
	CommonUses       []string `json:"common_uses"`
	DosageForms      []string `json:"dosage_forms"`
	KeyWarnings      []string `json:"key_warnings"`
	PatientTips      string   `json:"patient_tips"`
	Keywords         []string `json:"keywords"`
}

// EssentialMedicineSet wraps a collection of essential medicines.
type EssentialMedicineSet struct {
	Drugs []EssentialMedicine `json:"drugs"`
}

// ICD10Disease represents a disease entry from the ICD-10 classification.
type ICD10Disease struct {
	Code    string `json:"icd10_code"`
	NameZH  string `json:"name_zh"`
	Category string `json:"category"`
}

// ICD10DiseaseSet wraps a collection of ICD-10 diseases.
type ICD10DiseaseSet struct {
	Diseases []ICD10Disease `json:"diseases"`
}

// NMPADrug represents a drug entry from the NMPA (国家药品监督管理局) catalogue.
type NMPADrug struct {
	Code   string `json:"drug_code"`
	NameZH string `json:"name_zh"`
	Source string `json:"source"`
}

// NMPADrugSet wraps a collection of NMPA drugs.
type NMPADrugSet struct {
	Drugs []NMPADrug `json:"drugs"`
}

// MedicalKGTriple represents a knowledge graph triple (entity1-relation-entity2).
type MedicalKGTriple struct {
	Entity1  string `json:"entity1"`
	Relation string `json:"relation"`
	Entity2  string `json:"entity2"`
}

// MedicalKGRelationSummary summarizes counts by relation type.
type MedicalKGRelationSummary struct {
	Relation string `json:"relation"`
	Count    int    `json:"count"`
}

// MedicalKGSet wraps the medical knowledge graph triples.
type MedicalKGSet struct {
	Triples         []MedicalKGTriple        `json:"triples"`
	TotalCount      int                      `json:"total_count"`
	RelationSummary []MedicalKGRelationSummary `json:"relation_summary"`
}

// MedicalDialogue represents a patient-doctor dialogue seed.
type MedicalDialogue struct {
	Instruction string `json:"instruction"`
	Input       string `json:"input"`
	Output      string `json:"output"`
}

// MedicalDialogueSet wraps a collection of medical dialogues.
type MedicalDialogueSet struct {
	Dialogues []MedicalDialogue `json:"dialogues"`
}

// DiseaseEncyclopedia represents a comprehensive disease entry from CMeKG.
type DiseaseEncyclopedia struct {
	ID                    string   `json:"id"`
	NameZH                string   `json:"name_zh"`
	Description           string   `json:"description"`
	Category              []string `json:"category"`
	Prevention            string   `json:"prevention"`
	Etiology              string   `json:"etiology"`
	Symptoms              []string `json:"symptoms"`
	HighRiskGroups        string   `json:"high_risk_groups"`
	TransmissionRoute     string   `json:"transmission_route"`
	Complications         []string `json:"complications"`
	TreatmentDepartments  []string `json:"treatment_departments"`
	TreatmentMethods      []string `json:"treatment_methods"`
	TreatmentDuration     string   `json:"treatment_duration"`
	CureRate              string   `json:"cure_rate"`
	CostEstimate          string   `json:"cost_estimate"`
	DiagnosticTests       []string `json:"diagnostic_tests"`
	CommonDrugs           []string `json:"common_drugs"`
	RecommendedDrugs      []string `json:"recommended_drugs"`
	RecommendedFoods      []string `json:"recommended_foods"`
	FoodsToAvoid          []string `json:"foods_to_avoid"`
	FoodRecipes           []string `json:"food_recipes"`
	IncidenceRate         string   `json:"incidence_rate"`
	InsuranceStatus       string   `json:"insurance_status"`
	Source                string   `json:"source"`
}

// DiseaseEncyclopediaSet wraps a collection of disease encyclopedias.
type DiseaseEncyclopediaSet struct {
	Diseases    []DiseaseEncyclopedia `json:"diseases"`
	TotalCount  int                   `json:"total_count"`
}

// CPubMedTriple represents a medical knowledge triple from CPubMed-KG.
type CPubMedTriple struct {
	Head      string `json:"head"`
	Relation  string `json:"relation"`
	Tail      string `json:"tail"`
	TripleID  string `json:"triple_id"`
}

// CPubMedKGSet wraps CPubMed-KG triples.
type CPubMedKGSet struct {
	Triples     []CPubMedTriple `json:"triples"`
	TotalCount  int             `json:"total_count"`
	DiseaseCount int            `json:"disease_count"`
	Diseases    []string        `json:"diseases"`
}

// SIDERDrug represents a drug entry from SIDER with side effects and indications.
type SIDERDrug struct {
	ID           string   `json:"id"`
	SideEffects  []string `json:"side_effects"`
	Indications  []string `json:"indications"`
}

// SIDERDataSet wraps SIDER drug data.
type SIDERDataSet struct {
	Source      string      `json:"source"`
	Description string      `json:"description"`
	DrugCount   int         `json:"drug_count"`
	Drugs       []SIDERDrug `json:"drugs"`
}
