package knowledge

// AAPEntry is one healthychildren.org (American Academy of Pediatrics)
// article, English full text.
type AAPEntry struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// AAPSet is the embedded AAP corpus.
type AAPSet struct {
	Source  string     `json:"source"`
	Updated string     `json:"updated"`
	Entries []AAPEntry `json:"entries"`
}

// AAPResult is a retrieved AAP article with its relevance score.
type AAPResult struct {
	Entry AAPEntry `json:"entry"`
	Score float64  `json:"score"`
}
