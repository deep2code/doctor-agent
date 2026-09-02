package knowledge

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/doctor-agent/internal/config"
)

// Store holds all loaded medical knowledge. Data is NOT embedded: the compiled
// binary contains only logic. Each dataset is loaded lazily from MariaDB the
// first time a retriever or tool needs it (see the ensure* helpers), then
// cached in memory for the process lifetime.
// Cold reads therefore hit the database directly; warm reads stay fast.
type Store struct {
	mu sync.RWMutex

	kb    *KB
	onces sync.Map // dataset name -> *sync.Once

	MedicalEntries []KnowledgeEntry
	MedicalByID    map[string]*KnowledgeEntry

	DrugEntries       []DrugEntry
	DrugByID          map[string]*DrugEntry
	DrugByGenericName map[string]*DrugEntry

	FoodRiskEntries []FoodRiskEntry
	FoodRiskByID    map[string]*FoodRiskEntry

	EmergencyRules    []EmergencyRule
	LabTestReferences []LabTestReference
	LabTestByID       map[string]*LabTestReference

	// Reference index for post-verification: citation ID -> title
	ReferenceIndex map[string]string

	// DataVersion describes the knowledge base release and its sources.
	DataVersion *DataVersion

	// Literature corpus (Europe PMC abstracts) for reference_lookup-style
	// retrieval with real DOI/PMID.
	LiteratureTopics   []LiteratureTopic
	LiteratureArticles []LiteratureEntry
	LiteratureByTopic  map[string][]*LiteratureEntry

	// MSD Manual (默沙东诊疗手册) Chinese consumer pages, full-text search.
	MSDEntries []MSDEntry

	// ClinVar subset: pathogenic/likely-pathogenic variants of the core
	// China high-burden genes (HBB/HBA1/HBA2/G6PD).
	ClinVarVariants []ClinVarVariant

	// MedlinePlus consumer health encyclopedia (English), full-text search.
	MedlinePlusEntries []MedlinePlusEntry

	// National medical-insurance drug catalogue (国家医保药品目录).
	MedinsDrugs []MedinsDrug

	// WHO Model List of Essential Medicines (24th list, 2025).
	EMLEntries []EMLEntry

	// FDA drug labels (DailyMed/OpenFDA), curated Chinese summaries.
	FDALabels []FDALabelEntry

	// NHC official 诊疗方案/指南 (国家卫健委), Chinese full text.
	NHCGuides []NHCGuide

	// FHS parenting pages (香港卫生署家庭健康服务), Simplified Chinese full text.
	FHSGuides []FHSGuide

	// AAP parenting articles (healthychildren.org), English full text.
	AAPEntries []AAPEntry

	// Health myths and misconceptions (日常错误观念/习惯).
	HealthMyths []HealthMyth

	// Body-part triage (人体部位分诊): region -> common conditions / red flags.
	BodyParts     []BodyPartTriage
	BodyPartByKey map[string]*BodyPartTriage

	// Pediatric growth standards (WHO + 中国 WS/T 423-2022), one whole doc.
	GrowthStandards *GrowthStandardsDoc

	// CDC developmental milestones by age (12 checklists).
	MilestoneAges       []MilestoneAge
	MilestoneByMonth    map[int]*MilestoneAge
	MilestoneDefinition string

	// Newborn care: WHO preterm/LBW recommendations + China screening.
	NewbornCare *NewbornCareDoc

	// China National Essential Medicines List (国家基本药物目录).
	EssentialMedicines []EssentialMedicine

	// ICD-10 disease classification (国家临床版2.0疾病诊断编码).
	ICD10Diseases []ICD10Disease
	ICD10ByCode   map[string]*ICD10Disease

	// NMPA drug catalogue (国家药品编码本位码信息).
	NMPADrugs  []NMPADrug
	NMPAByName map[string]*NMPADrug

	// Medical knowledge graph triples (OpenCMKG).
	MedicalKGTriples []MedicalKGTriple

	// Medical dialogue seeds (MedicalGPT-zh).
	MedicalDialogues []MedicalDialogue

	// Disease encyclopedias (CMeKG/QASystemOnMedicalKG).
	DiseaseEncyclopedias       []DiseaseEncyclopedia
	DiseaseEncyclopediasByName map[string]*DiseaseEncyclopedia

	// CPubMed-KG triples.
	CPubMedTriples    []CPubMedTriple
	CPubMedByHead     map[string][]*CPubMedTriple
	CPubMedByRelation map[string][]*CPubMedTriple

	// Huatuo26M-Lite QA pairs (华佗26M医疗问答).
	HuatuoQAPairs *HuatuoQAPairs

	// Medical QA pairs (中文医疗对话数据集).
	MedicalQAData *MedicalQAData

	// TTD data (Therapeutic Target Database).
	TTDData *TTDData

	// SIDER drug side effects and indications.
	SIDERData *SIDERDataSet
}

