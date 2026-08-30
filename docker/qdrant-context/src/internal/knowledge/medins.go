package knowledge

// MedinsDrug is one drug from the national medical-insurance drug catalogue
// (国家医保药品目录 2024): Chinese name, insurance category (甲/乙), forms.
type MedinsDrug struct {
	Name     string   `json:"name"`
	Category string   `json:"category"` // 甲 / 乙
	Forms    []string `json:"forms,omitempty"`
}

// MedinsSet is the embedded catalogue subset.
type MedinsSet struct {
	Source  string       `json:"source"`
	Updated string       `json:"updated"`
	Drugs   []MedinsDrug `json:"drugs"`
}

// MedinsResult is a matched drug with its score.
type MedinsResult struct {
	Drug  MedinsDrug `json:"drug"`
	Score float64    `json:"score"`
}
