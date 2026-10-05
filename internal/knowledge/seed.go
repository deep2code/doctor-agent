package knowledge

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Seed reads every knowledge archive under gzDir/<dataset>/ and inserts its
// contents into the MariaDB knowledge database at dbPath. The compiled binary
// embeds nothing; this command (run at build/release time) materialises the
// data into the MariaDB doctor_knowledge database.
//
// The directory name is the dataset; each file is a top-level JSON array and
// each element becomes one row keyed by a stable id (or running index), with a
// lower-cased search_text column used for candidate filtering at query time.
// The directory is authoritative in both directions: after a successful pass the
// database holds exactly the datasets the tree ships, so gzDir must be the whole
// knowledge package, never a subset — a partial source would clear what it omits.
func Seed(dbPath, gzDir string) error {
	kb, err := OpenKB(dbPath)
	if err != nil {
		return err
	}
	defer kb.Close()

	archives, err := listSeedArchives(gzDir)
	if err != nil {
		return err
	}

	// Accumulate rows per dataset: several source files share one dataset
	// (e.g. every file under medical/ maps to the medical dataset), so we must
	// NOT clear the dataset between files. Collect everything first, then clear
	// each dataset once and bulk-insert.
	byDataset := make(map[string][]KBRow)
	for _, a := range archives {
		rows, err := a.rows()
		if err != nil {
			return fmt.Errorf("seeding: %w", err)
		}
		byDataset[a.Dataset] = append(byDataset[a.Dataset], rows...)
	}

	// Seed datasets in parallel (bounded worker pool). Each dataset is
	// independent, so parallelism is safe and cuts wall time on the large
	// tables (medicalqa 506k, nmpa 167k, cpubmed 105k rows).
	const seedWorkers = 4
	datasets := make([]string, 0, len(byDataset))
	for ds := range byDataset {
		datasets = append(datasets, ds)
	}
	sort.Strings(datasets)
	for _, ds := range datasets {
		dedupeDatasetKeys(byDataset[ds])
	}

	// Wipe every dataset first, sequentially. A DELETE range lock on
	// idx_kb_dataset deadlocks against the duplicate-key gap locks another
	// worker's INSERT takes on the shared unique index, so clears must not
	// overlap the parallel inserts below. Consequence: if a later insert fails
	// the knowledge DB is left partly empty — re-run seed-knowledge rather than
	// trusting it (insertTx retries transient lock conflicts, so this is rare).
	for _, ds := range datasets {
		if err := kb.Clear(ds); err != nil {
			return fmt.Errorf("clearing %s: %w", ds, err)
		}
	}

	// The tree is the whole classification, so it also decides what is *not*
	// knowledge any more: a dataset directory that was deleted keeps its rows
	// forever unless the seed pass drops them, and a dump taken from such a
	// database would publish that dead data to every deployment.
	stored, err := kb.ListDatasets()
	if err != nil {
		return fmt.Errorf("listing stored datasets: %w", err)
	}
	inTree := make(map[string]bool, len(datasets))
	for _, ds := range datasets {
		inTree[ds] = true
	}
	for _, ds := range stored {
		if inTree[ds] {
			continue
		}
		if err := kb.Clear(ds); err != nil {
			return fmt.Errorf("clearing removed dataset %s: %w", ds, err)
		}
		slog.Info("dropped a dataset the seed tree no longer ships", "dataset", ds)
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		werr error
	)
	sem := make(chan struct{}, seedWorkers)
	for _, ds := range datasets {
		rows := byDataset[ds]
		wg.Add(1)
		sem <- struct{}{}
		go func(ds string, rows []KBRow) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := kb.InsertBatch(ds, rows); err != nil {
				mu.Lock()
				if werr == nil {
					werr = fmt.Errorf("inserting %s: %w", ds, err)
				}
				mu.Unlock()
				return
			}
			slog.Info("seed: dataset done", "dataset", ds, "rows", len(rows))
		}(ds, rows)
	}
	wg.Wait()
	return werr
}

// seedList builds rows from a source file: a top-level JSON array whose
// elements are stored verbatim, one element per row. This is the only parse
// mode the seed tree has — a file that is not a non-empty array is a data
// error, not a different kind of source.
func seedList(raw []byte) ([]KBRow, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("not a JSON array of documents: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("JSON array has no documents")
	}
	rows := make([]KBRow, 0, len(items))
	seen := make(map[string]int, len(items))
	for i, it := range items {
		key := dedupeKey(seen, extractKey(it, i))
		rows = append(rows, KBRow{Key: key, SearchText: buildSearchText(it), Data: it})
	}
	return rows, nil
}