var globalStore *Store
var loadOnce sync.Once
var loadErr error

// Reload rebuilds the in-memory knowledge store from MariaDB. It is called
// after the admin API updates a dataset so running queries see the new rows.
// The old store is replaced atomically; per-dataset lazy loaders are reset.
func Reload() {
	newStore, err := buildStore()
	if err != nil {
		slog.Error("Knowledge reload failed; keeping previous store", "error", err)
		return
	}
	globalStore = newStore
	loadOnce = sync.Once{}
	loadErr = nil
}

// buildStore opens the KB and constructs a fresh Store (shared by Load/Reload).
func buildStore() (*Store, error) {
	cfg := config.Load()
	if err := cfg.EnsureKnowledgeDB(); err != nil {
		return nil, fmt.Errorf("ensure knowledge database: %w", err)
	}
	dsn := resolveKBPath()
	kb, err := OpenKB(dsn)
	if err != nil {
		return nil, err
	}
	return &Store{
		kb:                         kb,
		MedicalByID:                make(map[string]*KnowledgeEntry),
		DrugByID:                   make(map[string]*DrugEntry),
		DrugByGenericName:          make(map[string]*DrugEntry),
		FoodRiskByID:               make(map[string]*FoodRiskEntry),
		LabTestByID:                make(map[string]*LabTestReference),
		BodyPartByKey:              make(map[string]*BodyPartTriage),
		ReferenceIndex:             make(map[string]string),
		LiteratureByTopic:          make(map[string][]*LiteratureEntry),
		ICD10ByCode:                make(map[string]*ICD10Disease),
		NMPAByName:                 make(map[string]*NMPADrug),
		DiseaseEncyclopediasByName: make(map[string]*DiseaseEncyclopedia),
		CPubMedByHead:              make(map[string][]*CPubMedTriple),
		CPubMedByRelation:          make(map[string][]*CPubMedTriple),
		MilestoneByMonth:           make(map[int]*MilestoneAge),
	}, nil
}

// Load opens the knowledge database and returns a (lazily populated) Store.
// The DSN is taken from config (MARIA_DB_* / KNOWLEDGE_DB_DSN), so datasets are
// fetched from MariaDB on first use. No knowledge data is embedded in the
// binary.
func Load() (*Store, error) {
	loadOnce.Do(func() {
		globalStore, loadErr = buildStore()
	})
	if loadErr != nil {
		return nil, loadErr
	}
	return globalStore, nil
}

// resolveKBPath returns the knowledge-store DSN. An explicit KNOWLEDGE_DB_DSN
// env var wins; otherwise the DSN is composed from the MariaDB configuration.
func resolveKBPath() string {
	return config.Load().KnowledgeDBDSN()
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	if s.kb == nil {
		return nil
	}
	return s.kb.Close()
}

// once returns the sync.Once associated with a dataset name.
func (s *Store) once(name string) *sync.Once {
	actual, _ := s.onces.LoadOrStore(name, &sync.Once{})
	return actual.(*sync.Once)
}

// ensure runs fn exactly once per dataset name.
func (s *Store) ensure(name string, fn func() error) error {
	var err error
	s.once(name).Do(func() { err = fn() })
	return err
}

// loadDataset reads every row of a dataset from the database and ingests it.
func (s *Store) loadDataset(name string) error {
	rows, err := s.kb.All(name)
	if err != nil {
		return err
	}
	for _, raw := range rows {
		if err := s.ingest(name, raw); err != nil {
			return fmt.Errorf("ingesting %s: %w", name, err)
		}
	}
	return nil
}

