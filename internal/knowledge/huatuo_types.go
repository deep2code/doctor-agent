package knowledge

// HuatuoQA represents a medical QA pair from Huatuo26M-Lite
type HuatuoQA struct {
	ID              int    `json:"id"`
	Question        string `json:"question"`
	Answer          string `json:"answer"`
	Department      string `json:"department"`
	Score           int    `json:"score"`
	RelatedDiseases string `json:"related_diseases"`
}

// HuatuoQAPairs is the collection of QA pairs
type HuatuoQAPairs struct {
	QAPairs    []HuatuoQA      `json:"qa_pairs"`
	TotalCount int             `json:"total_count"`
	Departments map[string]int `json:"departments"`
	TopDiseases map[string]int `json:"top_diseases"`
}
