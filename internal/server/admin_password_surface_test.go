package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// httpMethodNames maps the stdlib selector names used in a `switch r.Method`
// to the HTTP verb they stand for.
var httpMethodNames = map[string]string{
	"MethodConnect": "CONNECT",
	"MethodDelete":  "DELETE",
	"MethodGet":     "GET",
	"MethodHead":    "HEAD",
	"MethodOptions": "OPTIONS",
	"MethodPatch":   "PATCH",
	"MethodPost":    "POST",
	"MethodPut":     "PUT",
	"MethodTrace":   "TRACE",
}

// TestAdminUsersSurfaceHasNoPasswordChange pins the fact two pieces of operator
// advice rest on: /admin can read and delete a user but cannot rotate a
// password, so the ADMIN_PASSWORD warning (main.go) and .env.example tell
// operators to write the generated password into .env or edit the users table,
// instead of promising a 修改密码 button that does not exist.
//
// Adding a real change-password path should turn this gate red on purpose —
// then update the hint and the docs along with the new route.
func TestAdminUsersSurfaceHasNoPasswordChange(t *testing.T) {
	verbs := adminUserHandlerVerbs(t)
	if len(verbs) == 0 {
		t.Fatal("no `switch r.Method` found in handleAdminUser — the extractor stopped matching")
	}
	for _, verb := range []string{"GET", "DELETE"} {
		if !verbs[verb] {
			t.Errorf("handleAdminUser no longer handles %s, which the docs describe as the whole surface", verb)
		}
	}
	for _, verb := range []string{"PUT", "PATCH", "POST"} {
		if verbs[verb] {
			t.Errorf("handleAdminUser now answers %s: if this is a password change, update the ADMIN_PASSWORD hint in main.go and .env.example, then expect this verb here", verb)
		}
	}

	for _, name := range authPasswordMutators(t) {
		t.Errorf("internal/auth exposes %s: a password-change path exists, so the startup hint and .env.example must stop telling operators to edit the users table", name)
	}
}

// adminUserHandlerVerbs reads the method set of handleAdminUser straight from
// source, so the gate needs no database and no running server.
func adminUserHandlerVerbs(t *testing.T) map[string]bool {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "server.go", nil, 0)
	if err != nil {
		t.Fatalf("parse server.go: %v", err)
	}

	verbs := make(map[string]bool)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "handleAdminUser" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok || !isMethodSelector(sw.Tag, "r", "Method") {
				return true
			}
			for _, stmt := range sw.Body.List {
				cc, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, e := range cc.List {
					if sel, ok := e.(*ast.SelectorExpr); ok {
						if verb, ok := httpMethodNames[sel.Sel.Name]; ok {
							verbs[verb] = true
						}
					}
				}
			}
			return false
		})
	}
	return verbs
}

// authPasswordMutators lists exported methods defined in internal/auth whose
// name contains Password — i.e. the API surface a change-password path would
// have to live on.
func authPasswordMutators(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir("../auth")
	if err != nil {
		t.Fatalf("read ../auth: %v", err)
	}

	fset := token.NewFileSet()
	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, "../auth/"+name, nil, 0)
		if err != nil {
			t.Fatalf("parse ../auth/%s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !fn.Name.IsExported() {
				continue
			}
			if strings.Contains(fn.Name.Name, "Password") {
				names = append(names, fn.Name.Name)
			}
		}
	}
	return names
}

func isMethodSelector(e ast.Expr, recv, field string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == recv && sel.Sel.Name == field
}
