// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"go/ast"
	"go/token"
	"path/filepath"
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
// answers without its detail, so nobody reads it. A code named by a string constant - the
// archive's `CodeChainBroken` - is held by the constant's value, in its own package or another.
func TestEveryDetailCodeAPersonCanMeetIsInTheCatalogue(t *testing.T) {
	messages := loadCatalogue(t)
	missing := map[string][]string{}
	found := 0
	consts := stringConstants(t, detailRoots)

	forEachGoFile(t, detailRoots,
		func(path string, file *ast.File, fset *token.FileSet) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			note := func(lit ast.Expr) {
				code, ok := codeOf(lit, filepath.Dir(rel(path)), file, consts)
				if !ok || code == "" {
					return
				}
				found++
				if _, ok := messages[code]; !ok {
					at := rel(path) + ":" + strconv.Itoa(fset.Position(lit.Pos()).Line)
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

var detailRoots = []string{"../../core", "../../infrastructure", "../../presentation", "../../cmd"}

const modulePath = "github.com/Jersyfi/hubtask/"

// stringConstants answers every package-level string constant, keyed by its package directory
// and name ("core/application/archive.CodeChainBroken").
func stringConstants(t *testing.T, roots []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	forEachGoFile(t, roots, func(path string, file *ast.File, _ *token.FileSet) {
		dir := filepath.Dir(rel(path))
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) != len(value.Values) {
					continue
				}
				for i, name := range value.Names {
					if basic, ok := value.Values[i].(*ast.BasicLit); ok && basic.Kind == token.STRING {
						if text, err := strconv.Unquote(basic.Value); err == nil {
							out[dir+"."+name.Name] = text
						}
					}
				}
			}
		}
	})
	return out
}

// codeOf answers the string a detail-code argument names: a literal, a constant of the file's own
// package, or a constant of an imported package of this module. Anything else is not judged.
func codeOf(expr ast.Expr, dir string, file *ast.File, consts map[string]string) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		code, err := strconv.Unquote(e.Value)
		return code, err == nil
	case *ast.Ident:
		code, ok := consts[dir+"."+e.Name]
		return code, ok
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		if !ok {
			return "", false
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil || !strings.HasPrefix(path, modulePath) {
				continue
			}
			name := filepath.Base(path)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			if name == pkg.Name {
				code, ok := consts[strings.TrimPrefix(path, modulePath)+"."+e.Sel.Name]
				return code, ok
			}
		}
	}
	return "", false
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
