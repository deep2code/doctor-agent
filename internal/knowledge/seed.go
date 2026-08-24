package knowledge

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Seed reads every gzip-compressed knowledge file in gzDir and inserts its
// contents into the knowledge database at dbPath. The compiled binary embeds
// nothing; this command (run at build/release time) materialises the data into
// an external SQLite file (knowledge.db) that ships alongside the binary.
//
// Each source file maps to one dataset; its entries become individual rows keyed
// by a stable id (or running index), with a lower-cased search_text column used
// for candidate filtering at query time.
func Seed(dbPath, gzDir string) error {
	kb, err := OpenKB(dbPath)
	if err != nil {
		return err
	}
	defer kb.Close()

	files, err := filepath.Glob(filepath.Join(gzDir, "*.gz"))
	if err != nil {
		return fmt.Errorf("globbing %s: %w", gzDir, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no .gz files found in %s", gzDir)
	}

	// Accumulate rows per dataset: several source files share one dataset
	// (e.g. every medical file maps to DSMedical), so we must NOT clear the
	// dataset between files. Collect everything first, then clear each dataset
	// once and bulk-insert.
	byDataset := make(map[string][]KBRow)
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".gz")
		raw, err := decompressFile(f)
		if err != nil {
			return fmt.Errorf("reading %s: %w", f, err)
		}
		ds, rows, err := seedFile(base, raw)
		if err != nil {
			return fmt.Errorf("seeding %s: %w", base, err)
		}
		if ds == "" {
			fmt.Printf("skipped %s\n", base)
			continue
		}
		byDataset[ds] = append(byDataset[ds], rows...)
	}

	for ds, rows := range byDataset {
		if err := kb.Clear(ds); err != nil {
			return fmt.Errorf("clearing %s: %w", ds, err)
		}
		if err := kb.InsertBatch(ds, rows); err != nil {
			return fmt.Errorf("inserting %s: %w", ds, err)
		}
		fmt.Printf("seeded %s (%d rows)\n", ds, len(rows))
	}
	return nil
}

func decompressFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

// seedFile classifies one source file by its (gz-stripped) name and returns the
// target dataset plus its rows. The classification mirrors the previous embedded
// loader so dataset boundaries stay identical.
func seedFile(base string, raw []byte) (string, []KBRow, error) {
	switch base {
	case "thalassemia.json", "g6pd_deficiency.json",
		"nasopharyngeal_carcinoma.json", "hepatitis_b.json",
		"lactose_intolerance.json", "aldh2_deficiency.json",
		"dengue.json", "fungal_infections.json",
		"who_factsheets.json", "who_vaccines.json",
		"china_vaccines.json", "feeding_guidelines.json",
		"cdc_entries.json", "diabetes.json", "hypertension.json", "cardiovascular.json",
		"copd.json", "tuberculosis.json", "hp_infection.json",
		"common_diseases.json", "common_diseases_batch2.json",
		"common_diseases_batch3.json", "common_diseases_batch4.json":
		rows, err := seedList(raw)
		return DSMedical, rows, err

	case "drug_contraindications.json":
		rows, err := seedList(raw)
		return DSDrug, rows, err
	case "emergency_triage.json":
		return DSEmergency, []KBRow{seedSingleton("rules", raw)}, nil
	case "food_risk.json":
		rows, err := seedList(raw)
		return DSFoodRisk, rows, err
	case "lab_tests.json":
		rows, err := seedList(raw)
		return DSLabTest, rows, err
	case "version.json":
		return DSVersion, []KBRow{seedSingleton("data", raw)}, nil
	case "literature.json":
		var ls LiteratureSet
		if err := json.Unmarshal(raw, &ls); err != nil {
			return "", nil, err
		}
		topicBytes, _ := json.Marshal(ls.Topics)
		rows := []KBRow{{Key: "topics", SearchText: buildSearchText(topicBytes), Data: topicBytes}}
		for i := range ls.Articles {
			a := &ls.Articles[i]
			b, _ := json.Marshal(a)
			key := a.ID
			if key == "" {
				key = fmt.Sprintf("art-%d", i)
			}
			rows = append(rows, KBRow{Key: key, SearchText: buildSearchText(b), Data: b})
		}
		return DSLiterature, rows, nil
	case "msd_manual.json":
		var set MSDSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Entries)
		return DSMSD, rows, err
	case "clinvar.json":
		var set ClinVarSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Variants)
		return DSClinVar, rows, err
	case "medlineplus.json":
		var set MedlinePlusSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Entries)
		return DSMedlinePlus, rows, err
	case "medins_drugs.json":
		var set MedinsSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Drugs)
		return DSMedins, rows, err
	case "who_eml.json":
		var set EMLSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Entries)
		return DSEML, rows, err
	case "fda_drug_labels.json":
		var set FDALabelSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Drugs)
		return DSFDA, rows, err
	case "nhc_guides.json":
		var set NHCGuideSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Entries)
		return DSNHC, rows, err
	case "fhs_guides.json":
		var set FHSGuideSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Entries)
		return DSFHS, rows, err
	case "aap_articles.json":
		var set AAPSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Entries)
		return DSAAP, rows, err
	case "health_myths.json":
		var myths []HealthMyth
		if err := json.Unmarshal(raw, &myths); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(myths)
		return DSHealthMyths, rows, err
	case "essential_medicines.json":
		var drugs []EssentialMedicine
		if err := json.Unmarshal(raw, &drugs); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(drugs)
		return DSEssential, rows, err
	case "icd10_diseases.json":
		var set ICD10DiseaseSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Diseases)
		return DSICD10, rows, err
	case "nmpa_drugs.json":
		var set NMPADrugSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Drugs)
		return DSNMPA, rows, err
	case "medical_kg_triples.json":
		var set MedicalKGSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Triples)
		return DSMedicalKG, rows, err
	case "medical_dialogues.json":
		var set MedicalDialogueSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Dialogues)
		return DSMedicalDialogues, rows, err
	case "disease_encyclopedias.json":
		var set DiseaseEncyclopediaSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Diseases)
		return DSDiseaseEnc, rows, err
	case "cpubmed_kg.json":
		var set CPubMedKGSet
		if err := json.Unmarshal(raw, &set); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(set.Triples)
		return DSCPubMed, rows, err
	case "huatuo_qa.json":
		var hp HuatuoQAPairs
		if err := json.Unmarshal(raw, &hp); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(hp.QAPairs)
		return DSHuatuo, rows, err
	case "medical_qa_pairs.json":
		var mq MedicalQAData
		if err := json.Unmarshal(raw, &mq); err != nil {
			return "", nil, err
		}
		rows, err := seedEntries(mq.QAPairs)
		return DSMedicalQA, rows, err
	case "ttd_data.json":
		return DSTTD, []KBRow{seedSingleton("data", raw)}, nil
	case "sider_drugs.json":
		return DSSIDER, []KBRow{seedSingleton("data", raw)}, nil
	default:
		// Unknown file — skip silently (mirrors the embedded loader).
		return "", nil, nil
	}
}

