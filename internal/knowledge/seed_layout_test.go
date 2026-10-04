package knowledge

// The seed tree has exactly two rules now:
//
//	data/<dataset>/<name>.json    one JSON array per file, one element per row
//	gz/<dataset>/<name>.json.zst  that file, compressed
//
// Nothing is registered anywhere: the directory name is the dataset, and the
// directory listing is the file list. These gates hold that shape in place,
// because every way it can decay is silent — a flat file under data/ is simply
// not seen, a file whose root is an object seeds zero rows, and a dataset
// directory nobody reads keeps its rows in MariaDB while retrieval stays empty.
//
// A new prose source therefore means: create data/<dataset>/ and drop the array
// in it, run make_gz, seed. No Go edit — except that loader.go's ingest switch
// must know the dataset name, which is what TestSeedTreeDatasetsAreLoaded pins.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// dsConsts maps a dataset's directory name to the Go identifier for it, read
// from kb.go's const block rather than repeated here.
func dsConsts(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("kb.go")
	if err != nil {
		t.Fatalf("reading kb.go: %v", err)
	}
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\t(DS\w+)\s*=\s*"([^"]+)"`).FindAllStringSubmatch(string(raw), -1) {
		out[m[2]] = m[1]
	}
	if len(out) < 30 {
		t.Fatalf("kb.go: found only %d dataset constants, the regex no longer matches the block", len(out))
	}
	return out
}

