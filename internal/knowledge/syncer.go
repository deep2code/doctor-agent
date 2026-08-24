package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/doctor-agent/internal/embedding"
)

// SyncStatus represents the status of a sync operation.
type SyncStatus struct {
	LastSync       time.Time `json:"last_sync"`
	TotalRecords   int       `json:"total_records"`
	SyncedRecords  int       `json:"synced_records"`
	PendingRecords int       `json:"pending_records"`
	Errors         []string  `json:"errors,omitempty"`
	InProgress     bool      `json:"in_progress"`
}

// SyncMetadata stores metadata about synced records for incremental sync.
type SyncMetadata struct {
	Hashes map[string]string `json:"hashes"` // source -> content hash
}

// Syncer manages synchronization between JSON data and vector database.
type Syncer struct {
	store    *Store
	vecStore *VectorStore
	embedder embedding.Provider
	metadata SyncMetadata
	mu       sync.RWMutex
	status   SyncStatus
}

// NewSyncer creates a new knowledge syncer.
func NewSyncer(store *Store, vecStore *VectorStore, embedder embedding.Provider) *Syncer {
	return &Syncer{
		store:    store,
		vecStore: vecStore,
		embedder: embedder,
		metadata: SyncMetadata{
			Hashes: make(map[string]string),
		},
	}
}

// SyncConfig holds sync configuration.
type SyncConfig struct {
	Full      bool   // Full sync (rebuild all vectors)
	Source    string // Specific source to sync (empty = all)
	FilePath  string // Path to JSON file to sync (for file upload)
	BatchSize int    // Batch size for embedding (default: 100)
}

