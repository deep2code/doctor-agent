package knowledge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
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
	Datasets int      `json:"datasets"`
	Points   int      `json:"points"`
	Duration string   `json:"duration"`
	Errors   []string `json:"errors,omitempty"`
	Skipped  []string `json:"skipped,omitempty"` // datasets excluded by vectorSkipDatasets
}

// vectorSkipDatasets lists structured datasets served by dedicated lookup
// tools (medical_kg_lookup / nmpa_drug_lookup / cpubmed_kg_lookup /
// icd10_lookup). Their rows are exact-match entities (KG triples, drug
// records, ICD codes) rather than free text, so vectorizing them adds no
// retrieval value — and they account for ~664k of the 1.37M baked points,
// roughly half the Qdrant image size. The runtime Syncer applies the same
// set so admin syncs cannot re-add them.
var vectorSkipDatasets = map[string]bool{
	DSMedicalKG: true, // 354,766 rows
	DSNMPA:      true, // 167,615 rows
	DSCPubMed:   true, // 105,416 rows
	DSICD10:     true, // 35,862 rows
}

// vectorBakeEligible reports whether a dataset should be vectorized.
func vectorBakeEligible(ds string) bool { return !vectorSkipDatasets[ds] }

// bakePayload builds the point payload stored in Qdrant. Only fields
// retrieval actually consumes are kept: VectorRetriever prefers the
// self-contained "data" JSON and falls back to "entry_id"; admin stats read
// "source" and drug filtering reads "type". The former "text" (a duplicate
// of data) and "timestamp" fields had no consumers and were dropped to
// shrink the baked image.
func bakePayload(dataset, key string, data []byte) map[string]string {
	typ := "knowledge"
	if dataset == DSDrug {
		typ = "drug"
	}
	return map[string]string{
		"source":   dataset,
		"type":     typ,
		"entry_id": key,
		"data":     string(data),
	}
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

	files, err := filepath.Glob(filepath.Join(cfg.GzDir, archiveGlob))
	if err != nil {
		return nil, fmt.Errorf("globbing %s: %w", cfg.GzDir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no knowledge archives found in %s", cfg.GzDir)
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
	var skipped []string
	for _, f := range files {
		base := archiveBaseName(f)
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
		if !vectorBakeEligible(ds) {
			fmt.Printf("  skip %s (lookup-tool covered)\n", base)
			skipped = append(skipped, ds)
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
		Skipped:  skipped,
	}
	slog.Info("Bake finished", "datasets", res.Datasets, "points", res.Points, "duration", res.Duration, "errors", len(res.Errors), "skipped", len(res.Skipped))
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
			points[j] = VectorPoint{
				ID:      id,
				Vector:  vectors[j],
				Payload: bakePayload(dataset, r.Key, r.Data),
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
