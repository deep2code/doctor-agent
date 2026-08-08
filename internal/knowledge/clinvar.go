package knowledge

// ClinVarVariant is one ClinVar entry for the southern-China core genes
// (HBB/HBA1/HBA2/G6PD): variation name, clinical significance, and traits.
type ClinVarVariant struct {
	ClinVarID           string   `json:"clinvar_id"`
	Gene                string   `json:"gene"`
	Variation           string   `json:"variation"`
	Cdna                string   `json:"cdna,omitempty"`
	ClinicalSignificance string  `json:"clinical_significance"`
	Traits              []string `json:"traits,omitempty"`
}

// ClinVarSet is the embedded ClinVar subset.
type ClinVarSet struct {
	Source   string           `json:"source"`
	Updated  string           `json:"updated"`
	Variants []ClinVarVariant `json:"variants"`
}

// ClinVarResult is a matched variant with its relevance score.
type ClinVarResult struct {
	Variant ClinVarVariant `json:"variant"`
	Score   float64        `json:"score"`
}

// geneNames maps ClinVar gene symbols to Chinese/aliases for recall.
var geneNames = map[string][]string{
	"HBB":  {"HBB", "β珠蛋白", "β-珠蛋白", "血红蛋白β", "β地中海贫血基因"},
	"HBA1": {"HBA1", "α珠蛋白1", "α-珠蛋白1", "血红蛋白α1"},
	"HBA2": {"HBA2", "α珠蛋白2", "α-珠蛋白2", "血红蛋白α2"},
	"G6PD": {"G6PD", "葡萄糖-6-磷酸脱氢酶"},
}
