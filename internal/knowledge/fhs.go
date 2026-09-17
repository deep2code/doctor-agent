package knowledge

// FHSGuide is one 香港卫生署家庭健康服务 (Family Health Service) parenting
// page, Simplified Chinese full text (母乳喂哺/儿童健康/亲子/睡眠/安全等).
type FHSGuide struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Source  string `json:"source"` // "fhs"
}

// FHSGuideSet is the embedded FHS corpus.
type FHSGuideSet struct {
	Source  string     `json:"source"`
	Updated string     `json:"updated"`
	Entries []FHSGuide `json:"entries"`
}

// FHSGuideResult is a retrieved FHS page with its relevance score.
type FHSGuideResult struct {
	Guide FHSGuide `json:"guide"`
	Score float64  `json:"score"`
}
