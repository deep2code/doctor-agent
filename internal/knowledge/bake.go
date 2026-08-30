package knowledge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/doctor-agent/internal/embedding"
)

// BakeConfig controls the offline gz -> Qdrant bake (RAG data image build).
// Unlike runtime sync (which reads MariaDB), Bake reads the gz source files
// directly, so the Qdrant data image is self-contained and MariaDB is only
// needed for business data (users/sessions/messages/feedback).
type BakeConfig struct {
	GzDir      string // source dir of *.json.gz (internal/knowledge/gz)
	Collection string // Qdrant collection (default medical_knowledge)
	BatchSize  int    // embedding batch size (default 100)
	Workers    int    // parallel dataset workers (default 4)
}

// BakeResult summarizes what was written.
type BakeResult struct {
	Datasets int   `json:"datasets"`
	Points   int   `json:"points"`
	Duration string `json:"duration"`
	Errors   []string `json:"errors,omitempty"`
}

// Bake reads every gz knowledge file, classifies it via seedFile (the same
// classification the MariaDB seeder uses, so dataset boundaries stay
// identical), embeds each row's search text offline (local-hash provider by
// default) and upserts the vector + full entry JSON into Qdrant. The Qdrant
// storage produced here is what gets baked into the doctor-agent-qdrant data
// image, making the vector store a complete RAG knowledge source on its own.
func Bake(ctx context.Context, vecStore *VectorStore, embedder embedding.Provider, cfg BakeConfig) (*BakeResult, error) {
	if cfg.Collection == "" {
		cfg.Collection = "medical_knowledge"
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}

	files, err := filepath.Glob(filepath.Join(cfg.GzDir, "*.gz"))
	if err != nil {
		return nil, fmt.Errorf("globbing %s: %w", cfg.GzDir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .gz files found in %s", cfg.GzDir)
	}

	start := time.Now()
	slog.Info("Starting gz -> Qdrant bake", "gz_dir", cfg.GzDir, "collection", cfg.Collection, "files", len(files))

	if err := vecStore.EnsureCollection(ctx); err != nil {
		return nil, fmt.Errorf("ensuring collection: %w", err)
	}

	// Classify every file into rows first (same as seed.go). Each row carries
	// the full entry JSON, so Qdrant payloads are self-contained.
	type fileRows struct {
		dataset string
		rows    []KBRow
	}
	classified := make([]fileRows, 0, len(files))
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".gz")
		raw, err := decompressFile(f)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f, err)
		}
		ds, rows, err := seedFile(base, raw)
		if err != nil {
			return nil, fmt.Errorf("classifying %s: %w", base, err)
		}
		if ds == "" {
			fmt.Printf("  skip %s (unsupported)\n", base)
			continue
		}
		classified = append(classified, fileRows{dataset: ds, rows: rows})
	}

	// Sort so datasets are baked in a deterministic order.
	sort.Slice(classified, func(i, j int) bool { return classified[i].dataset < classified[j].dataset })

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		total     int
		bakeErrs  []string
	)
	sem := make(chan struct{}, cfg.Workers)

	for _, fr := range classified {
		wg.Add(1)
		sem <- struct{}{}
		go func(ds string, rows []KBRow) {
			defer wg.Done()
			defer func() { <-sem }()
			n, errs := bakeDataset(ctx, vecStore, embedder, ds, rows, cfg.BatchSize)
			mu.Lock()
			total += n
			bakeErrs = append(bakeErrs, errs...)
			mu.Unlock()
			fmt.Printf("  baked %-16s %6d points\n", ds, n)
		}(fr.dataset, fr.rows)
	}
	wg.Wait()

	res := &BakeResult{
		Datasets: len(classified),
		Points:   total,
		Duration: time.Since(start).Round(time.Millisecond).String(),
		Errors:   bakeErrs,
	}
	slog.Info("Bake finished", "datasets", res.Datasets, "points", res.Points, "duration", res.Duration, "errors", len(res.Errors))
	return res, nil
}

// bakeDataset embeds and upserts one dataset's rows.
func bakeDataset(ctx context.Context, vecStore *VectorStore, embedder embedding.Provider, dataset string, rows []KBRow, batchSize int) (int, []string) {
	var errs []string
	if len(rows) == 0 {
		return 0, nil
	}
	total := 0
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]

		texts := make([]string, len(batch))
		for j, r := range batch {
			texts[j] = r.SearchText
			if texts[j] == "" {
				texts[j] = string(r.Data)
			}
		}
		vectors, err := embedder.EmbedBatch(texts)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s batch %d: %v", dataset, i/batchSize, err))
			continue
		}

		points := make([]VectorPoint, len(batch))
		for j, r := range batch {
			entryHash := sha256.Sum256(r.Data)
			id := uuidFromSourceHash(dataset+"|"+r.Key, entryHash[:])
			typ := "knowledge"
			if dataset == DSDrug {
				typ = "drug"
			}
			points[j] = VectorPoint{
				ID:     id,
				Vector: vectors[j],
				Payload: map[string]string{
					"source":    dataset,
					"type":      typ,
					"entry_id":  r.Key,
					"text":      texts[j],
					"data":      string(r.Data),
					"timestamp": time.Now().Format(time.RFC3339),
				},
			}
		}

		if err := vecStore.Upsert(ctx, points); err != nil {
			errs = append(errs, fmt.Sprintf("%s upsert batch %d: %v", dataset, i/batchSize, err))
			continue
		}
		total += len(points)
	}
	return total, errs
}
