package knowledge

// NHCGuide is one official 国家卫健委 (National Health Commission) 诊疗方案/指南
// full text, Chinese. Text extracts come from external/nhc/guides (nhc) and
// guides_ocr (nhc_ocr, scanned PDFs OCR'd with my-ocr; those have no URL).
type NHCGuide struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Year    string `json:"year,omitempty"`
	Content string `json:"content"`
	Source  string `json:"source"` // "nhc" | "nhc_ocr"
}

// NHCGuideSet is the embedded NHC guideline corpus.
type NHCGuideSet struct {
	Source  string      `json:"source"`
	Updated string      `json:"updated"`
	Entries []NHCGuide  `json:"entries"`
}

// NHCGuideResult is a retrieved guideline with its relevance score.
type NHCGuideResult struct {
	Guide NHCGuide `json:"guide"`
	Score float64  `json:"score"`
}
