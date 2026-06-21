package knowledge

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
)

//go:embed data/*.json
var knowledgeFS embed.FS

// Store holds all loaded medical knowledge entries in memory.
type Store struct {
	mu sync.RWMutex

	MedicalEntries      []KnowledgeEntry
	MedicalByID         map[string]*KnowledgeEntry

	DrugEntries         []DrugEntry
	DrugByID            map[string]*DrugEntry
	DrugByGenericName   map[string]*DrugEntry

	FoodRiskEntries     []FoodRiskEntry
	FoodRiskByID        map[string]*FoodRiskEntry

	EmergencyRules      []EmergencyRule
	LabTestReferences   []LabTestReference
	LabTestByID         map[string]*LabTestReference

	// Reference index for post-verification: citation ID -> title
	ReferenceIndex      map[string]string

	loaded bool
}

var globalStore *Store
var loadOnce sync.Once
var loadErr error

// Load loads all knowledge files from the embedded filesystem.
func Load() (*Store, error) {
	loadOnce.Do(func() {
		globalStore, loadErr = doLoad()
	})
	if loadErr != nil {
		return nil, loadErr
	}
	return globalStore, nil
}

func doLoad() (*Store, error) {
	store := &Store{
		MedicalByID:       make(map[string]*KnowledgeEntry),
		DrugByID:          make(map[string]*DrugEntry),
		DrugByGenericName: make(map[string]*DrugEntry),
		FoodRiskByID:      make(map[string]*FoodRiskEntry),
		LabTestByID:       make(map[string]*LabTestReference),
		ReferenceIndex:    make(map[string]string),
	}

	err := fs.WalkDir(knowledgeFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, err := knowledgeFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		base := filepath.Base(path)
		switch base {
		case "thalassemia.json", "g6pd_deficiency.json",
			"nasopharyngeal_carcinoma.json", "hepatitis_b.json",
			"lactose_intolerance.json", "aldh2_deficiency.json",
			"dengue.json", "fungal_infections.json":
			var entries []KnowledgeEntry
			if err := json.Unmarshal(data, &entries); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range entries {
				e := &entries[i]
				store.MedicalEntries = append(store.MedicalEntries, *e)
				store.MedicalByID[e.ID] = e
				for _, c := range e.Citations {
					key := fmt.Sprintf("%s-cite-%d", e.ID, c.Year)
					store.ReferenceIndex[key] = c.Title
				}
			}

		case "drug_contraindications.json":
			var entries []DrugEntry
			if err := json.Unmarshal(data, &entries); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range entries {
				e := &entries[i]
				store.DrugEntries = append(store.DrugEntries, *e)
				store.DrugByID[e.ID] = e
				store.DrugByGenericName[e.GenericNameEN] = e
				store.DrugByGenericName[e.GenericNameZH] = e
			}

		case "emergency_triage.json":
			var rules []EmergencyRule
			if err := json.Unmarshal(data, &rules); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			store.EmergencyRules = rules
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading knowledge: %w", err)
	}

	store.loaded = true
	return store, nil
}

// GetMedicalByID retrieves a medical knowledge entry by ID.
func (s *Store) GetMedicalByID(id string) *KnowledgeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.MedicalByID[id]
}

// GetDrugByName retrieves a drug entry by generic name (EN or ZH).
func (s *Store) GetDrugByName(name string) *DrugEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.DrugByGenericName[name]; ok {
		return e
	}
	return nil
}

// GetAllMedical returns all medical knowledge entries.
func (s *Store) GetAllMedical() []KnowledgeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]KnowledgeEntry, len(s.MedicalEntries))
	copy(entries, s.MedicalEntries)
	return entries
}

// GetAllDrugs returns all drug entries.
func (s *Store) GetAllDrugs() []DrugEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]DrugEntry, len(s.DrugEntries))
	copy(entries, s.DrugEntries)
	return entries
}

// GetAllEmergencyRules returns all emergency triage rules.
func (s *Store) GetAllEmergencyRules() []EmergencyRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := make([]EmergencyRule, len(s.EmergencyRules))
	copy(rules, s.EmergencyRules)
	return rules
}

// IsLoaded returns whether the knowledge store has been loaded.
func (s *Store) IsLoaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loaded
}

// GetReferenceIndex returns the reference index for post-verification.
func (s *Store) GetReferenceIndex() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx := make(map[string]string, len(s.ReferenceIndex))
	for k, v := range s.ReferenceIndex {
		idx[k] = v
	}
	return idx
}