// ingest appends a single database row to the appropriate Store field.
func (s *Store) ingest(name string, raw []byte) error {
	switch name {
	case DSMedical:
		var e KnowledgeEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		s.MedicalEntries = append(s.MedicalEntries, e)
		s.MedicalByID[e.ID] = &s.MedicalEntries[len(s.MedicalEntries)-1]
		for j, c := range e.Citations {
			s.ReferenceIndex[fmt.Sprintf("%s-cite-%d-%d", e.ID, c.Year, j)] = c.Title
		}
	case DSDrug:
		var e DrugEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		s.DrugEntries = append(s.DrugEntries, e)
		s.DrugByID[e.ID] = &s.DrugEntries[len(s.DrugEntries)-1]
		s.DrugByGenericName[e.GenericNameEN] = &s.DrugEntries[len(s.DrugEntries)-1]
		s.DrugByGenericName[e.GenericNameZH] = &s.DrugEntries[len(s.DrugEntries)-1]
	case DSEmergency:
		var rules []EmergencyRule
		if err := json.Unmarshal(raw, &rules); err != nil {
			return err
		}
		s.EmergencyRules = rules
	case DSFoodRisk:
		var e FoodRiskEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		s.FoodRiskEntries = append(s.FoodRiskEntries, e)
		s.FoodRiskByID[e.ID] = &s.FoodRiskEntries[len(s.FoodRiskEntries)-1]
	case DSLabTest:
		var e LabTestReference
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		s.LabTestReferences = append(s.LabTestReferences, e)
		s.LabTestByID[e.ID] = &s.LabTestReferences[len(s.LabTestReferences)-1]
	case DSLiterature:
		// Topics are stored as a single row keyed "topics"; articles as rows
		// keyed by their ID.
		var topics []LiteratureTopic
		if err := json.Unmarshal(raw, &topics); err == nil && len(topics) > 0 {
			s.LiteratureTopics = topics
			return nil
		}
		var a LiteratureEntry
		if err := json.Unmarshal(raw, &a); err != nil {
			return err
		}
		s.LiteratureArticles = append(s.LiteratureArticles, a)
		s.LiteratureByTopic[a.Topic] = append(s.LiteratureByTopic[a.Topic], &s.LiteratureArticles[len(s.LiteratureArticles)-1])
	case DSMSD:
		var e MSDEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		s.MSDEntries = append(s.MSDEntries, e)
	case DSClinVar:
		var v ClinVarVariant
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		s.ClinVarVariants = append(s.ClinVarVariants, v)
	case DSMedlinePlus:
		var e MedlinePlusEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		s.MedlinePlusEntries = append(s.MedlinePlusEntries, e)
	case DSMedins:
		var d MedinsDrug
		if err := json.Unmarshal(raw, &d); err != nil {
			return err
		}
		s.MedinsDrugs = append(s.MedinsDrugs, d)
	case DSEML:
		var e EMLEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		s.EMLEntries = append(s.EMLEntries, e)
	case DSFDA:
		var e FDALabelEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		s.FDALabels = append(s.FDALabels, e)
	case DSNHC:
		var e NHCGuide
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		s.NHCGuides = append(s.NHCGuides, e)
	case DSFHS:
		var e FHSGuide
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		s.FHSGuides = append(s.FHSGuides, e)
	case DSAAP:
		var e AAPEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		s.AAPEntries = append(s.AAPEntries, e)
	case DSHealthMyths:
		var m HealthMyth
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		s.HealthMyths = append(s.HealthMyths, m)
	case DSBodyPart:
		var b BodyPartTriage
		if err := json.Unmarshal(raw, &b); err != nil {
			return err
		}
		s.BodyParts = append(s.BodyParts, b)
		s.BodyPartByKey[b.PartKey] = &s.BodyParts[len(s.BodyParts)-1]
	case DSGrowth:
		var doc GrowthStandardsDoc
		if err := json.Unmarshal(raw, &doc); err != nil {
			return err
		}
		s.GrowthStandards = &doc
	case DSMilestones:
		// Rows keyed by age_key, plus one "meta" row (source/definition).
		var age MilestoneAge
		if err := json.Unmarshal(raw, &age); err == nil && age.AgeKey != "" {
			s.MilestoneAges = append(s.MilestoneAges, age)
			if m := milestoneAgeToMonth(age.AgeKey); m >= 0 {
				s.MilestoneByMonth[m] = &s.MilestoneAges[len(s.MilestoneAges)-1]
			}
			return nil
		}
		var meta struct {
			Source     string `json:"source"`
			Definition string `json:"definition"`
		}
		if err := json.Unmarshal(raw, &meta); err != nil {
			return err
		}
		s.MilestoneDefinition = meta.Definition
	case DSNewborn:
		var doc NewbornCareDoc
		if err := json.Unmarshal(raw, &doc); err != nil {
			return err
		}
		s.NewbornCare = &doc
	case DSEssential:
		var d EssentialMedicine
		if err := json.Unmarshal(raw, &d); err != nil {
			return err
		}
		s.EssentialMedicines = append(s.EssentialMedicines, d)
	case DSICD10:
		var d ICD10Disease
		if err := json.Unmarshal(raw, &d); err != nil {
			return err
		}
		s.ICD10Diseases = append(s.ICD10Diseases, d)
		s.ICD10ByCode[d.Code] = &s.ICD10Diseases[len(s.ICD10Diseases)-1]
	case DSNMPA:
		var d NMPADrug
		if err := json.Unmarshal(raw, &d); err != nil {
			return err
		}
		s.NMPADrugs = append(s.NMPADrugs, d)
		s.NMPAByName[d.NameZH] = &s.NMPADrugs[len(s.NMPADrugs)-1]
	case DSMedicalKG:
		var t MedicalKGTriple
		if err := json.Unmarshal(raw, &t); err != nil {
			return err
		}
		s.MedicalKGTriples = append(s.MedicalKGTriples, t)
	case DSMedicalDialogues:
		var d MedicalDialogue
		if err := json.Unmarshal(raw, &d); err != nil {
			return err
		}
		s.MedicalDialogues = append(s.MedicalDialogues, d)
	case DSDiseaseEnc:
		var d DiseaseEncyclopedia
		if err := json.Unmarshal(raw, &d); err != nil {
			return err
		}
		s.DiseaseEncyclopedias = append(s.DiseaseEncyclopedias, d)
		s.DiseaseEncyclopediasByName[d.NameZH] = &s.DiseaseEncyclopedias[len(s.DiseaseEncyclopedias)-1]
	case DSCPubMed:
		var t CPubMedTriple
		if err := json.Unmarshal(raw, &t); err != nil {
			return err
		}
		s.CPubMedTriples = append(s.CPubMedTriples, t)
		s.CPubMedByHead[t.Head] = append(s.CPubMedByHead[t.Head], &s.CPubMedTriples[len(s.CPubMedTriples)-1])
		s.CPubMedByRelation[t.Relation] = append(s.CPubMedByRelation[t.Relation], &s.CPubMedTriples[len(s.CPubMedTriples)-1])
	case DSHuatuo:
		var qa HuatuoQA
		if err := json.Unmarshal(raw, &qa); err != nil {
			return err
		}
		if s.HuatuoQAPairs == nil {
			s.HuatuoQAPairs = &HuatuoQAPairs{}
		}
		s.HuatuoQAPairs.QAPairs = append(s.HuatuoQAPairs.QAPairs, qa)
	case DSMedicalQA:
		var qa MedicalQAPair
		if err := json.Unmarshal(raw, &qa); err != nil {
			return err
		}
		if s.MedicalQAData == nil {
			s.MedicalQAData = &MedicalQAData{}
		}
		s.MedicalQAData.QAPairs = append(s.MedicalQAData.QAPairs, qa)
	case DSTTD:
		var ttd TTDData
		if err := json.Unmarshal(raw, &ttd); err != nil {
			return err
		}
		s.TTDData = &ttd
	case DSSIDER:
		var sider SIDERDataSet
		if err := json.Unmarshal(raw, &sider); err != nil {
			return err
		}
		s.SIDERData = &sider
	case DSVersion:
		var v DataVersion
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		s.DataVersion = &v
	default:
		// Unknown dataset — ignore.
	}
	return nil
}

