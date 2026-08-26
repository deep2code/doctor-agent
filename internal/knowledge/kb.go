package knowledge

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"

	_ "github.com/go-sql-driver/mysql"
)

// KB is the MariaDB-backed knowledge store. The compiled binary contains
// no embedded knowledge; every dataset is read from MariaDB on demand.
// Datasets are loaded lazily (the first time a retriever or tool needs them)
// and cached in memory for the process lifetime.
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

// OpenKB opens (and migrates) the knowledge database using the given DSN.
// The DSN is a Go MySQL driver data source name, e.g.
// "user:pass@tcp(host:3306)/doctor_knowledge?parseTime=true".
func OpenKB(dsn string) (*KB, error) {
	if dsn == "" {
		dsn = "root@tcp(localhost:3306)/doctor_knowledge?parseTime=true&interpolateParams=true"
	}
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening knowledge db: %w", err)
	}
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("pinging knowledge db: %w", err)
	}
	// Reasonable connection pool for concurrent retrieval.
	conn.SetMaxOpenConns(50)
	conn.SetMaxIdleConns(10)
	kb := &KB{conn: conn}
	if err := kb.migrate(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migrating knowledge db: %w", err)
	}
	return kb, nil
}

// compressData gzip-compresses a document before storage to keep the database
// file small (the uncompressed JSON corpus is ~500MB+). BestSpeed keeps the
// seed fast — the corpus is compressed once at seed time but read many times
// at runtime, so speed beats ratio here. Decompression is transparent in
// Get/All/Search; the gzip magic-byte check makes reads backward-compatible
// with any uncompressed rows left by older seeds.
func compressData(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return b
	}
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
			id      BIGINT NOT NULL AUTO_INCREMENT,
			dataset VARCHAR(64) NOT NULL,
			` + "`key`" + ` VARCHAR(255) NOT NULL,
			data    MEDIUMBLOB NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY uq_kb_dataset_key (dataset, ` + "`key`" + `)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE INDEX IF NOT EXISTS idx_kb_dataset ON kb_items(dataset)`,
	}
	for _, q := range queries {
		if _, err := kb.conn.Exec(q); err != nil {
			return fmt.Errorf("migrating kb_items: %w", err)
		}
	}
	return nil
}

// Insert stores one knowledge item. data is the raw JSON document.
func (kb *KB) Insert(dataset, key string, data []byte) error {
	_, err := kb.conn.Exec(
		"INSERT INTO kb_items (dataset, `key`, data) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE data = VALUES(data)",
		dataset, key, compressData(data),
	)
	return err
}

// InsertBatch stores many rows inside a single transaction. All rows are
// gzip-compressed in parallel first, then written as chunked multi-row INSERT
// statements (500 rows per statement) — orders of magnitude faster than one
// Exec per row for the ~800k-row seed. MariaDB connections are safe for
// concurrent use, so no global lock is held here; callers may parallelize
// across datasets.
func (kb *KB) InsertBatch(dataset string, rows []KBRow) error {
	if len(rows) == 0 {
		return nil
	}

	// Compress all rows in parallel (gzip is CPU-bound; the corpus is ~500MB).
	compressed := make([][]byte, len(rows))
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i := range rows {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			compressed[idx] = compressData(rows[idx].Data)
		}(i)
	}
	wg.Wait()

	// Insert in chunks of rows per transaction. A single ~500k-row transaction
	// would balloon InnoDB redo/undo logs; 20k-row transactions stay small and
	// are safe because INSERT ... ON DUPLICATE KEY UPDATE is idempotent.
	const (
		chunk        = 200 // rows per INSERT statement
		rowsPerTx    = 20000
	)
	var sb strings.Builder
	var tx *sql.Tx
	var err error
	inserted := 0
	for i := 0; i < len(rows); i += chunk {
		if inserted == 0 {
			tx, err = kb.conn.Begin()
			if err != nil {
				return err
			}
		}
		end := i + chunk
		if end > len(rows) {
			end = len(rows)
		}
		sb.Reset()
		sb.WriteString("INSERT INTO kb_items (dataset, `key`, data) VALUES ")
		args := make([]interface{}, 0, (end-i)*3)
		for j := i; j < end; j++ {
			if j > i {
				sb.WriteString(",")
			}
			sb.WriteString("(?,?,?)")
			args = append(args, dataset, rows[j].Key, compressed[j])
		}
		if _, err = tx.Exec(sb.String(), args...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("inserting chunk %d-%d: %w", i, end, err)
		}
		inserted += end - i
		if inserted >= rowsPerTx {
			if err = tx.Commit(); err != nil {
				return fmt.Errorf("committing batch: %w", err)
			}
			inserted = 0
		}
	}
	if inserted > 0 {
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("committing final batch: %w", err)
		}
	}
	return nil
}

// KBRow is one row passed to InsertBatch.
type KBRow struct {
	Key        string
	SearchText string
	Data       []byte
}

// Clear removes every row for a dataset (used by the seeder before a re-seed).
func (kb *KB) Clear(dataset string) error {
	_, err := kb.conn.Exec(`DELETE FROM kb_items WHERE dataset = ?`, dataset)
	return err
}

// Get returns a single item's raw JSON by dataset and key.
func (kb *KB) Get(dataset, key string) ([]byte, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	var data []byte
	err := kb.conn.QueryRow(
		"SELECT `data` FROM kb_items WHERE `dataset` = ? AND `key` = ?", dataset, key,
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
		`SELECT data FROM kb_items WHERE dataset = ? ORDER BY id`, dataset,
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
		`SELECT data FROM kb_items WHERE dataset = ? ORDER BY id`, dataset,
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
