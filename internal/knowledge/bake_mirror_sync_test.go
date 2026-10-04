package knowledge

// external/bake_onnx.py is a hand-copied mirror of this package's seeding rules:
// it is what bake-gpu.sh runs on the GPU box to build the Qdrant vector layer.
// Since the seed tree became self-classifying (data/<dataset>/<name>.json, one
// JSON array per file, one element per row) there are no per-file lists left to
// drift: the mirror walks the same directories and applies the same rule. What
// still has to match exactly is the content of each row — the key that
// identifies it and the text that gets embedded — so those two lists are pinned
// here. A stale copy of either produces a smaller or keywords-only vector layer
// with no failing log line, which is exactly what happened once (checked
// 2026-10-03: the mirror was frozen at knowledge 1.39, missing 47 of the 73
// 科普 sources and the four prose keys).
import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const bakeMirrorPath = "../../external/bake_onnx.py"

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

func TestBakeMirrorMatchesGoSeedRules(t *testing.T) {
	raw, err := os.ReadFile(bakeMirrorPath)
	if err != nil {
		t.Fatalf("reading the bake mirror: %v", err)
	}
	src := string(raw)

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
