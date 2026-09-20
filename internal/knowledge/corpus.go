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

// HPOTerm is one Human Phenotype Ontology entry (phenotype abnormality) for
// exact_lookup. Shape mirrors hpo_terms.json produced by external/convert_hpo.py.
type HPOTerm struct {
	HPOID      string   `json:"hpo_id"`
	Name       string   `json:"name"`
	NameZH     string   `json:"name_zh,omitempty"`
	Synonyms   []string `json:"synonyms,omitempty"`
	Definition string   `json:"definition,omitempty"`
}

// HPOTermSet is the top-level shape of hpo_terms.json.
type HPOTermSet struct {
	Source  string    `json:"source"`
	Updated string    `json:"updated"`
	Terms   []HPOTerm `json:"terms"`
}

// OrphanetDisease is one Orphanet rare-disease entry (zh name + ORPHA code +
// ICD-10/11 maps) for exact_lookup. Shape mirrors orphanet_diseases.json
// produced by external/convert_orphanet.py.
type OrphanetDisease struct {
	OrphaCode string   `json:"orpha_code"`
	NameZH    string   `json:"name_zh,omitempty"`
	NameEN    string   `json:"name_en,omitempty"`
	Synonyms  []string `json:"synonyms,omitempty"`
	ICD10     []string `json:"icd10,omitempty"`
	ICD11     []string `json:"icd11,omitempty"`
	Type      string   `json:"type,omitempty"`
}

// OrphanetDiseaseSet is the top-level shape of orphanet_diseases.json.
type OrphanetDiseaseSet struct {
	Source   string            `json:"source"`
	Updated  string            `json:"updated"`
	Diseases []OrphanetDisease `json:"diseases"`
}

// ICDO3Morphology is one ICD-O-3 morphology code (neoplasm histology) for
// exact_lookup. Shape mirrors icdo3_morphology.json produced by
// external/convert_icdo3.py.
type ICDO3Morphology struct {
	Code     string   `json:"code"`
	Behavior string   `json:"behavior,omitempty"`
	NameEN   string   `json:"name_en"`
	NameZH   string   `json:"name_zh,omitempty"`
	Synonyms []string `json:"synonyms,omitempty"`
}

// ICDO3MorphologySet is the top-level shape of icdo3_morphology.json.
type ICDO3MorphologySet struct {
	Source  string            `json:"source"`
	Updated string            `json:"updated"`
	Terms   []ICDO3Morphology `json:"terms"`
}
