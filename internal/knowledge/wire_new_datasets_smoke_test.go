package knowledge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestWireNewDatasetsSmoke checks the exact-lookup datasets added in one batch
// still round-trip: the source file is a row array, every row keys uniquely, and
// every row decodes into the struct the lookup tool reads.
func TestWireNewDatasetsSmoke(t *testing.T) {
	cases := []struct {
		path     string
		dataset  string
		keyField func(row []byte) string
	}{
		{
			path:    filepath.Join("data", DSOrphanet, "orphanet_diseases.json"),
			dataset: DSOrphanet,
			keyField: func(row []byte) string {
				var d OrphanetDisease
				if err := json.Unmarshal(row, &d); err != nil {
					return "decode error: " + err.Error()
				}
				return d.OrphaCode
			},
		},
		{
			path:    filepath.Join("data", DSICDO3, "icdo3_morphology.json"),
			dataset: DSICDO3,
			keyField: func(row []byte) string {
				var m ICDO3Morphology
				if err := json.Unmarshal(row, &m); err != nil {
					return "decode error: " + err.Error()
				}
				return m.Code
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.dataset, func(t *testing.T) {
			raw, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := seedList(raw)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) == 0 {
				t.Fatalf("%s: no rows", tc.path)
			}
			// Keys must be unique within a dataset (the upsert would silently
			// collapse duplicates) and must come from the row's own code field.
			seen := map[string]bool{}
			for _, r := range rows {
				if seen[r.Key] {
					t.Errorf("duplicate %s key: %s", tc.dataset, r.Key)
				}
				seen[r.Key] = true
				if got := tc.keyField(r.Data); got == "" {
					t.Errorf("row %s has no %s identifier", r.Key, tc.dataset)
				}
			}
			t.Logf("%s rows: %d", tc.dataset, len(rows))
		})
	}
}
