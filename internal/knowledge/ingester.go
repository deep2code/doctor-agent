package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Ingester processes documents and inserts them into the vector store.
type Ingester struct {
	embedder Embedder
	store    *QdrantStore
	chunker  *Chunker
}

// NewIngester creates a knowledge ingestion pipeline.
func NewIngester(embedder Embedder, store *QdrantStore) *Ingester {
	return &Ingester{
		embedder: embedder,
		store:    store,
		chunker:  NewChunker(DefaultChunkerConfig()),
	}
}

// IngestDocument chunks and embeds a single document into the vector store.
func (ing *Ingester) IngestDocument(ctx context.Context, doc *Document) error {
	chunks := ing.chunker.ChunkDocument(doc)
	slog.Info("Ingesting document",
		"doc_id", doc.ID,
		"title", doc.Title,
		"chunks", len(chunks),
	)

	vectors, err := ing.embedder.EmbedBatch(ctx, chunkContents(chunks))
	if err != nil {
		return fmt.Errorf("embed document %s: %w", doc.ID, err)
	}

	items := make([]VectorItem, 0, len(chunks))
	for i, chunk := range chunks {
		payload := map[string]any{
			"document_id":    doc.ID,
			"title":          doc.Title,
			"chunk_index":    chunk.ChunkIndex,
			"total_chunks":   chunk.TotalChunks,
			"source_type":    doc.SourceType,
			"content":        chunk.Content,
			"condition_zh":   doc.Title,      // For compatibility with retriever_vector
			"condition_en":   doc.Title,
			"category":       doc.SourceType,
			"keywords":       chunk.Keywords,
		}

		if doc.DOI != "" {
			payload["doi"] = doc.DOI
		}
		if doc.PMID != "" {
			payload["pmid"] = doc.PMID
		}
		if doc.Year > 0 {
			payload["year"] = float64(doc.Year)
		}
		if doc.EvidenceLevel != "" {
			payload["level"] = doc.EvidenceLevel
		}
		if doc.Journal != "" {
			payload["journal"] = doc.Journal
		}

		// Merge additional metadata
		for k, v := range doc.Metadata {
			payload[k] = v
		}

		items = append(items, VectorItem{
			ID:      chunk.ID,
			Vector:  vectors[i],
			Payload: payload,
		})
	}

	return ing.store.InsertBatch(ctx, items)
}

// IngestDocuments processes multiple documents.
func (ing *Ingester) IngestDocuments(ctx context.Context, docs []*Document) error {
	for _, doc := range docs {
		if err := ing.IngestDocument(ctx, doc); err != nil {
			return fmt.Errorf("ingest %s: %w", doc.ID, err)
		}
	}
	return nil
}

// IngestJSONFile reads a JSON file of KnowledgeEntry objects and ingests them.
// This bridges the existing structured knowledge into the vector store.
func (ing *Ingester) IngestJSONFile(ctx context.Context, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file %s: %w", filePath, err)
	}

	var entries []KnowledgeEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parse file %s: %w", filePath, err)
	}

	docs := make([]*Document, 0, len(entries))
	for _, entry := range entries {
		// Convert KnowledgeEntry to Document for chunking
		content := buildEntryText(&entry)
		doc := &Document{
			ID:            entry.ID,
			Title:         entry.ConditionZH,
			Content:       content,
			SourceType:    "structured_knowledge",
			EvidenceLevel: entry.Citations[0].Level,
			Keywords:      entry.Keywords,
		}
		if len(entry.Citations) > 0 {
			doc.DOI = entry.Citations[0].DOI
			doc.PMID = entry.Citations[0].PMID
			doc.Year = entry.Citations[0].Year
		}
		docs = append(docs, doc)
	}

	return ing.IngestDocuments(ctx, docs)
}

// IngestJSONDir ingests all JSON knowledge files from a directory.
func (ing *Ingester) IngestJSONDir(ctx context.Context, dirPath string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dirPath, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		fullPath := filepath.Join(dirPath, entry.Name())
		slog.Info("Ingesting knowledge file", "path", fullPath)
		if err := ing.IngestJSONFile(ctx, fullPath); err != nil {
			return fmt.Errorf("ingest %s: %w", fullPath, err)
		}
	}

	return nil
}

// buildEntryText creates a text representation of a KnowledgeEntry for embedding.
func buildEntryText(entry *KnowledgeEntry) string {
	var text string

	text += fmt.Sprintf("疾病: %s (%s)\n", entry.ConditionZH, entry.ConditionEN)
	text += fmt.Sprintf("分类: %s\n", entry.Category)
	if entry.ICD10 != "" {
		text += fmt.Sprintf("ICD-10: %s\n", entry.ICD10)
	}
	text += fmt.Sprintf("地区: %v\n", entry.Regions)

	if len(entry.Prevalence) > 0 {
		text += "流行病学:\n"
		for region, prev := range entry.Prevalence {
			text += fmt.Sprintf("  %s: %.1f%%\n", region, prev.Rate*100)
		}
	}

	if entry.Diagnosis != nil {
		text += fmt.Sprintf("诊断: %v\n", entry.Diagnosis.LabTests)
	}

	if len(entry.Treatment) > 0 {
		text += "治疗:\n"
		for _, t := range entry.Treatment {
			text += fmt.Sprintf("  - %s (指征: %s, 证据: %s)\n", t.Method, t.Indication, t.EvidenceLevel)
		}
	}

	text += fmt.Sprintf("关键词: %v\n", entry.Keywords)

	return text
}

// chunkContents extracts content strings from chunks.
func chunkContents(chunks []Chunk) []string {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	return texts
}