// dedupeKey appends a running suffix when the same key was already produced
// (e.g. FDA labels sharing one generic name), keeping the unique index happy.
func dedupeKey(seen map[string]int, key string) string {
	n := seen[key]
	seen[key] = n + 1
	if n == 0 {
		return key
	}
	return fmt.Sprintf("%s-%d", key, n+1)
}

// dedupeDatasetKeys resolves key collisions ACROSS source files of the same
// dataset: dedupeKey is per-file, so fallback keys like "idx-0" repeat when
// several files feed one dataset. Without this pass the unique index plus
// INSERT ... ON DUPLICATE KEY UPDATE would silently drop the colliding rows.
func dedupeDatasetKeys(rows []KBRow) {
	seen := make(map[string]bool, len(rows))
	for i := range rows {
		if !seen[rows[i].Key] {
			seen[rows[i].Key] = true
			continue
		}
		for n := 2; ; n++ {
			cand := fmt.Sprintf("%s~%d", rows[i].Key, n)
			if !seen[cand] {
				rows[i].Key = cand
				seen[cand] = true
				break
			}
		}
	}
}

// extractKeyFields and searchTextKeys are the field priority lists used to turn
// a document into a row key and into the text that gets vectorised. They are
// package-level because the Python bake mirror (external/bake_onnx.py) keeps a
// copy of both, and TestBakeMirrorMatchesGoSeedRules compares them.
var extractKeyFields = []string{
	"id", "ID", "clinvar_id", "icd10_code", "hpo_id", "orpha_code",
	"code", "Code", "name", "Name", "name_zh", "NameZH",
	"title", "Title", "variation", "Variation",
}

var searchTextKeys = []string{
	"id", "ID", "code", "Code", "hpo_id", "orpha_code", "name_en", "icd10", "icd11", "behavior",
	"title", "Title", "name", "Name", "name_zh", "NameZH",
	"question", "Question", "answer", "Answer", "keywords", "Keywords",
	"symptoms", "Symptoms", "content", "Content", "gene", "Gene",
	"disease", "Disease", "relation", "Relation", "category", "Category",
	"department", "Department", "description", "Description",
	"definition", "Definition", "head", "Head", "entity1", "Entity1",
	"entity2", "Entity2", "variation", "Variation", "synonyms", "Synonyms",
	"part_key", "PartKey", "part_zh", "PartZH", "aliases", "Aliases",
	"conditions", "Conditions", "red_flags", "RedFlags", "self_care", "SelfCare",
	"departments", "Departments",
	// 科普 batches hold their article prose in these keys (see KnowledgeEntry);
	// without them the baked vectors for ~6000 medical rows are keywords-only.
	"title_zh", "summary_zh", "details_zh", "body",
}

// extractKey derives a stable row key from a JSON document, preferring common
// id/code/name fields, falling back to the element index.
func extractKey(raw json.RawMessage, idx int) string {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Sprintf("idx-%d", idx)
	}
	for _, k := range extractKeyFields {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return fmt.Sprintf("idx-%d", idx)
}

// buildSearchText produces a lower-cased, whitespace-joined search string from
// the document's string/array-of-string fields, with a curated set of keys
// that matter for retrieval (title, name, keywords, symptoms, content, …).
func buildSearchText(raw []byte) string {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return strings.ToLower(string(raw))
	}
	var b strings.Builder
	for _, k := range searchTextKeys {
		if v, ok := m[k]; ok {
			b.WriteString(valueToString(v))
			b.WriteString(" ")
		}
	}
	// A present-but-empty key (e.g. "title_zh": "") contributes only padding, so
	// the guard must test for content, not for bytes written — otherwise those
	// rows lose the raw-JSON fallback and end up with no candidate text at all.
	if len(strings.TrimSpace(b.String())) == 0 {
		return strings.ToLower(string(raw))
	}
	return strings.ToLower(b.String())
}

func valueToString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, valueToString(e))
		}
		return strings.Join(parts, " ")
	case float64:
		return fmt.Sprintf("%v", t)
	case bool:
		return fmt.Sprintf("%v", t)
	default:
		return ""
	}
}

