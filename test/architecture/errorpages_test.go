// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestEveryWireCodeHasItsPage holds the error pages of the website to the codes the API answers
// with: a problem's `docs` link points at /errors/<code>/, so a code without its entry in
// apps/website/src/lib/errors/codes.json is a link into nothing, and an entry for a code the
// server never sends is a page about something that does not happen. The wire codes are the
// literals given to New(Category, code) in core and presentation, and the constants a Problem
// literal in presentation/rest names as its Code.
func TestEveryWireCodeHasItsPage(t *testing.T) {
	wire := map[string]string{}
	consts := map[string]string{}
	var problemCodes []string

	forEachGoFile(t, []string{"../../core", "../../presentation"},
		func(path string, file *ast.File, fset *token.FileSet) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			ast.Inspect(file, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.ValueSpec:
					for i, name := range n.Names {
						if i < len(n.Values) {
							if lit, ok := n.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
								consts[name.Name], _ = strconv.Unquote(lit.Value)
							}
						}
					}
				case *ast.CallExpr:
					if calledName(n.Fun) != "New" || len(n.Args) != 2 {
						return true
					}
					if sel, ok := n.Args[0].(*ast.SelectorExpr); !ok || !strings.HasPrefix(sel.Sel.Name, "Category") {
						if id, ok := n.Args[0].(*ast.Ident); !ok || !strings.HasPrefix(id.Name, "Category") {
							return true
						}
					}
					if lit, ok := n.Args[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						code, _ := strconv.Unquote(lit.Value)
						wire[code] = rel(path) + ":" + strconv.Itoa(fset.Position(lit.Pos()).Line)
					}
				case *ast.CompositeLit:
					if typeName(n.Type) != "Problem" {
						return true
					}
					for _, element := range n.Elts {
						kv, ok := element.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Code" {
							if id, ok := kv.Value.(*ast.Ident); ok {
								problemCodes = append(problemCodes, id.Name+"@"+rel(path))
							}
						}
					}
				}
				return true
			})
		})
	for _, entry := range problemCodes {
		name, at, _ := strings.Cut(entry, "@")
		code, ok := consts[name]
		if !ok {
			t.Fatalf("%s: a Problem's Code names %s, which is no string constant this gate can read", at, name)
		}
		wire[code] = at
	}
	if len(wire) < 10 {
		t.Fatalf("found only %d wire codes; the gate no longer sees where codes are made", len(wire))
	}

	raw, err := os.ReadFile("../../apps/website/src/lib/errors/codes.json")
	if err != nil {
		t.Fatal(err)
	}
	var pages struct {
		Codes []struct {
			Code   string `json:"code"`
			Status int    `json:"status"`
		} `json:"codes"`
	}
	if err := json.Unmarshal(raw, &pages); err != nil {
		t.Fatal(err)
	}
	paged := map[string]bool{}
	for _, page := range pages.Codes {
		if paged[page.Code] {
			t.Errorf("codes.json lists %s twice", page.Code)
		}
		paged[page.Code] = true
	}

	var missing, stray []string
	for code, at := range wire {
		if !paged[code] {
			missing = append(missing, code+" ("+at+")")
		}
	}
	for code := range paged {
		if _, ok := wire[code]; !ok {
			stray = append(stray, code)
		}
	}
	sort.Strings(missing)
	sort.Strings(stray)
	for _, code := range missing {
		t.Errorf("wire code %s has no entry in apps/website/src/lib/errors/codes.json", code)
	}
	for _, code := range stray {
		t.Errorf("codes.json has a page for %s, which the server never answers with", code)
	}
}