// ensure* load a single dataset from the database on first use.
func (s *Store) ensureMedical() error {
	return s.ensure(DSMedical, func() error { return s.loadDataset(DSMedical) })
}
func (s *Store) ensureDrug() error {
	return s.ensure(DSDrug, func() error { return s.loadDataset(DSDrug) })
}
func (s *Store) ensureEmergency() error {
	return s.ensure(DSEmergency, func() error { return s.loadDataset(DSEmergency) })
}
func (s *Store) ensureFoodRisk() error {
	return s.ensure(DSFoodRisk, func() error { return s.loadDataset(DSFoodRisk) })
}
func (s *Store) ensureLabTest() error {
	return s.ensure(DSLabTest, func() error { return s.loadDataset(DSLabTest) })
}
func (s *Store) ensureLiterature() error {
	return s.ensure(DSLiterature, func() error { return s.loadDataset(DSLiterature) })
}
func (s *Store) ensureMSD() error {
	return s.ensure(DSMSD, func() error { return s.loadDataset(DSMSD) })
}
func (s *Store) ensureClinVar() error {
	return s.ensure(DSClinVar, func() error { return s.loadDataset(DSClinVar) })
}
func (s *Store) ensureMedlinePlus() error {
	return s.ensure(DSMedlinePlus, func() error { return s.loadDataset(DSMedlinePlus) })
}
func (s *Store) ensureMedins() error {
	return s.ensure(DSMedins, func() error { return s.loadDataset(DSMedins) })
}
func (s *Store) ensureEML() error {
	return s.ensure(DSEML, func() error { return s.loadDataset(DSEML) })
}
func (s *Store) ensureFDA() error {
	return s.ensure(DSFDA, func() error { return s.loadDataset(DSFDA) })
}
func (s *Store) ensureNHC() error {
	return s.ensure(DSNHC, func() error { return s.loadDataset(DSNHC) })
}
func (s *Store) ensureFHS() error {
	return s.ensure(DSFHS, func() error { return s.loadDataset(DSFHS) })
}
func (s *Store) ensureAAP() error {
	return s.ensure(DSAAP, func() error { return s.loadDataset(DSAAP) })
}
func (s *Store) ensureHealthMyths() error {
	return s.ensure(DSHealthMyths, func() error { return s.loadDataset(DSHealthMyths) })
}
func (s *Store) ensureBodyPart() error {
	return s.ensure(DSBodyPart, func() error { return s.loadDataset(DSBodyPart) })
}
func (s *Store) ensureGrowth() error {
	return s.ensure(DSGrowth, func() error { return s.loadDataset(DSGrowth) })
}
func (s *Store) ensureMilestones() error {
	return s.ensure(DSMilestones, func() error { return s.loadDataset(DSMilestones) })
}
func (s *Store) ensureNewborn() error {
	return s.ensure(DSNewborn, func() error { return s.loadDataset(DSNewborn) })
}
func (s *Store) ensureEssential() error {
	return s.ensure(DSEssential, func() error { return s.loadDataset(DSEssential) })
}
func (s *Store) ensureICD10() error {
	return s.ensure(DSICD10, func() error { return s.loadDataset(DSICD10) })
}
func (s *Store) ensureNMPA() error {
	return s.ensure(DSNMPA, func() error { return s.loadDataset(DSNMPA) })
}
func (s *Store) ensureMedicalKG() error {
	return s.ensure(DSMedicalKG, func() error { return s.loadDataset(DSMedicalKG) })
}
func (s *Store) ensureMedicalDialogues() error {
	return s.ensure(DSMedicalDialogues, func() error { return s.loadDataset(DSMedicalDialogues) })
}
func (s *Store) ensureDiseaseEnc() error {
	return s.ensure(DSDiseaseEnc, func() error { return s.loadDataset(DSDiseaseEnc) })
}
func (s *Store) ensureCPubMed() error {
	return s.ensure(DSCPubMed, func() error { return s.loadDataset(DSCPubMed) })
}
func (s *Store) ensureHuatuo() error {
	return s.ensure(DSHuatuo, func() error { return s.loadDataset(DSHuatuo) })
}
func (s *Store) ensureMedicalQA() error {
	return s.ensure(DSMedicalQA, func() error { return s.loadDataset(DSMedicalQA) })
}
func (s *Store) ensureTTD() error {
	return s.ensure(DSTTD, func() error { return s.loadDataset(DSTTD) })
}
func (s *Store) ensureSIDER() error {
	return s.ensure(DSSIDER, func() error { return s.loadDataset(DSSIDER) })
}
func (s *Store) ensureVersion() error {
	return s.ensure(DSVersion, func() error { return s.loadDataset(DSVersion) })
}

