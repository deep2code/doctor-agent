package knowledge

import (
	"encoding/json"
	"os"
	"testing"
)

func TestWireNewDatasetsSmoke(t *testing.T) {
	raw, err := os.ReadFile("data/orphanet_diseases.json")
	if err != nil {
		t.Fatal(err)
	}
	ds, rows, err := seedFile("orphanet_diseases.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if ds != DSOrphanet {
		t.Fatalf("dataset = %q", ds)
	}
	t.Logf("orphanet rows: %d", len(rows))
	var oset OrphanetDiseaseSet
	if err := json.Unmarshal(raw, &oset); err != nil {
		t.Fatal(err)
	}
	if len(oset.Diseases) == 0 || oset.Diseases[0].OrphaCode == "" {
		t.Fatal("empty orphanet set")
	}

	raw2, err := os.ReadFile("data/icdo3_morphology.json")
	if err != nil {
		t.Fatal(err)
	}
	ds2, rows2, err := seedFile("icdo3_morphology.json", raw2)
	if err != nil {
		t.Fatal(err)
	}
	if ds2 != DSICDO3 {
		t.Fatalf("dataset2 = %q", ds2)
	}
	t.Logf("icdo3 rows: %d", len(rows2))
	var iset ICDO3MorphologySet
	if err := json.Unmarshal(raw2, &iset); err != nil {
		t.Fatal(err)
	}
	if len(iset.Terms) == 0 || iset.Terms[0].Code == "" {
		t.Fatal("empty icdo3 set")
	}

	// keys must be unique per dataset (upsert would silently collapse dupes)
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.Key] {
			t.Errorf("duplicate orphanet key: %s", r.Key)
		}
		seen[r.Key] = true
	}
	seen2 := map[string]bool{}
	for _, r := range rows2 {
		if seen2[r.Key] {
			t.Errorf("duplicate icdo3 key: %s", r.Key)
		}
		seen2[r.Key] = true
	}
}
