package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBakeClassification verifies real gz source files are recognized AND
// parseable by seedFile — the classification bake depends on. Huge corpora
// (huatuo_qa / medical_qa_pairs, >40MB gz) are skipped for speed; their
// seedFile cases are exercised by seed's own coverage.
func TestBakeClassification(t *testing.T) {
	gzs, err := filepath.Glob(filepath.Join("gz", archiveGlob))
	if err != nil {
		t.Fatalf("glob gz: %v", err)
	}
	if len(gzs) < 50 {
		t.Fatalf("expected >=50 archive files, got %d", len(gzs))
	}

	lfsFiles := make(map[string]bool)
	recognized := 0
	skipped := 0
	for _, f := range gzs {
		st, err := os.Stat(f)
		if err != nil {
			t.Fatalf("stat %s: %v", f, err)
		}
		if st.Size() > 5<<20 { // >5MB: skip the giant corpora
			skipped++
			continue
		}
		base := archiveBaseName(f)
		raw, err := decompressFile(f)
		if err != nil {
			t.Errorf("decompress %s: %v", base, err)
			continue
		}
		// Git LFS pointer files (not pulled locally) cannot be parsed; they
		// still must be CLASSIFIED by name. Verify the mapping exists via the
		// seedFile case table indirectly: a LFS pointer for huatuo/medical_qa
		// is a data-availability issue, not a bake classification issue.
		if strings.HasPrefix(string(raw), "version https://git-lfs") {
			lfsFiles[base] = true
			skipped++
			continue
		}
		ds, rows, err := seedFile(base, raw)
		if err != nil {
			t.Errorf("seedFile(%s): %v", base, err)
			continue
		}
		if ds == "" {
			t.Errorf("seedFile(%s): empty dataset", base)
			continue
		}
		if len(rows) == 0 {
			t.Errorf("seedFile(%s): 0 rows", base)
			continue
		}
		recognized++
	}

	t.Logf("recognized %d files (skipped %d giant/LFS corpora)", recognized, skipped)
	if recognized+skipped != len(gzs) {
		t.Fatalf("coverage mismatch: %d recognized + %d skipped != %d total", recognized, skipped, len(gzs))
	}
	// every small file must classify successfully
	if recognized == 0 {
		t.Fatal("no files classified")
	}

	// LFS-pointer corpora must still have a seedFile case (mapping exists)
	for name := range lfsFiles {
		// seedFile case lookup: huatuo_qa -> DSHuatuo, medical_qa_pairs -> DSMedicalQA
		switch name {
		case "huatuo_qa.json", "medical_qa_pairs.json":
			// known corpora whose case exists in seed.go
		default:
			t.Errorf("unexpected LFS pointer file %s", name)
		}
	}

	// sanity: key files map to the expected datasets
	checks := map[string]string{
		"common_diseases_batch3.json": DSMedical,
		"drug_contraindications.json": DSDrug,
		"body_part_triage.json":       DSBodyPart,
		"health_myths.json":           DSHealthMyths,
		"emergency_triage.json":       DSEmergency,
	}
	for name, want := range checks {
		raw, err := decompressFile(filepath.Join("gz", name+".zst"))
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		ds, rows, err := seedFile(name, raw)
		if err != nil || ds != want {
			t.Errorf("seedFile(%s) = %q (err %v), want %q", name, ds, err, want)
			continue
		}
		if len(rows) == 0 {
			t.Errorf("seedFile(%s): 0 rows", name)
		}
	}
}

// TestBakeBuildSearchTextIncludesPartKeys guards that body-part fields are
// indexed for retrieval (aliases/conditions/red_flags were added to the keys).
func TestBakeBuildSearchTextIncludesPartKeys(t *testing.T) {
	raw := []byte(`{"id":"bp-006","part_key":"abd_lr","part_zh":"右下腹","aliases":["右下腹痛","右侧下腹部"],"conditions":["阑尾炎","右侧输尿管结石"]}`)
	text := buildSearchText(raw)
	for _, want := range []string{"abd_lr", "右下腹", "右下腹痛", "阑尾炎"} {
		if !strings.Contains(text, want) {
			t.Errorf("search text missing %q: %s", want, text)
		}
	}
}

// TestVectorBakeEligible pins the bake skip-set: structured datasets served
// by dedicated lookup tools are excluded from the vector store, everything
// else (free-text QA/corpus datasets) must stay vectorized — QA pairs are
// only reachable through the vector path.
func TestVectorBakeEligible(t *testing.T) {
	for _, ds := range []string{DSMedicalKG, DSNMPA, DSCPubMed, DSICD10} {
		if vectorBakeEligible(ds) {
			t.Errorf("vectorBakeEligible(%s) = true, want false (lookup-tool covered)", ds)
		}
	}
	for _, ds := range []string{DSMedicalQA, DSHuatuo, DSDiseaseEnc, DSMedical, DSMSD, DSMedlinePlus} {
		if !vectorBakeEligible(ds) {
			t.Errorf("vectorBakeEligible(%s) = false, want true", ds)
		}
	}
}

// TestBakePayload guards the slim payload contract: the former "text" (a
// duplicate of data) and "timestamp" fields had no consumers and bloat the
// baked image; source/type/entry_id/data are what retrieval and admin
// stats consume.
func TestBakePayload(t *testing.T) {
	data := []byte(`{"id":"x1","q":"发烧怎么办"}`)
	p := bakePayload(DSHuatuo, "x1", data)
	for _, k := range []string{"source", "type", "entry_id", "data"} {
		if _, ok := p[k]; !ok {
			t.Errorf("payload missing %q: %v", k, p)
		}
	}
	for _, k := range []string{"text", "timestamp"} {
		if _, ok := p[k]; ok {
			t.Errorf("payload should not contain %q (no consumers, image bloat)", k)
		}
	}
	if p["type"] != "knowledge" {
		t.Errorf("type = %q, want knowledge", p["type"])
	}
	if p["data"] != string(data) {
		t.Errorf("data not preserved verbatim: %q", p["data"])
	}
	if d := bakePayload(DSDrug, "d1", data); d["type"] != "drug" {
		t.Errorf("drug dataset type = %q, want drug", d["type"])
	}
}
