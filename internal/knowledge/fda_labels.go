package knowledge

// FDALabelEntry is a curated Chinese drug-knowledge entry derived from an
// FDA label (DailyMed/OpenFDA): indications, contraindications, warnings,
// interactions, adverse reactions, dosage.
type FDALabelEntry struct {
	NameZH           string   `json:"name_zh"`
	NameEN           string   `json:"name_en"`
	RXCUI            string   `json:"rxcui"`
	Category         string   `json:"category,omitempty"`
	Indications      []string `json:"indications,omitempty"`
	Contraindications []string `json:"contraindications,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
	Interactions     []string `json:"interactions,omitempty"`
	AdverseReactions []string `json:"adverse_reactions,omitempty"`
	Dosage           []string `json:"dosage,omitempty"`
	Keywords         []string `json:"keywords"`
	SourceURL        string   `json:"source_url"`
}

// FDALabelSet is the embedded FDA-label subset.
type FDALabelSet struct {
	Source  string          `json:"source"`
	Updated string          `json:"updated"`
	Drugs   []FDALabelEntry `json:"drugs"`
}

// FDALabelResult is a matched FDA-label entry with its score.
type FDALabelResult struct {
	Drug  FDALabelEntry `json:"drug"`
	Score float64       `json:"score"`
}
