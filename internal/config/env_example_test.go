package config

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// envVarsReadByConfig extracts every environment variable name this package
// reads — via a getEnv* helper or os.Getenv — straight from config.go.
func envVarsReadByConfig(t *testing.T) map[string]bool {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "config.go", nil, 0)
	if err != nil {
		t.Fatalf("parse config.go: %v", err)
	}

	names := make(map[string]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		read := false
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			read = strings.HasPrefix(fn.Name, "getEnv")
		case *ast.SelectorExpr:
			read = fn.Sel.Name == "Getenv"
		}
		if !read || len(call.Args) == 0 {
			return true
		}
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if v, err := strconv.Unquote(lit.Value); err == nil {
				names[v] = true
			}
		}
		return true
	})
	if len(names) < 50 {
		t.Fatalf("only %d env vars found in config.go — the extractor stopped matching", len(names))
	}
	return names
}

// composeOnlyVars are set in .env.example because operators copy that file into
// place and docker-compose.yml consumes them; Go never reads them.
var composeOnlyVars = map[string]bool{
	"MARIA_DB_ROOT_PASSWORD": true,
	"QDRANT_IMAGE":           true,
}

func exampleKeys(t *testing.T) (uncommented, all map[string]bool) {
	t.Helper()

	path := filepath.Join("..", "..", ".env.example")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	uncommented = make(map[string]bool)
	all = make(map[string]bool)
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for lineNo := 1; sc.Scan(); lineNo++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		key, _, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "#")), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		all[key] = true
		if !strings.HasPrefix(line, "#") {
			uncommented[key] = true
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return uncommented, all
}

// TestEnvExampleAdvertisesOnlyRealVars: a key Go never reads is a knob that
// silently does nothing, which is worse than absent — the operator sets it and
// believes it took effect.
func TestEnvExampleAdvertisesOnlyRealVars(t *testing.T) {
	uncommented, _ := exampleKeys(t)
	read := envVarsReadByConfig(t)

	for key := range uncommented {
		if read[key] || composeOnlyVars[key] {
			continue
		}
		t.Errorf(".env.example sets %s, which config.go never reads", key)
	}
}

// TestEnvExampleCoversEveryLiveVar is the direction the 2026-10-04 audit found
// broken: seventeen variables Load() read that the example file never
// mentioned, including ADMIN_PASSWORD and PUBLIC_BASE_URL. Five of them were
// "read into a field nobody consumes"; those fields are gone (2026-10-06), so
// this check has no exemptions left.
func TestEnvExampleCoversEveryLiveVar(t *testing.T) {
	_, all := exampleKeys(t)
	read := envVarsReadByConfig(t)

	for name := range read {
		if all[name] {
			continue
		}
		t.Errorf("config.go reads %s but .env.example never mentions it", name)
	}
}
