package knowledge

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed gz/*.gz
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

	// DataVersion describes the knowledge base release and its sources.
	DataVersion         *DataVersion

	// Literature corpus (Europe PMC abstracts) for reference_lookup-style
	// retrieval with real DOI/PMID.
	LiteratureTopics    []LiteratureTopic
	LiteratureArticles  []LiteratureEntry
	LiteratureByTopic   map[string][]*LiteratureEntry

	// MSD Manual (默沙东诊疗手册) Chinese consumer pages, full-text search.
	MSDEntries          []MSDEntry

	// ClinVar subset: pathogenic/likely-pathogenic variants of the core
	// China high-burden genes (HBB/HBA1/HBA2/G6PD).
	ClinVarVariants     []ClinVarVariant

	// MedlinePlus consumer health encyclopedia (English), full-text search.
	MedlinePlusEntries  []MedlinePlusEntry

	// National medical-insurance drug catalogue (国家医保药品目录).
	MedinsDrugs         []MedinsDrug

	// WHO Model List of Essential Medicines (24th list, 2025).
	EMLEntries          []EMLEntry

	// FDA drug labels (DailyMed/OpenFDA), curated Chinese summaries.
	FDALabels           []FDALabelEntry

	// NHC official 诊疗方案/指南 (国家卫健委), Chinese full text.
	NHCGuides           []NHCGuide

	// FHS parenting pages (香港卫生署家庭健康服务), Simplified Chinese full text.
	FHSGuides           []FHSGuide

	// AAP parenting articles (healthychildren.org), English full text.
	AAPEntries          []AAPEntry

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
		LiteratureByTopic: make(map[string][]*LiteratureEntry),
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

		// Embedded files are gzip-compressed (see external/make_gz.py);
		// decompress before parsing. The switch below matches the source
		// filename without the ".gz" suffix.
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("opening gzip %s: %w", path, err)
		}
		raw, readErr := io.ReadAll(zr)
		closeErr := zr.Close()
		if readErr != nil {
			return fmt.Errorf("decompressing %s: %w", path, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("closing gzip %s: %w", path, closeErr)
		}
		data = raw

		base := strings.TrimSuffix(filepath.Base(path), ".gz")
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
				for j, c := range e.Citations {
					key := fmt.Sprintf("%s-cite-%d-%d", e.ID, c.Year, j)
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

		case "food_risk.json":
			var entries []FoodRiskEntry
			if err := json.Unmarshal(data, &entries); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range entries {
				e := &entries[i]
				store.FoodRiskEntries = append(store.FoodRiskEntries, *e)
				store.FoodRiskByID[e.ID] = e
			}

		case "lab_tests.json":
			var entries []LabTestReference
			if err := json.Unmarshal(data, &entries); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range entries {
				e := &entries[i]
				store.LabTestReferences = append(store.LabTestReferences, *e)
				store.LabTestByID[e.ID] = e
			}

		case "version.json":
			var v DataVersion
			if err := json.Unmarshal(data, &v); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			store.DataVersion = &v

		case "literature.json":
			var ls LiteratureSet
			if err := json.Unmarshal(data, &ls); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			// Cross-validate the corpus: every article must belong to a
			// declared topic and carry a traceable DOI or PMID.
			topicIDs := make(map[string]bool, len(ls.Topics))
			for _, t := range ls.Topics {
				topicIDs[t.ID] = true
			}
			for i := range ls.Articles {
				a := &ls.Articles[i]
				if a.Title == "" {
					return fmt.Errorf("literature.json: article %s missing title", a.ID)
				}
				if !topicIDs[a.Topic] {
					return fmt.Errorf("literature.json: article %s references unknown topic %q", a.ID, a.Topic)
				}
				if a.DOI == "" && a.PMID == "" {
					return fmt.Errorf("literature.json: article %s has neither DOI nor PMID", a.ID)
				}
			}
			store.LiteratureTopics = ls.Topics
			store.LiteratureArticles = ls.Articles
			for i := range ls.Articles {
				a := &ls.Articles[i]
				store.LiteratureByTopic[a.Topic] = append(store.LiteratureByTopic[a.Topic], a)
			}

		case "who_factsheets.json":
			var entries []KnowledgeEntry
			if err := json.Unmarshal(data, &entries); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range entries {
				e := &entries[i]
				store.MedicalEntries = append(store.MedicalEntries, *e)
				store.MedicalByID[e.ID] = e
				for j, c := range e.Citations {
					key := fmt.Sprintf("%s-cite-%d-%d", e.ID, c.Year, j)
					store.ReferenceIndex[key] = c.Title
				}
			}

		case "who_vaccines.json":
			var entries []KnowledgeEntry
			if err := json.Unmarshal(data, &entries); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range entries {
				e := &entries[i]
				store.MedicalEntries = append(store.MedicalEntries, *e)
				store.MedicalByID[e.ID] = e
				for j, c := range e.Citations {
					key := fmt.Sprintf("%s-cite-%d-%d", e.ID, c.Year, j)
					store.ReferenceIndex[key] = c.Title
				}
			}

		case "china_vaccines.json":
			var entries []KnowledgeEntry
			if err := json.Unmarshal(data, &entries); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range entries {
				e := &entries[i]
				store.MedicalEntries = append(store.MedicalEntries, *e)
				store.MedicalByID[e.ID] = e
				for j, c := range e.Citations {
					key := fmt.Sprintf("%s-cite-%d-%d", e.ID, c.Year, j)
					store.ReferenceIndex[key] = c.Title
				}
			}

		case "feeding_guidelines.json":
			var entries []KnowledgeEntry
			if err := json.Unmarshal(data, &entries); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range entries {
				e := &entries[i]
				store.MedicalEntries = append(store.MedicalEntries, *e)
				store.MedicalByID[e.ID] = e
				for j, c := range e.Citations {
					key := fmt.Sprintf("%s-cite-%d-%d", e.ID, c.Year, j)
					store.ReferenceIndex[key] = c.Title
				}
			}

		case "cdc_entries.json":
			var entries []KnowledgeEntry
			if err := json.Unmarshal(data, &entries); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range entries {
				e := &entries[i]
				store.MedicalEntries = append(store.MedicalEntries, *e)
				store.MedicalByID[e.ID] = e
				for j, c := range e.Citations {
					key := fmt.Sprintf("%s-cite-%d-%d", e.ID, c.Year, j)
					store.ReferenceIndex[key] = c.Title
				}
			}

		case "msd_manual.json":
			var set MSDSet
			if err := json.Unmarshal(data, &set); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range set.Entries {
				if set.Entries[i].Title == "" || set.Entries[i].Content == "" {
					return fmt.Errorf("msd_manual.json: entry %d missing title/content", i)
				}
			}
			store.MSDEntries = set.Entries

		case "clinvar.json":
			var set ClinVarSet
			if err := json.Unmarshal(data, &set); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range set.Variants {
				if set.Variants[i].Variation == "" || set.Variants[i].Gene == "" {
					return fmt.Errorf("clinvar.json: variant %d missing variation/gene", i)
				}
			}
			store.ClinVarVariants = set.Variants

		case "medlineplus.json":
			var set MedlinePlusSet
			if err := json.Unmarshal(data, &set); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range set.Entries {
				if set.Entries[i].Title == "" || set.Entries[i].Content == "" {
					return fmt.Errorf("medlineplus.json: entry %d missing title/content", i)
				}
			}
			store.MedlinePlusEntries = set.Entries

		case "medins_drugs.json":
			var set MedinsSet
			if err := json.Unmarshal(data, &set); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range set.Drugs {
				if set.Drugs[i].Name == "" {
					return fmt.Errorf("medins_drugs.json: drug %d missing name", i)
				}
			}
			store.MedinsDrugs = set.Drugs

		case "who_eml.json":
			var set EMLSet
			if err := json.Unmarshal(data, &set); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range set.Entries {
				if set.Entries[i].Name == "" {
					return fmt.Errorf("who_eml.json: entry %d missing name", i)
				}
			}
			store.EMLEntries = set.Entries

		case "fda_drug_labels.json":
			var set FDALabelSet
			if err := json.Unmarshal(data, &set); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range set.Drugs {
				if set.Drugs[i].NameZH == "" {
					return fmt.Errorf("fda_drug_labels.json: drug %d missing name_zh", i)
				}
			}
			store.FDALabels = set.Drugs

		case "nhc_guides.json":
			var set NHCGuideSet
			if err := json.Unmarshal(data, &set); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range set.Entries {
				if set.Entries[i].Title == "" || set.Entries[i].Content == "" {
					return fmt.Errorf("nhc_guides.json: entry %d missing title/content", i)
				}
			}
			store.NHCGuides = set.Entries

		case "fhs_guides.json":
			var set FHSGuideSet
			if err := json.Unmarshal(data, &set); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range set.Entries {
				if set.Entries[i].Title == "" || set.Entries[i].Content == "" {
					return fmt.Errorf("fhs_guides.json: entry %d missing title/content", i)
				}
			}
			store.FHSGuides = set.Entries

		case "aap_articles.json":
			var set AAPSet
			if err := json.Unmarshal(data, &set); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			for i := range set.Entries {
				if set.Entries[i].Title == "" || set.Entries[i].Content == "" {
					return fmt.Errorf("aap_articles.json: entry %d missing title/content", i)
				}
			}
			store.AAPEntries = set.Entries

		default:
			// Unknown/extra data files are intentionally ignored here; add a
			// case above when wiring a new knowledge file.
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

// GetDataVersion returns the knowledge base version metadata, if present.
func (s *Store) GetDataVersion() *DataVersion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.DataVersion
}

// FoodEntriesAsKnowledge projects food-risk entries as KnowledgeEntry so the
// retriever and prompt builder can index them uniformly.
func (s *Store) FoodEntriesAsKnowledge() []KnowledgeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]KnowledgeEntry, 0, len(s.FoodRiskEntries))
	for i := range s.FoodRiskEntries {
		f := &s.FoodRiskEntries[i]
		out = append(out, KnowledgeEntry{
			ID:          f.ID,
			ConditionZH: f.FoodNameZH,
			ConditionEN: f.FoodNameEN,
			Category:    "food_risk",
			Keywords:    f.Keywords,
			Citations:   f.Citations,
		})
	}
	return out
}

// LabEntriesAsKnowledge projects lab-test references as KnowledgeEntry so the
// retriever and prompt builder can index them uniformly.
func (s *Store) LabEntriesAsKnowledge() []KnowledgeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]KnowledgeEntry, 0, len(s.LabTestReferences))
	for i := range s.LabTestReferences {
		l := &s.LabTestReferences[i]
		out = append(out, KnowledgeEntry{
			ID:          l.ID,
			ConditionZH: l.TestNameZH,
			ConditionEN: l.TestNameEN,
			Category:    "lab_test",
			Keywords:    l.Keywords,
			Citations:   l.Citations,
		})
	}
	return out
}

// IsLoaded returns whether the knowledge store has been loaded.
func (s *Store) IsLoaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loaded
}

// GetLiteratureTopics returns the literature topic table.
func (s *Store) GetLiteratureTopics() []LiteratureTopic {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LiteratureTopic, len(s.LiteratureTopics))
	copy(out, s.LiteratureTopics)
	return out
}

// GetLiteratureByTopic returns the articles of one topic (nil if unknown).
func (s *Store) GetLiteratureByTopic(topic string) []*LiteratureEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	arts := s.LiteratureByTopic[topic]
	out := make([]*LiteratureEntry, len(arts))
	copy(out, arts)
	return out
}

// GetLiteratureCount returns the total number of embedded literature entries.
func (s *Store) GetLiteratureCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.LiteratureArticles)
}

// GetMSDEntries returns the MSD Manual corpus (copy).
func (s *Store) GetMSDEntries() []MSDEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MSDEntry, len(s.MSDEntries))
	copy(out, s.MSDEntries)
	return out
}

// GetMSDCount returns the number of embedded MSD pages.
func (s *Store) GetMSDCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.MSDEntries)
}

// GetClinVarVariants returns the ClinVar subset (copy).
func (s *Store) GetClinVarVariants() []ClinVarVariant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ClinVarVariant, len(s.ClinVarVariants))
	copy(out, s.ClinVarVariants)
	return out
}

// GetClinVarCount returns the number of embedded ClinVar variants.
func (s *Store) GetClinVarCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.ClinVarVariants)
}

// GetMedlinePlusEntries returns the MedlinePlus corpus (copy).
func (s *Store) GetMedlinePlusEntries() []MedlinePlusEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MedlinePlusEntry, len(s.MedlinePlusEntries))
	copy(out, s.MedlinePlusEntries)
	return out
}

// GetMedinsDrugs returns the medical-insurance drug catalogue (copy).
func (s *Store) GetMedinsDrugs() []MedinsDrug {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MedinsDrug, len(s.MedinsDrugs))
	copy(out, s.MedinsDrugs)
	return out
}

// GetEMLEntries returns the WHO Essential Medicines List entries (copy).
func (s *Store) GetEMLEntries() []EMLEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]EMLEntry, len(s.EMLEntries))
	copy(out, s.EMLEntries)
	return out
}

// GetFDALabels returns the FDA-label entries (copy).
func (s *Store) GetFDALabels() []FDALabelEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FDALabelEntry, len(s.FDALabels))
	copy(out, s.FDALabels)
	return out
}

// GetNHCGuides returns the NHC guideline corpus (copy).
func (s *Store) GetNHCGuides() []NHCGuide {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]NHCGuide, len(s.NHCGuides))
	copy(out, s.NHCGuides)
	return out
}

// GetNHCGuideCount returns the number of embedded NHC guidelines.
func (s *Store) GetNHCGuideCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.NHCGuides)
}

// GetFHSGuides returns the FHS parenting corpus (copy).
func (s *Store) GetFHSGuides() []FHSGuide {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FHSGuide, len(s.FHSGuides))
	copy(out, s.FHSGuides)
	return out
}

// GetFHSGuideCount returns the number of embedded FHS pages.
func (s *Store) GetFHSGuideCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.FHSGuides)
}

// GetAAPEntries returns the AAP corpus (copy).
func (s *Store) GetAAPEntries() []AAPEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AAPEntry, len(s.AAPEntries))
	copy(out, s.AAPEntries)
	return out
}

// GetAAPCount returns the number of embedded AAP articles.
func (s *Store) GetAAPCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.AAPEntries)
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