// ensureAll loads every dataset from the database into memory. It is intended
// for maintenance paths (knowledge verification, vector sync) that must operate
// over the whole corpus at once; runtime retrieval uses per-dataset lazy loads.
func (s *Store) ensureAll() {
	_ = s.ensureMedical()
	_ = s.ensureDrug()
	_ = s.ensureEmergency()
	_ = s.ensureFoodRisk()
	_ = s.ensureLabTest()
	_ = s.ensureLiterature()
	_ = s.ensureMSD()
	_ = s.ensureNHC()
	_ = s.ensureFHS()
	_ = s.ensureAAP()
	_ = s.ensureClinVar()
	_ = s.ensureMedlinePlus()
	_ = s.ensureMedins()
	_ = s.ensureEML()
	_ = s.ensureFDA()
	_ = s.ensureICD10()
	_ = s.ensureNMPA()
	_ = s.ensureMedicalKG()
	_ = s.ensureMedicalDialogues()
	_ = s.ensureDiseaseEnc()
	_ = s.ensureCPubMed()
	_ = s.ensureHuatuo()
	_ = s.ensureMedicalQA()
	_ = s.ensureTTD()
	_ = s.ensureSIDER()
	_ = s.ensureHealthMyths()
	_ = s.ensureBodyPart()
	_ = s.ensureEssential()
	_ = s.ensureVersion()
}