// seedList builds rows from a JSON array of arbitrary entries
// (KnowledgeEntry/DrugEntry/…).
func seedList(raw []byte) ([]KBRow, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	rows := make([]KBRow, 0, len(items))
	for i, it := range items {
		rows = append(rows, KBRow{Key: extractKey(it, i), SearchText: buildSearchText(it), Data: it})
	}
	return rows, nil
}

// seedEntries builds rows from a typed slice by re-marshalling each element.
func seedEntries(items interface{}) ([]KBRow, error) {
	raw, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, err
	}
	rows := make([]KBRow, 0, len(arr))
	for i, it := range arr {
		rows = append(rows, KBRow{Key: extractKey(it, i), SearchText: buildSearchText(it), Data: it})
	}
	return rows, nil
}

// seedSingleton builds a single row from one whole JSON document.
func seedSingleton(key string, raw []byte) KBRow {
	return KBRow{Key: key, SearchText: buildSearchText(raw), Data: raw}
}

// extractKey derives a stable row key from a JSON document, preferring common
// id/code/name fields, falling back to the element index.
func extractKey(raw json.RawMessage, idx int) string {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Sprintf("idx-%d", idx)
	}
	for _, k := range []string{"id", "ID", "code", "Code", "name", "Name", "name_zh", "NameZH", "title", "Title", "variation", "Variation"} {
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
	keys := []string{
		"id", "ID", "code", "Code", "title", "Title", "name", "Name", "name_zh", "NameZH",
		"question", "Question", "answer", "Answer", "keywords", "Keywords",
		"symptoms", "Symptoms", "content", "Content", "gene", "Gene",
		"disease", "Disease", "relation", "Relation", "category", "Category",
		"department", "Department", "description", "Description",
		"definition", "Definition", "head", "Head", "entity1", "Entity1",
		"entity2", "Entity2", "variation", "Variation", "synonyms", "Synonyms",
	}
	var b strings.Builder
	for _, k := range keys {
		if v, ok := m[k]; ok {
			b.WriteString(valueToString(v))
			b.WriteString(" ")
		}
	}
	if b.Len() == 0 {
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
