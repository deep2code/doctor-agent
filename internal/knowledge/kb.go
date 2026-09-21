package knowledge

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
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
	DSBodyPart         = "bodypart"
	DSGrowth           = "growth"
	DSMilestones       = "milestones"
	DSNewborn          = "newborn"
	DSCorpus           = "corpus" // unified medkb corpora (corpus_<source>.json)
	DSICD11            = "icd11"
	DSHPO              = "hpo"      // Human Phenotype Ontology terms (en + zh names)
	DSOrphanet         = "orphanet" // Orphanet rare diseases (zh + ORPHA code, ICD-10/11 maps)
	DSICDO3            = "icdo3"    // ICD-O-3 tumor morphology codes
	DSVersion          = "version"

	// 中国医学数据集
	DSChinaStats            = "china_stats"             // 卫生统计年鉴
	DSChinaClinicalPathways = "china_clinical_pathways" // 临床路径
	DSChinaCDC              = "china_cdc"               // 法定传染病
	DSChinaTCM              = "china_tcm"               // 中医药知识库
	DSChinaCSO              = "china_cso"               // CSCO肿瘤指南
	DSChinaDietary          = "china_dietary"           // 膳食指南

	// 公共医学资源
	DSPublicResources = "public_resources" // 公共医学资料库（教科书、视频、科普等）
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
		return nil, fmt.Errorf("reading gzip payload: %w", err)
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

// ErrNotSeeded is returned by Health when the knowledge database answers but
// holds no rows: retrieval would then silently return nothing at runtime.
var ErrNotSeeded = errors.New("knowledge base is empty (run: go run . seed-knowledge)")

// Health pings the knowledge database and checks that kb_items holds at least
// one row. It deliberately avoids COUNT(*) — the table carries over a million
// rows, and /health is polled by orchestrators every few seconds.
func (kb *KB) Health(ctx context.Context) error {
	kb.mu.RLock()
	conn := kb.conn
	kb.mu.RUnlock()
	if err := conn.PingContext(ctx); err != nil {
		return err
	}
	var one int
	err := conn.QueryRowContext(ctx, `SELECT 1 FROM kb_items LIMIT 1`).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotSeeded
	}
	return err
}

func (kb *KB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS kb_items (
			id      BIGINT NOT NULL AUTO_INCREMENT,
			dataset VARCHAR(64) NOT NULL COLLATE utf8mb4_bin,
			` + "`key`" + ` VARCHAR(255) NOT NULL COLLATE utf8mb4_bin,
			data    MEDIUMBLOB NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY uq_kb_dataset_key (dataset, ` + "`key`" + `)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE INDEX idx_kb_dataset ON kb_items(dataset)`,
	}
	for _, q := range queries {
		if _, err := kb.conn.Exec(q); err != nil {
			if isDuplicateIndexError(err) {
				continue
			}
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

	// Insert in transactions of rowsPerTx rows, each built from chunk-sized
	// multi-row statements. A single ~500k-row transaction would balloon
	// InnoDB redo/undo logs; 20k-row transactions stay small.
	const (
		chunk     = 200 // rows per INSERT statement
		rowsPerTx = 20000
	)
	for start := 0; start < len(rows); start += rowsPerTx {
		end := start + rowsPerTx
		if end > len(rows) {
			end = len(rows)
		}
		if err := kb.insertTx(dataset, rows[start:end], compressed[start:end], chunk); err != nil {
			return err
		}
	}
	return nil
}

// insertTx writes rows in chunk-sized upserts inside one transaction, replaying
// the whole transaction when InnoDB drops it on a lock conflict. Concurrent
// seed workers contend on kb_items' indexes (one dataset's DELETE range against
// another's duplicate-key gap locks), so error 1213 is a transient conflict
// rather than bad data — and because the statement is an idempotent upsert
// (INSERT ... ON DUPLICATE KEY UPDATE), re-running an aborted transaction is
// safe even if part of it had already been written.
func (kb *KB) insertTx(dataset string, rows []KBRow, compressed [][]byte, chunk int) error {
	const maxAttempts = 5
	for attempt := 1; ; attempt++ {
		err := kb.runInsertTx(dataset, rows, compressed, chunk)
		if err == nil {
			return nil
		}
		if !isRetryableLockError(err) || attempt >= maxAttempts {
			return err
		}
		slog.Warn("knowledge seed hit a lock conflict, replaying transaction",
			"dataset", dataset, "attempt", attempt, "rows", len(rows), "error", err)
		time.Sleep(time.Duration(attempt*attempt) * 100 * time.Millisecond)
	}
}

func (kb *KB) runInsertTx(dataset string, rows []KBRow, compressed [][]byte, chunk int) error {
	var sb strings.Builder
	tx, err := kb.conn.Begin()
	if err != nil {
		return err
	}
	for i := 0; i < len(rows); i += chunk {
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
		// Idempotent upsert: a re-run of a partially seeded dataset (or a
		// full-width/half-width key collision folded by a legacy ci collation)
		// updates the row instead of aborting the batch.
		if _, err = tx.Exec(sb.String()+" ON DUPLICATE KEY UPDATE data = VALUES(data)", args...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("inserting chunk %d-%d: %w", i, end, err)
		}
	}
	if err = tx.Commit(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("committing batch: %w", err)
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

// isDuplicateIndexError reports whether err is MySQL/MariaDB error 1061
// (ER_DUP_KEYNAME) — raised when CREATE INDEX targets an existing index.
// Kept in this package (rather than shared) to avoid a knowledge→database
// dependency; the check is a single error-number comparison.
func isDuplicateIndexError(err error) bool {
	if err == nil {
		return false
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1061
	}
	return false
}

// isRetryableLockError reports whether err is MariaDB's transient lock conflict
// (1213 ER_LOCK_DEADLOCK, 1205 ER_LOCK_WAIT_TIMEOUT) — both mean "you were
// picked as the victim, replay the transaction".
func isRetryableLockError(err error) bool {
	if err == nil {
		return false
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1213 || me.Number == 1205
	}
	return false
}