// GetMedicalByID retrieves a medical knowledge entry by ID.
func (s *Store) GetMedicalByID(id string) *KnowledgeEntry {
	_ = s.ensureMedical()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.MedicalByID[id]
}

// GetDrugByName retrieves a drug entry by generic name (EN or ZH).
func (s *Store) GetDrugByName(name string) *DrugEntry {
	_ = s.ensureDrug()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.DrugByGenericName[name]; ok {
		return e
	}
	return nil
}

// GetAllMedical returns all medical knowledge entries.
// GetAllMedical returns all medical knowledge entries.
func (s *Store) GetAllMedical() []KnowledgeEntry {
	_ = s.ensureMedical()
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]KnowledgeEntry, len(s.MedicalEntries))
	copy(entries, s.MedicalEntries)
	return entries
}

// GetAllBodyParts returns all body-part triage entries (人体部位分诊).
func (s *Store) GetAllBodyParts() []BodyPartTriage {
	_ = s.ensureBodyPart()
	s.mu.RLock()
	defer s.mu.RUnlock()
	parts := make([]BodyPartTriage, len(s.BodyParts))
	copy(parts, s.BodyParts)
	return parts
}

// GetBodyPartByKey returns one body-part triage entry by part_key.
func (s *Store) GetBodyPartByKey(key string) *BodyPartTriage {
	_ = s.ensureBodyPart()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.BodyPartByKey[key]
}

// GetAllDrugs returns all drug entries.
func (s *Store) GetAllDrugs() []DrugEntry {
	_ = s.ensureDrug()
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]DrugEntry, len(s.DrugEntries))
	copy(entries, s.DrugEntries)
	return entries
}

// GetAllEmergencyRules returns all emergency triage rules.
func (s *Store) GetAllEmergencyRules() []EmergencyRule {
	_ = s.ensureEmergency()
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := make([]EmergencyRule, len(s.EmergencyRules))
	copy(rules, s.EmergencyRules)
	return rules
}

// GetDataVersion returns the knowledge base version metadata, if present.
func (s *Store) GetDataVersion() *DataVersion {
	_ = s.ensureVersion()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.DataVersion
}

