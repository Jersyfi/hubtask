// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestEveryDetailCodeAPersonCanMeetIsInTheCatalogue holds every detail code by its place rather
// than by its prefix: a string given to WithDetail, to shared.Validation or as a FieldError's Code
// in a non-test file has its sentence in locales/en.json (i18n-l10n.md §3). A prefix list holds
// only the products that existed when it was written; a new product's refusal would reach a person
// as the generic sentence or its humanised key. The one exception is a code on ErrInternal: a 500
// answers without its detail, so nobody reads it.
func TestEveryDetailCodeAPersonCanMeetIsInTheCatalogue(t *testing.T) {
	messages := loadCatalogue(t)
	missing := map[string][]string{}
	found := 0

	forEachGoFile(t, []string{"../../core", "../../infrastructure", "../../presentation", "../../cmd"},
		func(path string, file *ast.File, fset *token.FileSet) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			note := func(lit ast.Expr) {
				basic, ok := lit.(*ast.BasicLit)
				if !ok || basic.Kind != token.STRING {
					return
				}
				code, err := strconv.Unquote(basic.Value)
				if err != nil || code == "" {
					return
				}
				found++
				if _, ok := messages[code]; !ok {
					at := rel(path) + ":" + strconv.Itoa(fset.Position(basic.Pos()).Line)
					missing[code] = append(missing[code], at)
				}
			}
			ast.Inspect(file, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.CallExpr:
					switch name := calledName(n.Fun); {
					case name == "WithDetail" && len(n.Args) == 1 && !onInternal(n.Fun):
						note(n.Args[0])
					case name == "Validation" && len(n.Args) == 3:
						note(n.Args[1])
					}
				case *ast.CompositeLit:
					if typeName(n.Type) != "FieldError" {
						return true
					}
					for _, element := range n.Elts {
						if kv, ok := element.(*ast.KeyValueExpr); ok {
							if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Code" {
								note(kv.Value)
							}
						}
					}
				}
				return true
			})
		})

	if found == 0 {
		t.Fatal("no detail code found at all - the walk no longer recognises them")
	}
	codes := make([]string, 0, len(missing))
	for code := range missing {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		t.Errorf("the detail code %s (%s) is missing from locales/en.json", code, strings.Join(missing[code], ", "))
	}
}

func calledName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		return f.Sel.Name
	case *ast.Ident:
		return f.Name
	}
	return ""
}

func typeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.Ident:
		return e.Name
	}
	return ""
}

// onInternal reports whether a WithDetail chain starts at ErrInternal.
func onInternal(fun ast.Expr) bool {
	expr := fun
	for {
		switch e := expr.(type) {
		case *ast.SelectorExpr:
			if e.Sel.Name == "ErrInternal" {
				return true
			}
			expr = e.X
		case *ast.CallExpr:
			expr = e.Fun
		case *ast.Ident:
			return e.Name == "ErrInternal"
		default:
			return false
		}
	}
}
