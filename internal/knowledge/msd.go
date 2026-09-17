package knowledge

// MSDEntry is one page of the MSD Manual (默沙东诊疗手册) Chinese edition:
// full Chinese body text with editorial metadata. Source distinguishes the
// consumer (大众版) and professional (专业版) editions. This is a full-text
// retrieval layer (unlike the structured KnowledgeEntry corpus).
type MSDEntry struct {
	URL     string `json:"url"`
	Path    string `json:"path"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Updated string `json:"updated,omitempty"`
	Source  string `json:"source,omitempty"` // "consumer" | "professional"
}

// MSDSet is the embedded MSD Manual corpus.
type MSDSet struct {
	Source  string     `json:"source"`
	Updated string     `json:"updated"`
	Entries []MSDEntry `json:"entries"`
}

// MSDResult is a retrieved MSD page with its relevance score.
type MSDResult struct {
	Entry MSDEntry `json:"entry"`
	Score float64  `json:"score"`
}
