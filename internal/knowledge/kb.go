package knowledge

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// KB is the SQLite-backed knowledge store. It replaces the previous
// //go:embed approach: the compiled binary contains no knowledge data; every
// dataset is read from an external SQLite database file (knowledge.db) on
// demand. Datasets are loaded lazily (the first time a retriever or tool
// needs them) and cached in the Store, so cold reads hit the database
// directly while warm reads stay fast.
type KB struct {
	conn *sql.DB
	mu   sync.RWMutex
}

// Dataset identifiers. These mirror the source filenames (without .gz) used by
// the previous embedded loader, so the seeder and the runtime share a stable
// naming scheme.
const (
	DSMedical          = "medical"
	DSDrug             = "drug"
	DSEmergency        = "emergency"
	DSFoodRisk         = "foodrisk"
	DSLabTest          = "labtest"
	DSLiterature       = "literature"
	DSMSD              = "msd"
	DSClinVar          = "clinvar"
	DSMedlinePlus      = "medlineplus"
	DSMedins           = "medins"
	DSEML              = "eml"
	DSFDA              = "fda"
	DSNHC              = "nhc"
	DSFHS              = "fhs"
	DSAAP              = "aap"
	DSHealthMyths      = "healthmyths"
	DSEssential        = "essential"
	DSICD10            = "icd10"
	DSNMPA             = "nmpa"
	DSMedicalKG        = "medkg"
	DSMedicalDialogues = "medicaldialogues"
	DSDiseaseEnc       = "diseaseenc"
	DSCPubMed          = "cpubmed"
	DSHuatuo           = "huatuo"
	DSMedicalQA        = "medicalqa"
	DSTTD              = "ttd"
	DSSIDER            = "sider"
	DSVersion          = "version"
)

// OpenKB opens (and migrates) the knowledge database at the given path.
func OpenKB(path string) (*KB, error) {
	if path == "" {
		path = "knowledge.db"
	}
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening knowledge db: %w", err)
	}
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("pinging knowledge db: %w", err)
	}
	// Tune for bulk seeding throughput and read concurrency.
	for _, p := range []string{
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-131072", // 128 MB page cache
		"PRAGMA busy_timeout=10000",
	} {
		if _, err := conn.Exec(p); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}
	kb := &KB{conn: conn}
	if err := kb.migrate(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migrating knowledge db: %w", err)
	}
	return kb, nil
}

// compressData gzip-compresses a document before storage to keep the database
// file small (the uncompressed JSON corpus is ~500MB+). Decompression is
// transparent in Get/All/Search; the gzip magic-byte check makes reads
// backward-compatible with any uncompressed rows left by older seeds.
func compressData(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(b); err != nil {
		return b
	}
	if err := w.Close(); err != nil {
		return b
	}
	return buf.Bytes()
}

// decompressData reverses compressData. If the bytes are not gzip (no magic
// header), they are returned as-is so pre-compression rows still read.
func decompressData(b []byte) ([]byte, error) {
	if len(b) < 2 || b[0] != 0x1f || b[1] != 0x8b {
		return b, nil
	}
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return b, nil
	}
	defer r.Close()
	return io.ReadAll(r)
}

// Close closes the database connection.
func (kb *KB) Close() error {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	return kb.conn.Close()
}

func (kb *KB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS kb_items (
			dataset TEXT NOT NULL,
			key     TEXT NOT NULL,
			data    BLOB NOT NULL,
			PRIMARY KEY (dataset, key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_kb_dataset ON kb_items(dataset)`,
	}
	for _, q := range queries {
		if _, err := kb.conn.Exec(q); err != nil {
			return fmt.Errorf("migrating kb_items: %w", err)
		}
	}
	return nil
}

// Insert stores one knowledge item. data is the raw JSON document; searchText
// is a lower-cased concatenation of the item's searchable fields used for
// candidate filtering.
func (kb *KB) Insert(dataset, key, searchText string, data []byte) error {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	_, err := kb.conn.Exec(
		`INSERT OR REPLACE INTO kb_items (dataset, key, data) VALUES (?, ?, ?)`,
		dataset, key, compressData(data),
	)
	return err
}

// InsertBatch stores many items inside a single transaction. It is far faster
// than repeated Insert calls for large datasets (e.g. 350k KG triples).
func (kb *KB) InsertBatch(dataset string, rows []KBRow) error {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	tx, err := kb.conn.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO kb_items (dataset, key, data) VALUES (?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(dataset, r.Key, compressData(r.Data)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// KBRow is one row passed to InsertBatch.
type KBRow struct {
	Key        string
	SearchText string
	Data       []byte
}

// Clear removes every row for a dataset (used by the seeder before a re-seed).
func (kb *KB) Clear(dataset string) error {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	_, err := kb.conn.Exec(`DELETE FROM kb_items WHERE dataset = ?`, dataset)
	return err
}

// Get returns a single item's raw JSON by dataset and key.
func (kb *KB) Get(dataset, key string) ([]byte, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	var data []byte
	err := kb.conn.QueryRow(
		`SELECT data FROM kb_items WHERE dataset = ? AND key = ?`, dataset, key,
	).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decompressData(data)
}

// All returns every item's raw JSON for a dataset.
func (kb *KB) All(dataset string) ([][]byte, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	rows, err := kb.conn.Query(
		`SELECT data FROM kb_items WHERE dataset = ? ORDER BY rowid`, dataset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		d, err := decompressData(data)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Search returns candidate raw JSON documents whose searchable text contains any
// of the supplied lower-cased terms. The search_text column is not persisted (it
// would roughly double the database size); instead it is rebuilt in Go from the
// decompressed document. This is used only for the optional vector-retrieval
// candidate path, so a full scan + rebuild per query is acceptable.
func (kb *KB) Search(dataset string, terms []string) ([][]byte, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	rows, err := kb.conn.Query(
		`SELECT data FROM kb_items WHERE dataset = ? ORDER BY rowid`, dataset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lowered := make([]string, len(terms))
	for i, t := range terms {
		lowered[i] = strings.ToLower(t)
	}
	var out [][]byte
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		raw, err := decompressData(data)
		if err != nil {
			return nil, err
		}
		text := buildSearchText(raw)
		hit := false
		for _, t := range lowered {
			if strings.Contains(text, t) {
				hit = true
				break
			}
		}
		if hit {
			out = append(out, raw)
		}
	}
	return out, rows.Err()
}
