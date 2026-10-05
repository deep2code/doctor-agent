package tools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// executeDatasetCases reads the `switch dataset` in KnowledgeSearch.Execute
// straight from source, so this gate proves the model-facing dataset list and
// the datasets the tool actually answers for are the same set — without
// needing a seeded MariaDB to call Execute.
func executeDatasetCases(t *testing.T) map[string]bool {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "knowledge_search.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse knowledge_search.go: %v", err)
	}

	cases := make(map[string]bool)
	seenSwitch := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Execute" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			if id, ok := sw.Tag.(*ast.Ident); !ok || id.Name != "dataset" {
				return true
			}
			seenSwitch = true
			for _, stmt := range sw.Body.List {
				cc, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, e := range cc.List {
					lit, ok := e.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					v, err := strconv.Unquote(lit.Value)
					if err != nil || v == "" {
						continue
					}
					cases[v] = true
				}
			}
			return false
		})
	}
	if !seenSwitch {
		t.Fatal("no `switch dataset` in Execute — update this gate alongside the switch")
	}
	return cases
}

func TestDatasetTableMatchesExecuteSwitch(t *testing.T) {
	cases := executeDatasetCases(t)

	table := make(map[string]bool)
	for _, d := range knowledgeSearchDatasets {
		if d.label == "" {
			t.Errorf("dataset %q has an empty label, so the model sees a bare name", d.name)
		}
		if table[d.name] {
			t.Errorf("dataset %q is listed twice", d.name)
		}
		table[d.name] = true
		if !cases[d.name] {
			t.Errorf("dataset %q is advertised but Execute has no case for it", d.name)
		}
	}
	for name := range cases {
		if !table[name] {
			t.Errorf("Execute accepts dataset %q but the table never advertises it, so the model cannot know to ask", name)
		}
	}
}

func TestDescriptionAndSchemaAdvertiseEveryDataset(t *testing.T) {
	desc := (&KnowledgeSearch{}).Description()
	schema := (&KnowledgeSearch{}).Schema()
	dsProp, _ := schema["properties"].(map[string]any)
	ds, _ := dsProp["dataset"].(map[string]any)
	schemaDesc, _ := ds["description"].(string)

	for _, d := range knowledgeSearchDatasets {
		// Description carries "name=说明"; a bare name match would let a
		// dataset vanish from the label list unnoticed.
		if !strings.Contains(desc, d.name+"=") {
			t.Errorf("Description() omits dataset %q (or its label)", d.name)
		}
		if !strings.Contains(schemaDesc, d.name) {
			t.Errorf("Schema() dataset description omits %q", d.name)
		}
	}
}
