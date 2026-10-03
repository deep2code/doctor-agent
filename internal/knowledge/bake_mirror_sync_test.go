package knowledge

// external/bake_onnx.py is a hand-copied mirror of this package's seeding rules:
// it is what bake-gpu.sh runs on the GPU box to build the Qdrant vector layer.
// A file it does not know is skipped *silently*, and a field missing from its key
// list is dropped from the embedded text — so a stale copy produces a smaller,
// keywords-only vector layer with nothing failing and no log line to notice.
// That drift actually happened (checked 2026-10-03): the mirror was frozen at
// knowledge 1.39 and was missing 47 of the 73 科普 sources (6,902 rows) plus the
// four prose keys. These assertions are the reason it cannot happen again.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const bakeMirrorPath = "../../external/bake_onnx.py"

var pyConstant = regexp.MustCompile(`(?m)^(DS_\w+) = "([^"]+)"`)

// pyBlock returns the literal body of a top-level Python list or dict assigned to
// name, so tests can read the mirror without running Python.
func pyBlock(t *testing.T, src, name string) string {
	t.Helper()
	i := strings.Index(src, name+" = ")
	if i < 0 {
		t.Fatalf("bake_onnx.py: %s not found (renamed or deleted?)", name)
	}
	open := src[i+len(name)+3]
	close := byte(']')
	if open == '{' {
		close = '}'
	} else if open != '[' {
		t.Fatalf("bake_onnx.py: %s is not a list or dict literal (got %q)", name, open)
	}
	end := strings.IndexByte(src[i:], close)
	if end < 0 {
		t.Fatalf("bake_onnx.py: unterminated literal for %s", name)
	}
	return src[i : i+end]
}

var quotedString = regexp.MustCompile(`"([^"]+)"`)

// pyStrings lists the string literals in a block, line by line so trailing
// comments are ignored (the mirror annotates entries with row counts like
// `# 354,752 rows — medical_kg_lookup`).
func pyStrings(block string) []string {
	var out []string
	for _, line := range strings.Split(block, "\n") {
		code := strings.SplitN(line, "#", 2)[0]
		for _, m := range quotedString.FindAllStringSubmatch(code, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

func pyConstants(src string) map[string]string {
	out := map[string]string{}
	for _, m := range pyConstant.FindAllStringSubmatch(src, -1) {
		out[m[1]] = m[2]
	}
	return out
}

func diffStrings(goNames, pyNames []string) (missing, extra []string) {
	have := map[string]bool{}
	for _, n := range pyNames {
		have[n] = true
	}
	for _, n := range goNames {
		if !have[n] {
			missing = append(missing, n)
		}
	}
	seen := map[string]bool{}
	for _, n := range goNames {
		seen[n] = true
	}
	for _, n := range pyNames {
		if !seen[n] {
			extra = append(extra, n)
		}
	}
	return missing, extra
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestBakeMirrorMatchesGoSeedLists(t *testing.T) {
	raw, err := os.ReadFile(bakeMirrorPath)
	if err != nil {
		t.Fatalf("reading the bake mirror: %v", err)
	}
	src := string(raw)
	consts := pyConstants(src)

	t.Run("medical seed files", func(t *testing.T) {
		goFiles := make([]string, 0, len(medicalSeedFiles))
		for f := range medicalSeedFiles {
			goFiles = append(goFiles, f)
		}
		py := pyStrings(pyBlock(t, src, "SEED_LIST_MEDICAL_FILES"))
		missing, extra := diffStrings(sortedCopy(goFiles), sortedCopy(py))
		if len(missing)+len(extra) > 0 {
			t.Errorf("bake_onnx.py SEED_LIST_MEDICAL_FILES != medicalSeedFiles\n"+
				"  not baked by the GPU run (would be skipped silently): %v\n  unknown to Go: %v",
				missing, extra)
		}
	})

	t.Run("other seedList datasets", func(t *testing.T) {
		block := pyBlock(t, src, "SEED_LIST_OTHER_FILES")
		pyMap := map[string]string{}
		for _, m := range regexp.MustCompile(`"([^"]+\.json)":\s*(DS_\w+)`).FindAllStringSubmatch(block, -1) {
			pyMap[m[1]] = consts[m[2]]
		}
		for f, ds := range seedListDatasets {
			if !strings.Contains(block, `"`+f+`"`) {
				t.Errorf("bake_onnx.py SEED_LIST_OTHER_FILES is missing %s (%s)", f, ds)
			} else if pyMap[f] != ds {
				t.Errorf("bake_onnx.py maps %s to %q, Go maps it to %q", f, pyMap[f], ds)
			}
		}
		for f := range pyMap {
			if _, ok := seedListDatasets[f]; !ok {
				t.Errorf("bake_onnx.py maps %s, which Go does not seed this way", f)
			}
		}
	})

	// Order is part of the contract: the baked text is truncated to MaxTextChars
	// runes, so a reordered list silently changes which content survives.
	t.Run("search text keys", func(t *testing.T) {
		py := pyStrings(pyBlock(t, src, "BUILD_SEARCH_KEYS"))
		if strings.Join(py, "|") != strings.Join(searchTextKeys, "|") {
			missing, extra := diffStrings(searchTextKeys, py)
			t.Errorf("bake_onnx.py BUILD_SEARCH_KEYS != searchTextKeys (order matters: %d vs %d entries)\n"+
				"  missing from the mirror: %v\n  extra in the mirror: %v",
				len(py), len(searchTextKeys), missing, extra)
		}
	})

	t.Run("extract key fields", func(t *testing.T) {
		py := pyStrings(pyBlock(t, src, "EXTRACT_KEY_PRIORITY"))
		if strings.Join(py, "|") != strings.Join(extractKeyFields, "|") {
			t.Errorf("bake_onnx.py EXTRACT_KEY_PRIORITY != extractKeyFields: %v vs %v", py, extractKeyFields)
		}
	})

	t.Run("vector skip datasets", func(t *testing.T) {
		goDS := make([]string, 0, len(vectorSkipDatasets))
		for ds := range vectorSkipDatasets {
			goDS = append(goDS, ds)
		}
		py := pyStrings(pyBlock(t, src, "VECTOR_SKIP_DATASETS"))
		missing, extra := diffStrings(sortedCopy(goDS), sortedCopy(py))
		if len(missing)+len(extra) > 0 {
			t.Errorf("bake_onnx.py VECTOR_SKIP_DATASETS != vectorSkipDatasets\n"+
				"  the GPU run would vectorize: %v\n  the GPU run skips what Go bakes: %v", missing, extra)
		}
	})
}
