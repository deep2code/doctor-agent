package knowledge

import (
	"os"
	"strings"
	"testing"
)

// TestBakeClassification verifies the gz tree the bake reads is complete and
// parseable: every archive sits in a <dataset>/ directory, decompresses, and
// yields rows. Giant corpora (>5MB) are skipped for speed — seeding the dev DB
// exercises them, and their sources are LFS pointers or .parts splits on disk.
func TestBakeClassification(t *testing.T) {
	archives, err := listSeedArchives("gz")
	if err != nil {
		t.Fatalf("listing the gz tree: %v", err)
	}
	if len(archives) < 50 {
		t.Fatalf("expected >=50 archive files, got %d", len(archives))
	}

	recognized, skipped := 0, 0
	for _, a := range archives {
		st, err := os.Stat(a.Path)
		if err != nil {
			t.Fatalf("stat %s: %v", a.Path, err)
		}
		if st.Size() > 5<<20 { // >5MB: skip the giant corpora
			skipped++
			continue
		}
		raw, err := decompressFile(a.Path)
		if err != nil {
			t.Errorf("decompress %s: %v", a.Path, err)
			continue
		}
		// A Git-LFS pointer is a data-availability issue on this machine, not a
		// tree-layout one: the archive still sits in the right directory.
		if strings.HasPrefix(string(raw), "version https://git-lfs") {
			skipped++
			continue
		}
		rows, err := seedList(raw)
		if err != nil {
			t.Errorf("%s/%s: %v", a.Dataset, a.Base, err)
			continue
		}
		if len(rows) == 0 {
			t.Errorf("%s/%s: 0 rows", a.Dataset, a.Base)
			continue
		}
		recognized++
	}

	t.Logf("parsed %d archives (skipped %d giant/LFS corpora)", recognized, skipped)
	if recognized == 0 {
		t.Fatal("no archives parsed")
	}

	// sanity: files land in the dataset directories the loaders read from
	checks := map[string]string{
		"medical/common_diseases_batch3.json": DSMedical,
		"drug/drug_contraindications.json":    DSDrug,
		"bodypart/body_part_triage.json":      DSBodyPart,
		"healthmyths/health_myths.json":       DSHealthMyths,
		"emergency/emergency_triage.json":     DSEmergency,
	}
	byRel := make(map[string]seedArchive, len(archives))
	for _, a := range archives {
		byRel[a.Dataset+"/"+a.Base] = a
	}
	for rel, want := range checks {
		a, ok := byRel[rel]
		if !ok {
			t.Errorf("%s is not in the scanned tree (renamed or misfiled?)", rel)
			continue
		}
		if a.Dataset != want {
			t.Errorf("%s sits in dataset %q, want %q", rel, a.Dataset, want)
		}
		raw, err := decompressFile(a.Path)
		if err != nil {
			t.Errorf("read %s: %v", rel, err)
			continue
		}
		rows, err := seedList(raw)
		if err != nil {
			t.Errorf("seedList(%s): %v", rel, err)
			continue
		}
		if len(rows) == 0 {
			t.Errorf("%s: 0 rows", rel)
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
