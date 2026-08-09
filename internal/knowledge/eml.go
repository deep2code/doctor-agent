package knowledge

// EMLEntry is one medicine from the WHO Model List of Essential Medicines
// (24th list, 2025): INN name, dosage forms, indications, list type.
type EMLEntry struct {
	Name                  string         `json:"name"`   // INN, e.g. "cefotaxime"
	NameZH                string         `json:"name_zh"` // Chinese name (LLM-translated; empty until then)
	Section               string         `json:"section"` // EML section, e.g. "6.2.1 Beta lactam medicines"
	List                  string         `json:"list"`    // "core" | "complementary"
	Forms                 []string       `json:"forms"`   // dosage forms & strengths
	Indications           []EMLIndication `json:"indications"`
	Note                  string         `json:"note"` // WHO footnotes
	Children              bool           `json:"children"` // on the EMLc (children) list
	SquareBox             bool           `json:"square_box"` // square-box listing (therapeutic group)
	TherapeuticAlternatives []string     `json:"therapeutic_alternatives"`
}

// EMLIndication is a first/second-choice indication for an EML medicine.
type EMLIndication struct {
	Choice string `json:"choice"` // "first" | "second" | "both"
	Text   string `json:"text"`
}

// EMLSet is the embedded WHO EML subset.
type EMLSet struct {
	Source  string     `json:"source"`
	URL     string     `json:"url"`
	Updated string     `json:"updated"`
	Entries []EMLEntry `json:"entries"`
}

// EMLResult is a matched EML entry with its score.
type EMLResult struct {
	Entry EMLEntry `json:"entry"`
	Score float64  `json:"score"`
}
