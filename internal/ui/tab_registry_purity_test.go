package ui

// TestRegistryStaysDeclarative is the "registry is a registry"
// ratchet. tab_registry.go is the app's dispatch table AND the spec
// every guardrail test + the inventory generator walk — logic hiding
// inside inline closure bodies is invisible to all of them. Entries
// must point at named hooks (tab_*_hooks.go etc.); inline closures
// are for one-line glue only.
//
// The ceiling is body LINE COUNT per function literal inside
// tabRegistry(). Raise it only with a written justification; the
// right fix is always extracting a named method instead.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

const registryClosureMaxLines = 5

func TestRegistryStaysDeclarative(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "tab_registry.go", nil, 0)
	if err != nil {
		t.Fatalf("parse tab_registry.go: %v", err)
	}

	var registryFn *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == "tabRegistry" {
			registryFn = fd
			break
		}
	}
	if registryFn == nil {
		t.Fatal("tabRegistry() not found — if it was renamed, update this test")
	}

	var violations []string
	ast.Inspect(registryFn.Body, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		start := fset.Position(lit.Body.Lbrace).Line
		end := fset.Position(lit.Body.Rbrace).Line
		// Body lines between the braces (a one-liner has start==end).
		body := end - start - 1
		if body < 0 {
			body = 0
		}
		if body > registryClosureMaxLines {
			violations = append(violations, fmt.Sprintf(
				"tab_registry.go:%d closure body is %d lines (max %d) — extract a named hook",
				start, body, registryClosureMaxLines))
		}
		// Don't descend into the literal: nested literals inside an
		// already-flagged body would double-report.
		return false
	})

	for _, v := range violations {
		t.Error(v)
	}
}
