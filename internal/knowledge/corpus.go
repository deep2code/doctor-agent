package knowledge

// CorpusDoc is the unified document format for new prose corpora (medkb
// pipeline: StatPearls, MedlinePlus Genetics, LactMed, …). One Go type serves
// every future source — adding a corpus means dropping a corpus_<source>.json
// file in data/ and a plugin in external/medkb/plugins/, no Go changes.
// Keep field names in sync with external/medkb/schema.py (CorpusDoc dataclass).
type CorpusDoc struct {
	Source   string            `json:"source"` // statpearls | medgen | lactmed | …
	ID       string            `json:"id"`     // source-unique, e.g. medgen-21631
	Lang     string            `json:"lang"`   // en | zh
	Title    string            `json:"title"`
	Summary  string            `json:"summary"`            // first paragraph(s), retrieval-weighted
	Kind     string            `json:"kind,omitempty"`     // condition | drug | gene | chromosome
	TitleZH  string            `json:"title_zh,omitempty"` // only when a reliable zh mapping exists
	URL      string            `json:"url,omitempty"`
	Sections map[string]string `json:"sections,omitempty"` // treatment/prevention/…
	Keywords []string          `json:"keywords,omitempty"`
	Body     string            `json:"body,omitempty"` // capped full text (24KB default)
}

// CorpusSet is the top-level shape of a corpus_<source>.json data file.
type CorpusSet struct {
	Source  string      `json:"source"`
	Updated string      `json:"updated"`
	Entries []CorpusDoc `json:"entries"`
}

// CorpusResult is a retrieved corpus document with relevance score and a
// body excerpt centered on the first match.
type CorpusResult struct {
	Doc     CorpusDoc `json:"doc"`
	Score   float64   `json:"score"`
	Excerpt string    `json:"excerpt"`
}

// ICD11Term is one WHO ICD-11 MMS entry (zh + en titles, ICD-10 map) for
// exact_lookup. Shape mirrors icd11_terms.json produced by medkb icd11 plugin.
type ICD11Term struct {
	ICD11Code string `json:"icd11_code"`
	TitleZH   string `json:"title_zh,omitempty"`
	TitleEN   string `json:"title_en,omitempty"`
	ICD10Map  string `json:"icd10_map,omitempty"`
	Chapter   string `json:"chapter,omitempty"`
}

// ICD11TermSet is the top-level shape of icd11_terms.json.
type ICD11TermSet struct {
	Source  string      `json:"source"`
	Updated string      `json:"updated"`
	Release string      `json:"release,omitempty"`
	Terms   []ICD11Term `json:"terms"`
}