// FullSync performs a complete synchronization of all knowledge to vector database.
func (s *Syncer) FullSync(ctx context.Context, cfg SyncConfig) (*SyncStatus, error) {
	s.store.ensureAll()
	if err := s.vecStore.EnsureCollection(ctx); err != nil {
		return nil, fmt.Errorf("ensuring vector collection: %w", err)
	}
	s.mu.Lock()
	s.status = SyncStatus{InProgress: true}
	s.mu.Unlock()

	start := time.Now()
	slog.Info("Starting full sync", "source", cfg.Source, "file", cfg.FilePath)

	var totalPoints int
	var totalAttempted int
	var errors []string

	// Sync medical entries
	if cfg.Source == "" || cfg.Source == "medical" {
		count, errs := s.syncMedicalEntries(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize // Approximate
		errors = append(errors, errs...)
	}

	// Sync drug entries
	if cfg.Source == "" || cfg.Source == "drugs" {
		count, errs := s.syncDrugEntries(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	// Sync food risk entries
	if cfg.Source == "" || cfg.Source == "food_risk" {
		count, errs := s.syncFoodRiskEntries(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	// Sync lab test entries
	if cfg.Source == "" || cfg.Source == "lab_tests" {
		count, errs := s.syncLabTestEntries(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	// Sync literature entries
	if cfg.Source == "" || cfg.Source == "literature" {
		count, errs := s.syncLiteratureEntries(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	// Sync MSD entries
	if cfg.Source == "" || cfg.Source == "msd" {
		count, errs := s.syncMSDEntries(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	// Sync MedlinePlus entries
	if cfg.Source == "" || cfg.Source == "medlineplus" {
		count, errs := s.syncMedlinePlusEntries(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	// Sync disease encyclopedia entries
	if cfg.Source == "" || cfg.Source == "disease_encyclopedia" {
		count, errs := s.syncDiseaseEncyclopedias(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	// Sync CPubMed KG triples
	if cfg.Source == "" || cfg.Source == "cpubmed_kg" {
		count, errs := s.syncCPubMedKG(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	// Sync Huatuo QA pairs
	if cfg.Source == "" || cfg.Source == "huatuo_qa" {
		count, errs := s.syncHuatuoQA(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	// Sync Medical QA pairs
	if cfg.Source == "" || cfg.Source == "medical_qa" {
		count, errs := s.syncMedicalQA(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	// Sync file if provided
	if cfg.FilePath != "" {
		count, errs := s.syncFile(ctx, cfg.FilePath, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	duration := time.Since(start)

	s.mu.Lock()
	s.status = SyncStatus{
		LastSync:       time.Now(),
		TotalRecords:   totalAttempted,
		SyncedRecords:  totalPoints,
		PendingRecords: 0,
		Errors:         errors,
		InProgress:     false,
	}
	s.mu.Unlock()

	slog.Info("Sync completed",
		"duration", duration,
		"total", totalAttempted,
		"synced", totalPoints,
		"errors", len(errors))

	return &s.status, nil
}

// IncrementalSync performs an incremental sync of changed records.
func (s *Syncer) IncrementalSync(ctx context.Context, cfg SyncConfig) (*SyncStatus, error) {
	s.store.ensureAll()
	if err := s.vecStore.EnsureCollection(ctx); err != nil {
		return nil, fmt.Errorf("ensuring vector collection: %w", err)
	}
	s.mu.Lock()
	s.status = SyncStatus{InProgress: true}
	s.mu.Unlock()

	start := time.Now()
	slog.Info("Starting incremental sync", "source", cfg.Source)

	var totalPoints int
	var totalAttempted int
	var errors []string

	// For incremental sync, we check hashes and only sync changed records
	// This is a simplified version - in production, you'd want to track changes

	// Sync medical entries
	if cfg.Source == "" || cfg.Source == "medical" {
		count, errs := s.syncMedicalEntriesIncremental(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	// Sync drug entries
	if cfg.Source == "" || cfg.Source == "drugs" {
		count, errs := s.syncDrugEntriesIncremental(ctx, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	// Sync file if provided
	if cfg.FilePath != "" {
		count, errs := s.syncFile(ctx, cfg.FilePath, cfg.BatchSize)
		totalPoints += count
		totalAttempted += count + len(errs)*cfg.BatchSize
		errors = append(errors, errs...)
	}

	duration := time.Since(start)

	s.mu.Lock()
	s.status = SyncStatus{
		LastSync:       time.Now(),
		TotalRecords:   totalAttempted,
		SyncedRecords:  totalPoints,
		PendingRecords: 0,
		Errors:         errors,
		InProgress:     false,
	}
	s.mu.Unlock()

	slog.Info("Incremental sync completed",
		"duration", duration,
		"total", totalAttempted,
		"synced", totalPoints,
		"errors", len(errors))

	return &s.status, nil
}

// GetStatus returns the current sync status.
func (s *Syncer) GetStatus() *SyncStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := s.status
	return &status
}

// syncMedicalEntries syncs medical knowledge entries to vector database.
func (s *Syncer) syncMedicalEntries(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	entries := s.store.GetAllMedical()
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, "medical", batch, func(e KnowledgeEntry) string {
			return fmt.Sprintf("%s %s %s", e.ID, e.ConditionZH, e.ConditionEN)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("medical batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("medical upsert batch %d: %v", i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncMedicalEntriesIncremental syncs only changed medical entries.
func (s *Syncer) syncMedicalEntriesIncremental(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	entries := s.store.GetAllMedical()
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		// Check if batch has changed
		batchHash := computeBatchHash(batch)
		sourceKey := fmt.Sprintf("medical_%d", i/batchSize)
		if s.metadata.Hashes[sourceKey] == batchHash {
			continue // No changes
		}

		points, err := embedBatch(ctx, s.embedder, "medical", batch, func(e KnowledgeEntry) string {
			return fmt.Sprintf("%s %s %s", e.ID, e.ConditionZH, e.ConditionEN)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("medical batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("medical upsert batch %d: %v", i/batchSize, err))
			continue
		}

		s.metadata.Hashes[sourceKey] = batchHash
		total += len(points)
	}

	return total, errors
}

// syncDrugEntries syncs drug entries to vector database.
func (s *Syncer) syncDrugEntries(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	entries := s.store.DrugEntries
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, "drug", batch, func(e DrugEntry) string {
			return fmt.Sprintf("%s %s %s", e.ID, e.GenericNameZH, e.GenericNameEN)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("drug batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("drug upsert batch %d: %v", i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncDrugEntriesIncremental syncs only changed drug entries.
func (s *Syncer) syncDrugEntriesIncremental(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	entries := s.store.DrugEntries
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		batchHash := computeBatchHash(batch)
		sourceKey := fmt.Sprintf("drug_%d", i/batchSize)
		if s.metadata.Hashes[sourceKey] == batchHash {
			continue
		}

		points, err := embedBatch(ctx, s.embedder, "drug", batch, func(e DrugEntry) string {
			return fmt.Sprintf("%s %s %s", e.ID, e.GenericNameZH, e.GenericNameEN)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("drug batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("drug upsert batch %d: %v", i/batchSize, err))
			continue
		}

		s.metadata.Hashes[sourceKey] = batchHash
		total += len(points)
	}

	return total, errors
}

// syncFoodRiskEntries syncs food risk entries to vector database.
func (s *Syncer) syncFoodRiskEntries(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	entries := s.store.FoodRiskEntries
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, "food_risk", batch, func(e FoodRiskEntry) string {
			return fmt.Sprintf("%s %s %s", e.ID, e.FoodNameZH, e.FoodNameEN)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("food_risk batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("food_risk upsert batch %d: %v", i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncLabTestEntries syncs lab test entries to vector database.
func (s *Syncer) syncLabTestEntries(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	entries := s.store.LabTestReferences
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, "lab_test", batch, func(e LabTestReference) string {
			return fmt.Sprintf("%s %s %s", e.ID, e.TestNameZH, e.TestNameEN)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("lab_test batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("lab_test upsert batch %d: %v", i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncLiteratureEntries syncs literature entries to vector database.
func (s *Syncer) syncLiteratureEntries(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	entries := s.store.LiteratureArticles
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, "literature", batch, func(e LiteratureEntry) string {
			return fmt.Sprintf("%s %s %s", e.ID, e.Title, e.Abstract)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("literature batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("literature upsert batch %d: %v", i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncMSDEntries syncs MSD entries to vector database.
func (s *Syncer) syncMSDEntries(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	entries := s.store.MSDEntries
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, "msd", batch, func(e MSDEntry) string {
			return fmt.Sprintf("%s %s", e.Title, e.Content)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("msd batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("msd upsert batch %d: %v", i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncMedlinePlusEntries syncs MedlinePlus entries to vector database.
func (s *Syncer) syncMedlinePlusEntries(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	entries := s.store.MedlinePlusEntries
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, "medlineplus", batch, func(e MedlinePlusEntry) string {
			return fmt.Sprintf("%s %s", e.Title, e.Content)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("medlineplus batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("medlineplus upsert batch %d: %v", i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncDiseaseEncyclopedias syncs disease encyclopedia entries to vector database.
func (s *Syncer) syncDiseaseEncyclopedias(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	entries := s.store.DiseaseEncyclopedias
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, "disease_encyclopedia", batch, func(e DiseaseEncyclopedia) string {
			return fmt.Sprintf("%s %s", e.NameZH, e.Description)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("disease_encyclopedia batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("disease_encyclopedia upsert batch %d: %v", i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncCPubMedKG syncs CPubMed KG triples to vector database.
func (s *Syncer) syncCPubMedKG(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	entries := s.store.CPubMedTriples
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, "cpubmed_kg", batch, func(e CPubMedTriple) string {
			return fmt.Sprintf("%s %s %s", e.Head, e.Relation, e.Tail)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("cpubmed_kg batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("cpubmed_kg upsert batch %d: %v", i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncHuatuoQA syncs Huatuo QA pairs to vector database.
func (s *Syncer) syncHuatuoQA(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	if s.store.HuatuoQAPairs == nil {
		return 0, nil
	}

	entries := s.store.HuatuoQAPairs.QAPairs
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, "huatuo_qa", batch, func(e HuatuoQA) string {
			return fmt.Sprintf("%s %s", e.Question, e.Answer)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("huatuo_qa batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("huatuo_qa upsert batch %d: %v", i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncMedicalQA syncs Medical QA pairs to vector database.
func (s *Syncer) syncMedicalQA(ctx context.Context, batchSize int) (int, []string) {
	var errors []string
	if s.store.MedicalQAData == nil {
		return 0, nil
	}

	entries := s.store.MedicalQAData.QAPairs
	if len(entries) == 0 {
		return 0, nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, "medical_qa", batch, func(e MedicalQAPair) string {
			return fmt.Sprintf("%s %s", e.Question, e.Answer)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("medical_qa batch %d: %v", i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("medical_qa upsert batch %d: %v", i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncFile syncs a JSON file to vector database.
func (s *Syncer) syncFile(ctx context.Context, filePath string, batchSize int) (int, []string) {
	var errors []string

	data, err := os.ReadFile(filePath)
	if err != nil {
		errors = append(errors, fmt.Sprintf("reading file %s: %v", filePath, err))
		return 0, errors
	}

	// Determine source type from file name
	source := strings.TrimSuffix(filepath.Base(filePath), ".json")
	source = strings.TrimSuffix(source, ".json")

	// Try to parse as different types
	var count int
	var parseErrors []string

	// Try as medical entries
	var medicalEntries []KnowledgeEntry
	if err := json.Unmarshal(data, &medicalEntries); err == nil && len(medicalEntries) > 0 {
		count, parseErrors = s.syncFileMedical(ctx, medicalEntries, source, batchSize)
		errors = append(errors, parseErrors...)
	} else {
		// Try as drug entries
		var drugEntries []DrugEntry
		if err := json.Unmarshal(data, &drugEntries); err == nil && len(drugEntries) > 0 {
			count, parseErrors = s.syncFileDrugs(ctx, drugEntries, source, batchSize)
			errors = append(errors, parseErrors...)
		} else {
			// Try as literature
			var literatureSet LiteratureSet
			if err := json.Unmarshal(data, &literatureSet); err == nil && len(literatureSet.Articles) > 0 {
				count, parseErrors = s.syncFileLiterature(ctx, literatureSet.Articles, source, batchSize)
				errors = append(errors, parseErrors...)
			} else {
				errors = append(errors, fmt.Sprintf("unsupported file format: %s", filePath))
			}
		}
	}

	return count, errors
}

// syncFileMedical syncs medical entries from file.
func (s *Syncer) syncFileMedical(ctx context.Context, entries []KnowledgeEntry, source string, batchSize int) (int, []string) {
	var errors []string
	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, source, batch, func(e KnowledgeEntry) string {
			return fmt.Sprintf("%s %s %s", e.ID, e.ConditionZH, e.ConditionEN)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s batch %d: %v", source, i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("%s upsert batch %d: %v", source, i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncFileDrugs syncs drug entries from file.
func (s *Syncer) syncFileDrugs(ctx context.Context, entries []DrugEntry, source string, batchSize int) (int, []string) {
	var errors []string
	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, source, batch, func(e DrugEntry) string {
			return fmt.Sprintf("%s %s %s", e.ID, e.GenericNameZH, e.GenericNameEN)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s batch %d: %v", source, i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("%s upsert batch %d: %v", source, i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// syncFileLiterature syncs literature entries from file.
func (s *Syncer) syncFileLiterature(ctx context.Context, entries []LiteratureEntry, source string, batchSize int) (int, []string) {
	var errors []string
	if batchSize <= 0 {
		batchSize = 100
	}

	total := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		points, err := embedBatch(ctx, s.embedder, source, batch, func(e LiteratureEntry) string {
			return fmt.Sprintf("%s %s %s", e.ID, e.Title, e.Abstract)
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s batch %d: %v", source, i/batchSize, err))
			continue
		}

		if err := s.vecStore.Upsert(ctx, points); err != nil {
			errors = append(errors, fmt.Sprintf("%s upsert batch %d: %v", source, i/batchSize, err))
			continue
		}

		total += len(points)
	}

	return total, errors
}

// embedBatch embeds a batch of entries and creates vector points.
func embedBatch[T any](ctx context.Context, embedder embedding.Provider, source string, entries []T, textFunc func(T) string) ([]VectorPoint, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	// Extract text for embedding
	texts := make([]string, len(entries))
	for i, entry := range entries {
		texts[i] = textFunc(entry)
	}

	// Batch embed
	vectors, err := embedder.EmbedBatch(texts)
	if err != nil {
		return nil, fmt.Errorf("embedding batch: %w", err)
	}

	// Create vector points
	points := make([]VectorPoint, len(entries))
	for i, entry := range entries {
		// Generate ID based on source and content hash
		entryJSON, _ := json.Marshal(entry)
		hash := sha256.Sum256(entryJSON)
		id := fmt.Sprintf("%s_%x", source, hash[:8])

		// Create payload with metadata
		payload := map[string]string{
			"source":    source,
			"entry_id":  id,
			"text":      texts[i],
			"timestamp": time.Now().Format(time.RFC3339),
		}

		points[i] = VectorPoint{
			ID:      id,
			Vector:  vectors[i],
			Payload: payload,
		}
	}

	return points, nil
}

// computeBatchHash computes a hash for a batch of entries.
func computeBatchHash[T any](batch []T) string {
	data, _ := json.Marshal(batch)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}