// IngestUpload replaces the rows of one dataset with an uploaded knowledge file
// (JSON, or gzip-compressed JSON). Powers the admin upload API. The file name IS
// the dataset — the same rule the seed tree uses, where the directory name is
// the dataset — so `medical.json` replaces the medical dataset and
// `medical.json.gz` does the same with a compressed body. The body must be a
// top-level JSON array; each element becomes one row.
//
// Only a dataset that already has rows can be replaced: that is what stops a
// mistyped file name from quietly creating an orphan dataset nothing reads.
// Permanent additions need a new image (make_gz → seed → build.sh kb), not an
// upload — the kb container's storage is throwaway by design.
func IngestUpload(dsn, filename string, raw []byte) (string, int, error) {
	data := raw
	// Transparently decompress .gz uploads.
	if strings.HasSuffix(filename, ".gz") || strings.HasSuffix(filename, ".gzip") {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return "", 0, fmt.Errorf("not a valid gzip file: %w", err)
		}
		defer zr.Close()
		if data, err = io.ReadAll(zr); err != nil {
			return "", 0, fmt.Errorf("decompressing upload: %w", err)
		}
		filename = strings.TrimSuffix(filename, filepath.Ext(filename))
	}

	ds := DatasetFromFileName(filename)
	if !validDatasetName(ds) {
		return "", 0, fmt.Errorf("upload must be named <dataset>.json or <dataset>.json.gz (got %q)", filename)
	}
	rows, err := seedList(data)
	if err != nil {
		return "", 0, fmt.Errorf("parsing %s: %w", ds, err)
	}

	kb, err := OpenKB(dsn)
	if err != nil {
		return "", 0, err
	}
	defer kb.Close()
	ok, err := kb.HasDataset(ds)
	if err != nil {
		return "", 0, err
	}
	if !ok {
		return "", 0, fmt.Errorf("unknown dataset %q: uploads replace an existing dataset only", ds)
	}
	if err := kb.Clear(ds); err != nil {
		return "", 0, fmt.Errorf("clearing %s: %w", ds, err)
	}
	if err := kb.InsertBatch(ds, rows); err != nil {
		return "", 0, fmt.Errorf("inserting %s: %w", ds, err)
	}
	return ds, len(rows), nil
}

// DatasetForSeedPath applies the seed tree's naming rule to a path on disk: the
// directory holding the file IS the dataset.
func DatasetForSeedPath(path string) string {
	return filepath.Base(filepath.Dir(path))
}

// DatasetFromFileName is the upload-side stand-in for the seed tree's directory
// name: an uploaded file carries no path, so its name has to do the registering
// the dataset directory does on disk. `medical.json.gz` and `medical.json` both
// name the medical dataset; anything else fails validDatasetName downstream.
func DatasetFromFileName(name string) string {
	base := filepath.Base(name)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return strings.TrimSuffix(base, ".json")
}

// validDatasetName guards the dataset name that comes straight from an uploaded
// file name: it is used in SQL parameters and (via the seed tree) as a directory
// name, so it stays in the character set the dataset constants actually use.
func validDatasetName(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// DatasetStats returns the row count per dataset in the knowledge store,
// used by the admin dashboard to show what is currently loaded.
func DatasetStats(dsn string) (map[string]int, error) {
	kb, err := OpenKB(dsn)
	if err != nil {
		return nil, err
	}
	defer kb.Close()
	rows, err := kb.conn.Query(
		"SELECT dataset, COUNT(*) FROM kb_items GROUP BY dataset ORDER BY dataset")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := make(map[string]int)
	for rows.Next() {
		var ds string
		var n int
		if err := rows.Scan(&ds, &n); err != nil {
			return nil, err
		}
		stats[ds] = n
	}
	return stats, rows.Err()
}

// ExportDataset exports all items from a dataset as a JSON array.
func ExportDataset(dsn, dataset string) ([]byte, error) {
	kb, err := OpenKB(dsn)
	if err != nil {
		return nil, err
	}
	defer kb.Close()
	rows, err := kb.conn.Query(
		"SELECT `key`, `data` FROM kb_items WHERE dataset = ? ORDER BY id", dataset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []map[string]any
	for rows.Next() {
		var key string
		var data []byte
		if err := rows.Scan(&key, &data); err != nil {
			return nil, err
		}
		// Decompress gzip if needed
		if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
			gz, err := gzip.NewReader(bytes.NewReader(data))
			if err == nil {
				decompressed, err := io.ReadAll(gz)
				gz.Close()
				if err == nil {
					data = decompressed
				}
			}
		}
		var item map[string]any
		if err := json.Unmarshal(data, &item); err != nil {
			continue // skip malformed entries
		}
		items = append(items, item)
	}
	return json.MarshalIndent(items, "", "  ")
}
