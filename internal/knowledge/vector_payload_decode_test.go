package knowledge

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// This file is the gate for the vector leg's payload decoder. Baked Qdrant
// points carry the verbatim seed row in payload["data"], and a row that does not
// decode into a usable KnowledgeEntry is not an error anywhere — it is simply
// never returned, so the paid bake silently stops recalling that whole dataset.
// That is exactly how 7,580 prose pages, 8,807 disease cards and 4,425 abstracts
// were dead until the projections in bakedProjections existed.
//
// Everything here is offline: it reads gz/ and package source, never MariaDB.
//
//  1. every dataset with a declared projection decodes 100% of its rows into an
//     entry that carries displayable text, and no bake-eligible row fails to
//     decode as a JSON object (the shape drift — a renamed field, an envelope —
//     that turns a dataset into dead embedding spend);
//  2. no two rows in the collection share an entry id: HybridRetriever's RRF
//     fusion dedupes strictly by id, so a clash double-lists one article, and a
//     load-order id (the old "msd-0001") cannot be re-derived from a payload at
//     all, which made the two legs disagree about what the same article is;
//  3. every projector the keyword leg uses has a bakedProjections entry, read
//     from source rather than from a hand-kept list, so a family cannot be
//     visible to one retrieval leg and invisible to the other.
//
// Rows that decode cleanly but hold no prose are the exact-lookup tables
// (medins/clinvar/eml/fda/…); they left the bake tree on 2026-10-07 via
// vectorSkipDatasets in bake.go, so scanBakeTree no longer feeds them here and
// every row this gate does walk is expected to be returnable. A dataset with a
// declared projection that returns less than all of its rows is still the
// failure this gate exists to catch.
func TestBakedPayloadDecodesEveryProjectedRow(t *testing.T) {
	stats := scanBakeTree(t)

	names := make([]string, 0, len(stats))
	for ds := range stats {
		names = append(names, ds)
	}
	sort.Strings(names)

	for _, name := range names {
		s := stats[name]
		_, projected := bakedProjections[name]

		if projected && s.returned != s.rows {
			t.Errorf("dataset %s has a projection but only %d/%d rows decode with text%s",
				name, s.returned, s.rows, s.detail())
		}
		if s.badShape > 0 {
			t.Errorf("dataset %s: %d/%d rows are not decodable JSON objects%s",
				name, s.badShape, s.rows, s.detail())
		}
		if s.clash > 0 {
			t.Errorf("dataset %s: %d/%d rows share an entry id with another row%s",
				name, s.clash, s.rows, s.detail())
		}
	}
}

func TestEveryLegSharedProjectionIsBaked(t *testing.T) {
	keyword := projectorCallsInStoreGetters(t)
	vector := projectorCallsInBakedProjections(t)

	if len(keyword) == 0 {
		t.Fatal("found no projectXxx helper called by a *Store ...AsKnowledge getter — " +
			"the extractor is broken, so this gate proves nothing")
	}

	var missing []string
	for name := range keyword {
		if !vector[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("bakedProjections has no entry for %s — the keyword leg projects these "+
			"families, so the vector leg silently cannot return them", strings.Join(missing, ", "))
	}
}

type payloadStat struct {
	rows     int
	returned int
	badShape int
	clash    int
	problem  string
}

func (s *payloadStat) note(key, msg string) {
	if s.problem == "" {
		s.problem = key + ": " + msg
	}
}

func (s *payloadStat) detail() string {
	if s.problem == "" {
		return ""
	}
	return " — first: " + s.problem
}

// scanBakeTree feeds every bake-eligible seed row through the production
// payload builder and the production decoder.
func scanBakeTree(t *testing.T) map[string]*payloadStat {
	t.Helper()

	archives, err := listSeedArchives("gz")
	if err != nil {
		t.Fatalf("list seed archives: %v", err)
	}

	stats := map[string]*payloadStat{}
	seenID := map[string]string{}

	for _, a := range archives {
		ds := a.Dataset
		if !vectorBakeEligible(ds) {
			continue
		}
		s := stats[ds]
		if s == nil {
			s = &payloadStat{}
			stats[ds] = s
		}
		rows, err := a.rows()
		if err != nil {
			t.Fatalf("%s: %v", a.Path, err)
		}
		for _, r := range rows {
			payload := bakePayload(ds, r.Key, r.Data)
			s.rows++

			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(payload["data"]), &fields); err != nil {
				s.badShape++
				s.note(r.Key, fmt.Sprintf("row is not a JSON object: %v", err))
				continue
			}

			e, ok := decodeBakedPayload(payload)
			if !ok {
				continue
			}
			if prev, dup := seenID[e.ID]; dup {
				s.clash++
				s.note(r.Key, fmt.Sprintf("id %s shared by %s and %s/%s", e.ID, prev, ds, r.Key))
				continue
			}
			seenID[e.ID] = ds + "/" + r.Key
			s.returned++
		}
	}
	return stats
}

func parsePackageFile(t *testing.T, name string) *ast.File {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return file
}

// collectProjectorCalls returns the names of every projectXxx helper called
// inside root.
func collectProjectorCalls(root ast.Node) map[string]bool {
	found := map[string]bool{}
	ast.Inspect(root, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, ok := call.Fun.(*ast.Ident)
		if !ok || !strings.HasPrefix(fn.Name, "project") {
			return true
		}
		found[fn.Name] = true
		return true
	})
	return found
}

// projectorCallsInStoreGetters reads loader.go: the *Store getters that hand the
// keyword leg its projected entries, and the projectors each one calls.
func projectorCallsInStoreGetters(t *testing.T) map[string]bool {
	t.Helper()

	all := map[string]bool{}
	for _, decl := range parsePackageFile(t, "loader.go").Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !strings.HasSuffix(fn.Name.Name, "AsKnowledge") || !isStoreMethod(fn) {
			continue
		}
		for name := range collectProjectorCalls(fn.Body) {
			all[name] = true
		}
	}
	return all
}

// projectorCallsInBakedProjections reads the bakedProjections literal in
// retriever_vector.go: the projector each dataset entry's closure calls.
func projectorCallsInBakedProjections(t *testing.T) map[string]bool {
	t.Helper()

	all := map[string]bool{}
	for _, decl := range parsePackageFile(t, "retriever_vector.go").Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) == 0 || len(vs.Values) == 0 {
				continue
			}
			if vs.Names[0].Name != "bakedProjections" {
				continue
			}
			for name := range collectProjectorCalls(vs.Values[0]) {
				all[name] = true
			}
		}
	}
	return all
}

func isStoreMethod(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return false
	}
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	id, ok := star.X.(*ast.Ident)
	return ok && id.Name == "Store"
}
