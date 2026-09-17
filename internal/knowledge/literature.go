package knowledge

// LiteratureTopic describes one retrieval topic (a disease/focus area) and
// the search terms that map a user query onto its article set.
type LiteratureTopic struct {
	ID       string   `json:"id"`
	NameZH   string   `json:"name_zh"`
	Keywords []string `json:"keywords"`
	Count    int      `json:"count"`
}

// LiteratureEntry is one Europe PMC abstract, carrying real DOI/PMID so the
// agent can cite traceable sources.
type LiteratureEntry struct {
	ID       string `json:"id"`
	Topic    string `json:"topic"`
	Title    string `json:"title"`
	Abstract string `json:"abstract"`
	Journal  string `json:"journal,omitempty"`
	Year     int    `json:"year,omitempty"`
	DOI      string `json:"doi,omitempty"`
	PMID     string `json:"pmid,omitempty"`
}

// LiteratureSet is the embedded literature corpus: topic table + articles.
type LiteratureSet struct {
	Source   string            `json:"source"`
	Updated  string            `json:"updated"`
	Topics   []LiteratureTopic `json:"topics"`
	Articles []LiteratureEntry `json:"articles"`
}

// LiteratureResult is a retrieved article with its relevance score.
type LiteratureResult struct {
	Entry LiteratureEntry `json:"entry"`
	Topic LiteratureTopic `json:"topic"`
	Score float64         `json:"score"`
}
