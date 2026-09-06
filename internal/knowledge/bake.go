package knowledge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
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
	Workers    int    // parallel embedding workers (default 4; 1 = sequential)
	// MaxTextChars truncates each embed text to N runes before sending it to
	// the provider (0 = default 1024). Critical for Ollama: /v1/embeddings
	// pads the whole batch to the longest input, so one long text makes all
	// 500 siblings pay full-length compute. 1024 runes covers bge-m3
	// retrieval-quality context and matches runtime query lengths.
	MaxTextChars int
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
// identical), embeds each row's search text with the injected provider
// (must be the same model the query side uses, e.g. bge-m3) and upserts the
// vector + full entry JSON into Qdrant. The Qdrant
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
	if cfg.MaxTextChars <= 0 {
		cfg.MaxTextChars = 1024
	}

	files, err := filepath.Glob(filepath.Join(cfg.GzDir, archiveGlob))
	if err != nil {
		return nil, fmt.Errorf("globbing %s: %w", cfg.GzDir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no knowledge archives found in %s", cfg.GzDir)
	}

	start := time.Now()
	slog.Info("Starting gz -> Qdrant bake", "gz_dir", cfg.GzDir, "collection", cfg.Collection, "files", len(files), "batch_size", cfg.BatchSize, "workers", cfg.Workers)

	if err := vecStore.EnsureCollection(ctx); err != nil {
		return nil, fmt.Errorf("ensuring collection: %w", err)
	}

	// Sort files for deterministic processing order (point IDs are
	// content-hashed UUIDs so upsert order does not affect the final
	// storage, but a stable order makes build logs reproducible).
	sort.Strings(files)

	// Stream processing: handle one file at a time instead of loading all
	// decompressed data into memory before processing. The previous
	// "load-all-then-parallel" approach held ~743k entries (each with full
	// JSON payload) in a single slice, which caused OOM kills (exit 137)
	// on memory-constrained build hosts (~1.6 GB RAM). Streaming keeps
	// only one dataset's data in memory at any time.
	var (
		datasets int
		total    int
		bakeErrs []string
		skipped  []string
	)

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

		fmt.Printf("  baking %-16s %6d rows (%d batches, %d workers)...\n", ds, len(rows), (len(rows)+cfg.BatchSize-1)/cfg.BatchSize, cfg.Workers)
		n, errs := bakeDataset(ctx, vecStore, embedder, ds, rows, cfg.BatchSize, cfg.Workers, cfg.MaxTextChars)
		total += n
		bakeErrs = append(bakeErrs, errs...)
		datasets++
		fmt.Printf("  baked  %-16s %6d points\n", ds, n)

		// Promptly reclaim the decompressed data before the next file.
		runtime.GC()
	}

	res := &BakeResult{
		Datasets: datasets,
		Points:   total,
		Duration: time.Since(start).Round(time.Millisecond).String(),
		Errors:   bakeErrs,
		Skipped:  skipped,
	}
	slog.Info("Bake finished", "datasets", res.Datasets, "points", res.Points, "duration", res.Duration, "errors", len(res.Errors), "skipped", len(res.Skipped))
	return res, nil
}

// bakeDataset embeds and upserts one dataset's rows. When workers > 1,
// batches are processed by a pool of goroutines sending concurrent embedding
// requests to the provider (e.g. Ollama), dramatically reducing wall time
// for large datasets. Point IDs are content-hashed UUIDs so concurrent
// upsert order does not affect the final storage state.
func bakeDataset(ctx context.Context, vecStore *VectorStore, embedder embedding.Provider, dataset string, rows []KBRow, batchSize, workers, maxTextChars int) (int, []string) {
	var errs []string
	if len(rows) == 0 {
		return 0, nil
	}

	// Sort rows by text length (ascending) before batching.
	//
	// Ollama /v1/embeddings pads every text in the input array to the
	// longest one. A batch mixing 10-char titles with 5000-char articles
	// wastes ~99% of GPU compute on padding tokens. Grouping similar-length
	// texts into the same batch eliminates this waste — typically 3-5x
	// throughput improvement for heterogeneous datasets (e.g. huatuo 177k
	// rows range from 20 to 8000 chars).
	//
	// Point IDs are content-hashed UUIDs, so reordering does not affect the
	// final storage state.
	sort.Slice(rows, func(i, j int) bool {
		return len(rows[i].SearchText) < len(rows[j].SearchText)
	})

	// Pre-slice all batches so workers can index without slicing under lock.
	batches := make([][]KBRow, 0, (len(rows)+batchSize-1)/batchSize)
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batches = append(batches, rows[i:end])
	}

	if workers <= 1 {
		// Sequential path (original behavior, for workers<=1 or debugging).
		total := 0
		nBatches := len(batches)
		for idx, batch := range batches {
			n, msg := bakeBatch(ctx, vecStore, embedder, dataset, idx, batch, maxTextChars)
			if msg != "" {
				errs = append(errs, msg)
			}
			total += n
			if nBatches > 20 && ((idx+1)%10 == 0 || idx+1 == nBatches) {
				fmt.Printf("    progress %-16s %d/%d batches (%.0f%%)\n", dataset, idx+1, nBatches, float64(idx+1)*100/float64(nBatches))
			}
		}
		return total, errs
	}

	// Parallel path: worker pool with concurrent embedding + upsert.
	var (
		mu       sync.Mutex
		total    atomic.Int64
		done     atomic.Int64
		wg       sync.WaitGroup
		nBatches = int64(len(batches))
	)

	batchCh := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range batchCh {
				n, msg := bakeBatch(ctx, vecStore, embedder, dataset, idx, batches[idx], maxTextChars)
				mu.Lock()
				if msg != "" {
					errs = append(errs, msg)
				}
				mu.Unlock()
				total.Add(int64(n))
				d := done.Add(1)
				if nBatches > 20 && (d%10 == 0 || d == nBatches) {
					fmt.Printf("    progress %-16s %d/%d batches (%.0f%%)\n", dataset, d, nBatches, float64(d)*100/float64(nBatches))
				}
			}
		}()
	}
	for i := range batches {
		batchCh <- i
	}
	close(batchCh)
	wg.Wait()

	return int(total.Load()), errs
}

// bakeBatch processes a single batch: embed texts, build points, upsert.
// Embed texts are truncated to maxTextChars runes (rune-safe) before hitting
// the provider — byte slicing would split CJK runes and send invalid UTF-8.
func bakeBatch(ctx context.Context, vecStore *VectorStore, embedder embedding.Provider, dataset string, idx int, batch []KBRow, maxTextChars int) (int, string) {
	texts := make([]string, len(batch))
	for j, r := range batch {
		texts[j] = r.SearchText
		if texts[j] == "" {
			texts[j] = string(r.Data)
		}
		if maxTextChars > 0 {
			if rs := []rune(texts[j]); len(rs) > maxTextChars {
				texts[j] = string(rs[:maxTextChars])
			}
		}
	}
	vectors, err := embedder.EmbedBatch(texts)
	if err != nil {
		return 0, fmt.Sprintf("%s batch %d: %v", dataset, idx, err)
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
		return 0, fmt.Sprintf("%s upsert batch %d: %v", dataset, idx, err)
	}
	return len(points), ""
}
