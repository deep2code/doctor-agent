package knowledge

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ChunkerConfig controls document chunking behavior.
type ChunkerConfig struct {
	MaxChunkSize     int  // Maximum characters per chunk (default: 1500)
	OverlapSize      int  // Overlap characters between adjacent chunks (default: 300)
	RespectParagraphs bool // Split on paragraph boundaries when possible (default: true)
}

// DefaultChunkerConfig returns sensible defaults for medical document chunking.
func DefaultChunkerConfig() ChunkerConfig {
	return ChunkerConfig{
		MaxChunkSize:      1500,
		OverlapSize:       300,
		RespectParagraphs: true,
	}
}

// Chunker splits medical documents into retrievable chunks while
// preserving semantic boundaries and source attribution.
type Chunker struct {
	cfg ChunkerConfig
}

// NewChunker creates a document chunker.
func NewChunker(cfg ChunkerConfig) *Chunker {
	return &Chunker{cfg: cfg}
}

// ChunkDocument splits a document into overlapping chunks.
func (c *Chunker) ChunkDocument(doc *Document) []Chunk {
	paragraphs := c.splitParagraphs(doc.Content)
	chunks := make([]Chunk, 0)

	var currentChunk strings.Builder
	chunkIndex := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// If a single paragraph exceeds max size, split it further
		if utf8.RuneCountInString(para) > c.cfg.MaxChunkSize {
			// Flush current chunk first
			if currentChunk.Len() > 0 {
				chunks = append(chunks, c.buildChunk(doc, currentChunk.String(), chunkIndex))
				chunkIndex++
				currentChunk.Reset()
			}

			// Split long paragraph
			subChunks := c.splitLongText(para)
			for _, sc := range subChunks {
				chunks = append(chunks, c.buildChunk(doc, sc, chunkIndex))
				chunkIndex++
			}
			continue
		}

		// Check if adding this paragraph exceeds max size
		if currentChunk.Len()+utf8.RuneCountInString(para) > c.cfg.MaxChunkSize && currentChunk.Len() > 0 {
			chunks = append(chunks, c.buildChunk(doc, currentChunk.String(), chunkIndex))
			chunkIndex++

			// Start new chunk with overlap from previous
			currentChunk.Reset()
			if c.cfg.OverlapSize > 0 {
				prevText := chunks[len(chunks)-1].Content
				overlap := c.extractOverlap(prevText)
				currentChunk.WriteString(overlap)
				currentChunk.WriteString("\n\n")
			}
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(para)
	}

	// Flush remaining content
	if currentChunk.Len() > 0 {
		chunks = append(chunks, c.buildChunk(doc, currentChunk.String(), chunkIndex))
		chunkIndex++
	}

	// Update total counts
	total := len(chunks)
	for i := range chunks {
		chunks[i].TotalChunks = total
	}

	return chunks
}

func (c *Chunker) splitParagraphs(text string) []string {
	if !c.cfg.RespectParagraphs {
		return []string{text}
	}

	// Split on double newlines (paragraph boundaries)
	raw := strings.Split(text, "\n\n")
	result := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 && strings.TrimSpace(text) != "" {
		result = append(result, strings.TrimSpace(text))
	}
	return result
}

func (c *Chunker) splitLongText(text string) []string {
	var chunks []string
	runes := []rune(text)

	for i := 0; i < len(runes); i += c.cfg.MaxChunkSize - c.cfg.OverlapSize {
		end := i + c.cfg.MaxChunkSize
		if end > len(runes) {
			end = len(runes)
		}

		// Try to break at sentence boundary
		if end < len(runes) {
			// Look back for sentence-ending punctuation
			for j := end; j > i+c.cfg.MaxChunkSize/2; j-- {
				if runes[j-1] == '。' || runes[j-1] == '.' || runes[j-1] == '！' || runes[j-1] == '\n' {
					end = j
					break
				}
			}
		}

		chunks = append(chunks, string(runes[i:end]))
	}

	return chunks
}

func (c *Chunker) extractOverlap(text string) string {
	if c.cfg.OverlapSize <= 0 {
		return ""
	}

	runes := []rune(text)
	if len(runes) <= c.cfg.OverlapSize {
		return text
	}

	return string(runes[len(runes)-c.cfg.OverlapSize:])
}

func (c *Chunker) buildChunk(doc *Document, content string, index int) Chunk {
	return Chunk{
		ID:          fmt.Sprintf("%s-chunk-%d", doc.ID, index),
		DocumentID:  doc.ID,
		Content:     content,
		ChunkIndex:  index,
		TotalChunks: 0, // Set after all chunks created
		Keywords:    doc.Keywords,
	}
}
