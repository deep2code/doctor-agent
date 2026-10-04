package knowledge

// MedlinePlusEntry is one page of MedlinePlus (US National Library of
// Medicine consumer health encyclopedia), English full text.
type MedlinePlusEntry struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// MedlinePlusResult is a retrieved page with its relevance score.
type MedlinePlusResult struct {
	Entry MedlinePlusEntry `json:"entry"`
	Score float64          `json:"score"`
}
