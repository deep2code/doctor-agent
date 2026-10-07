package knowledge

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