// FoodEntriesAsKnowledge projects food-risk entries as KnowledgeEntry so the
// retriever and prompt builder can index them uniformly.
func (s *Store) FoodEntriesAsKnowledge() []KnowledgeEntry {
	_ = s.ensureFoodRisk()
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
	_ = s.ensureLabTest()
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

// GetLiteratureTopics returns the literature topic table.
func (s *Store) GetLiteratureTopics() []LiteratureTopic {
	_ = s.ensureLiterature()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LiteratureTopic, len(s.LiteratureTopics))
	copy(out, s.LiteratureTopics)
	return out
}

// GetLiteratureByTopic returns the articles of one topic (nil if unknown).
func (s *Store) GetLiteratureByTopic(topic string) []*LiteratureEntry {
	_ = s.ensureLiterature()
	s.mu.RLock()
	defer s.mu.RUnlock()
	arts := s.LiteratureByTopic[topic]
	out := make([]*LiteratureEntry, len(arts))
	copy(out, arts)
	return out
}

// GetLiteratureCount returns the total number of embedded literature entries.
func (s *Store) GetLiteratureCount() int {
	_ = s.ensureLiterature()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.LiteratureArticles)
}

// GetMSDEntries returns the MSD Manual corpus (copy).
func (s *Store) GetMSDEntries() []MSDEntry {
	_ = s.ensureMSD()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MSDEntry, len(s.MSDEntries))
	copy(out, s.MSDEntries)
	return out
}

// GetMSDCount returns the number of embedded MSD pages.
func (s *Store) GetMSDCount() int {
	_ = s.ensureMSD()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.MSDEntries)
}

// GetClinVarCount returns the number of embedded ClinVar variants.
func (s *Store) GetClinVarCount() int {
	_ = s.ensureClinVar()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.ClinVarVariants)
}

// GetMedlinePlusEntries returns the MedlinePlus corpus (copy).
func (s *Store) GetMedlinePlusEntries() []MedlinePlusEntry {
	_ = s.ensureMedlinePlus()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MedlinePlusEntry, len(s.MedlinePlusEntries))
	copy(out, s.MedlinePlusEntries)
	return out
}

// GetMedinsDrugs returns the medical-insurance drug catalogue (copy).
func (s *Store) GetMedinsDrugs() []MedinsDrug {
	_ = s.ensureMedins()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MedinsDrug, len(s.MedinsDrugs))
	copy(out, s.MedinsDrugs)
	return out
}

// GetEMLEntries returns the WHO Essential Medicines List entries (copy).
func (s *Store) GetEMLEntries() []EMLEntry {
	_ = s.ensureEML()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]EMLEntry, len(s.EMLEntries))
	copy(out, s.EMLEntries)
	return out
}

// GetFDALabels returns the FDA-label entries (copy).
func (s *Store) GetFDALabels() []FDALabelEntry {
	_ = s.ensureFDA()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FDALabelEntry, len(s.FDALabels))
	copy(out, s.FDALabels)
	return out
}

// GetNHCGuides returns the NHC guideline corpus (copy).
func (s *Store) GetNHCGuides() []NHCGuide {
	_ = s.ensureNHC()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]NHCGuide, len(s.NHCGuides))
	copy(out, s.NHCGuides)
	return out
}

// GetFHSGuides returns the FHS parenting corpus (copy).
func (s *Store) GetFHSGuides() []FHSGuide {
	_ = s.ensureFHS()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FHSGuide, len(s.FHSGuides))
	copy(out, s.FHSGuides)
	return out
}

// GetAAPEntries returns the AAP corpus (copy).
func (s *Store) GetAAPEntries() []AAPEntry {
	_ = s.ensureAAP()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AAPEntry, len(s.AAPEntries))
	copy(out, s.AAPEntries)
	return out
}

// GetReferenceIndex returns the reference index for post-verification.
func (s *Store) GetReferenceIndex() map[string]string {
	_ = s.ensureMedical()
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx := make(map[string]string, len(s.ReferenceIndex))
	for k, v := range s.ReferenceIndex {
		idx[k] = v
	}
	return idx
}

// GetICD10DiseaseByCode retrieves an ICD-10 disease by code.
func (s *Store) GetICD10DiseaseByCode(code string) *ICD10Disease {
	_ = s.ensureICD10()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if d, ok := s.ICD10ByCode[code]; ok {
		return d
	}
	return nil
}

