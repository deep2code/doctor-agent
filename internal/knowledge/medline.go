package knowledge

// MedlinePlusEntry is one page of MedlinePlus (US National Library of
// Medicine consumer health encyclopedia), English full text.
type MedlinePlusEntry struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// MedlinePlusSet is the embedded MedlinePlus corpus.
type MedlinePlusSet struct {
	Source  string             `json:"source"`
	Updated string             `json:"updated"`
	Entries []MedlinePlusEntry `json:"entries"`
}

// MedlinePlusResult is a retrieved page with its relevance score.
type MedlinePlusResult struct {
	Entry MedlinePlusEntry `json:"entry"`
	Score float64          `json:"score"`
}
