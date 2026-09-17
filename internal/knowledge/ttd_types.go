package knowledge

// MedicalQAPair represents a medical QA pair from Chinese medical dialogue dataset
type MedicalQAPair struct {
	Department string `json:"department"`
	Title      string `json:"title"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
}

// MedicalQAData is the collection of QA pairs
type MedicalQAData struct {
	QAPairs     []MedicalQAPair   `json:"qa_pairs"`
	TotalCount  int               `json:"total_count"`
	Departments map[string]int    `json:"departments"`
}

// TTDTarget represents a therapeutic target from TTD
type TTDTarget struct {
	ID      string `json:"id"`
	Uniprot string `json:"uniprot"`
	Name    string `json:"name"`
	Type    string `json:"type"`
}

// TTDDrug represents a drug from TTD
type TTDDrug struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Synonyms  []string `json:"synonyms"`
}

// TTDData contains all TTD data
type TTDData struct {
	Targets    []TTDTarget `json:"targets"`
	Drugs      []TTDDrug   `json:"drugs"`
	TargetCount int        `json:"target_count"`
	DrugCount  int         `json:"drug_count"`
}