// SearchICD10Diseases searches ICD-10 diseases by name substring.
func (s *Store) SearchICD10Diseases(query string, limit int) []ICD10Disease {
	_ = s.ensureICD10()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matches []ICD10Disease
	for _, d := range s.ICD10Diseases {
		if strings.Contains(d.NameZH, query) || strings.Contains(strings.ToLower(d.Code), strings.ToLower(query)) {
			matches = append(matches, d)
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches
}

// GetNMPADrugByName retrieves an NMPA drug by name.
func (s *Store) GetNMPADrugByName(name string) *NMPADrug {
	_ = s.ensureNMPA()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if d, ok := s.NMPAByName[name]; ok {
		return d
	}
	return nil
}

// SearchNMPADrugs searches NMPA drugs by name substring.
func (s *Store) SearchNMPADrugs(query string, limit int) []NMPADrug {
	_ = s.ensureNMPA()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matches []NMPADrug
	for _, d := range s.NMPADrugs {
		if strings.Contains(d.NameZH, query) {
			matches = append(matches, d)
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches
}

// SearchMedicalKG searches medical knowledge graph triples by entity.
func (s *Store) SearchMedicalKG(entity string, relation string, limit int) []MedicalKGTriple {
	_ = s.ensureMedicalKG()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matches []MedicalKGTriple
	for _, triple := range s.MedicalKGTriples {
		if strings.Contains(triple.Entity1, entity) || strings.Contains(triple.Entity2, entity) {
			if relation == "" || triple.Relation == relation {
				matches = append(matches, triple)
				if len(matches) >= limit {
					break
				}
			}
		}
	}
	return matches
}

// GetDiseaseEncyclopediaByName retrieves a disease encyclopedia entry by name.
func (s *Store) GetDiseaseEncyclopediaByName(name string) *DiseaseEncyclopedia {
	_ = s.ensureDiseaseEnc()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if d, ok := s.DiseaseEncyclopediasByName[name]; ok {
		return d
	}
	return nil
}

// SearchDiseaseEncyclopedias searches disease encyclopedias by name substring.
func (s *Store) SearchDiseaseEncyclopedias(query string, limit int) []DiseaseEncyclopedia {
	_ = s.ensureDiseaseEnc()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matches []DiseaseEncyclopedia
	for _, d := range s.DiseaseEncyclopedias {
		if strings.Contains(d.NameZH, query) {
			matches = append(matches, d)
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches
}

// GetCPubMedTriplesByHead retrieves CPubMed triples by head entity.
func (s *Store) GetCPubMedTriplesByHead(head string) []*CPubMedTriple {
	_ = s.ensureCPubMed()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CPubMedByHead[head]
}

// SearchCPubMedTriples searches CPubMed triples by head entity substring.
func (s *Store) SearchCPubMedTriples(query string, limit int) []*CPubMedTriple {
	_ = s.ensureCPubMed()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matches []*CPubMedTriple
	for head, triples := range s.CPubMedByHead {
		if strings.Contains(head, query) {
			matches = append(matches, triples...)
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches
}

// GetHuatuoQA returns the Huatuo26M-Lite QA pairs.
func (s *Store) GetHuatuoQA() *HuatuoQAPairs {
	_ = s.ensureHuatuo()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.HuatuoQAPairs
}

// GetMedicalQA returns the Medical QA data.
func (s *Store) GetMedicalQA() *MedicalQAData {
	_ = s.ensureMedicalQA()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.MedicalQAData
}

// GetTTDData returns the TTD data.
func (s *Store) GetTTDData() *TTDData {
	_ = s.ensureTTD()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TTDData
}

// GetSIDERData returns the SIDER drug side effects data.
func (s *Store) GetSIDERData() *SIDERDataSet {
	_ = s.ensureSIDER()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SIDERData
}

// SearchSIDERDrugs searches SIDER drugs by ID.
func (s *Store) SearchSIDERDrugs(query string, limit int) []SIDERDrug {
	_ = s.ensureSIDER()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matches []SIDERDrug
	query = strings.ToLower(query)
	for _, drug := range s.SIDERData.Drugs {
		if strings.Contains(strings.ToLower(drug.ID), query) {
			matches = append(matches, drug)
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches
}