// ingestedDatasets returns the dataset names loader.go's ingest switch handles.
func ingestedDatasets(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("loader.go")
	if err != nil {
		t.Fatalf("reading loader.go: %v", err)
	}
	src := string(raw)
	i := strings.Index(src, "func (s *Store) ingest(")
	if i < 0 {
		t.Fatal("loader.go: ingest() not found (renamed?)")
	}
	end := strings.Index(src[i+10:], "\nfunc ")
	if end < 0 {
		t.Fatal("loader.go: unterminated ingest()")
	}
	body := src[i : i+10+end]

	consts := dsConsts(t)
	byIdent := map[string]string{}
	for name, ident := range consts {
		byIdent[ident] = name
	}
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\tcase (DS\w+):`).FindAllStringSubmatch(body, -1) {
		name, ok := byIdent[m[1]]
		if !ok {
			t.Fatalf("loader.go ingests %s, which kb.go does not define", m[1])
		}
		out[name] = true
	}
	if len(out) < 20 {
		t.Fatalf("loader.go: ingest() handles only %d datasets — the case-regex no longer "+
			"matches the switch, so this test would report a wrong list", len(out))
	}
	return out
}

// filesByDataset lists the seed sources under root, keyed by dataset directory.
// It also returns the strays sitting at the root, which must always be empty.
func filesByDataset(t *testing.T, root string) (map[string][]string, []string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading %s: %v", root, err)
	}
	out := map[string][]string{}
	var strays []string
	for _, e := range entries {
		if e.IsDir() {
			files, err := os.ReadDir(filepath.Join(root, e.Name()))
			if err != nil {
				t.Fatalf("reading %s/%s: %v", root, e.Name(), err)
			}
			for _, f := range files {
				if !f.IsDir() {
					out[e.Name()] = append(out[e.Name()], f.Name())
				}
			}
			continue
		}
		if strings.HasSuffix(e.Name(), ".json") || isArchive(e.Name()) {
			strays = append(strays, e.Name())
		}
	}
	return out, strays
}

func TestSeedTreeHasNoFlatSources(t *testing.T) {
	for _, root := range []string{"data", "gz"} {
		if _, strays := filesByDataset(t, root); len(strays) > 0 {
			t.Errorf("%s/ holds files outside a <dataset>/ directory: %v\n"+
				"  the seed tree reads datasets by directory name, so these are ignored in silence", root, strays)
		}
	}
}

// TestSeedTreeDatasetsAreLoaded is the registry that used to be three hand-kept
// file lists: a dataset directory must be one the loader knows, and the gz tree
// must be the data tree.
func TestSeedTreeDatasetsAreLoaded(t *testing.T) {
	known := ingestedDatasets(t)
	consts := dsConsts(t)

	dataFiles, _ := filesByDataset(t, "data")
	gzFiles, _ := filesByDataset(t, "gz")

	for ds := range dataFiles {
		if !known[ds] {
			t.Errorf("data/%s/ is seeded but loader.go's ingest() ignores it: rows go into MariaDB and are never read\n"+
				"  (add `case %s:` there, or rename the directory to an existing dataset)", ds, consts[ds])
		}
	}
	var extra []string
	for ds := range gzFiles {
		if _, ok := dataFiles[ds]; !ok {
			extra = append(extra, ds)
		}
	}
	for ds := range dataFiles {
		if _, ok := gzFiles[ds]; !ok {
			t.Errorf("data/%s/ has no gz/%s/ archive: it will never reach a fresh deploy\n"+
				"  run python3 external/make_gz.py", ds, ds)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		t.Errorf("gz/ holds datasets with no data/ source: %v (a stale archive; delete it with its dataset)", extra)
	}

	// Every source file must have its archive, matched by name.
	splitRe := regexp.MustCompile(`\.json\.part\d+$`)
	for ds, files := range dataFiles {
		for _, f := range files {
			switch {
			case splitRe.MatchString(f):
				// a slice of a >90MiB source; the manifest names the whole file
				continue
			case strings.HasSuffix(f, ".parts"):
				f = strings.TrimSuffix(f, ".parts") // split source; the merged archive carries the whole name
			case strings.HasSuffix(f, ".json"):
			default:
				t.Errorf("data/%s/%s: only .json sources (and their .partNNN splits) belong in the seed tree", ds, f)
				continue
			}
			if !hasGzArchive(gzFiles[ds], f) {
				t.Errorf("data/%s/%s has no gz/%s/%s.json.zst — run python3 external/make_gz.py", ds, f, ds, f)
			}
		}
	}
}

func hasGzArchive(archives []string, sourceName string) bool {
	for _, a := range archives {
		if strings.TrimSuffix(strings.TrimSuffix(a, ".zst"), ".gz") == sourceName {
			return true
		}
	}
	return false
}

// TestSeedSourcesAreArrays checks the one content rule the seeder has: each
// source is a top-level JSON array. Only the opening token is read, so this
// stays cheap on the 343MB QA corpus.
func TestSeedSourcesAreArrays(t *testing.T) {
	dataFiles, _ := filesByDataset(t, "data")
	for ds, files := range dataFiles {
		for _, f := range files {
			if !strings.HasSuffix(f, ".json") {
				continue
			}
			path := filepath.Join("data", ds, f)
			rootToken(t, path)
		}
	}
}

func rootToken(t *testing.T, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Errorf("%s: %v", path, err)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("%s: closing: %v", path, err)
		}
	}()
	tok, err := json.NewDecoder(f).Token()
	if err != nil {
		t.Errorf("%s: not readable as JSON: %v", path, err)
		return
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '[' {
		t.Errorf("%s: root is %v, want a JSON array (one element per row)", path, tok)
	}
}

// TestSeedListRejectsNonArrays covers the failure the format rule exists to
// prevent: an object-shaped source used to be silently accepted by one of the
// three parse modes.
func TestSeedListRejectsNonArrays(t *testing.T) {
	for _, in := range []string{`{"id":"x1"}`, `null`, `[]`, `"text"`} {
		rows, err := seedList([]byte(in))
		if err == nil {
			t.Errorf("seedList(%s) returned %d rows and no error, want a parse error", in, len(rows))
		}
	}
	rows, err := seedList([]byte(`[{"id":"x1"},{"id":"x2"}]`))
	if err != nil || len(rows) != 2 {
		t.Errorf("seedList of a 2-element array = %d rows, err %v", len(rows), err)
	}
}
