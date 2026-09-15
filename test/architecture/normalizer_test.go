// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// normalizerPort is the package whose Normalizer every constructor storing user text takes (M-07,
// i18n-l10n.md §5).
const normalizerPort = module + "/core/port/text"

// TestEveryTextNormalizerIsHandedIn is the wiring half of M-07.
//
// shared.NFC fails closed: a constructor handed no Normalizer refuses any text that is not ASCII
// rather than storing it in whatever form it arrived. That protects the rows, and it turns a
// forgotten wire into an internal error the first person to type an umlaut meets in production.
// This test meets it first. A struct that declares a field of the port's type is a struct that
// needs one, so every composite literal of that type - a domain input built by a use case, a use
// case built by the composition root - has to set the field. Syntactic on purpose: the test reads
// the source the way a reviewer would, and needs no type checker and no dependency to do it.
func TestEveryTextNormalizerIsHandedIn(t *testing.T) {
	// The types that declare the field, keyed by import path and type name.
	needs := map[string]map[string]bool{}
	forEachGoFile(t, []string{"../../core"}, func(path string, f *ast.File, _ *token.FileSet) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		alias := importAlias(f, normalizerPort)
		if alias == "" {
			return
		}
		pkg := packagePath(path)
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok || !declaresNormalizer(structType, alias) {
					continue
				}
				if needs[pkg] == nil {
					needs[pkg] = map[string]bool{}
				}
				needs[pkg][typeSpec.Name.Name] = true
			}
		}
	})
	if len(needs) == 0 {
		t.Fatal("no struct declares a text.Normalizer field - the port is not in use")
	}

	// Every literal of one of those types, wherever it is built, sets the field. The catalogue is
	// the one place that builds a use case with no dependencies at all: it reads Descriptor(),
	// which reads none of them, and nothing there ever runs.
	var missing []string
	forEachGoFile(t, []string{"../../core", "../../cmd", "../../presentation"},
		func(path string, f *ast.File, fset *token.FileSet) {
			if strings.HasSuffix(path, "_test.go") ||
				strings.HasPrefix(rel(path), "core/application/catalogue/") {
				return
			}
			byAlias := map[string]string{}
			for _, imp := range f.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				if needs[importPath] == nil {
					continue
				}
				byAlias[aliasOf(imp, importPath)] = importPath
			}
			own := packagePath(path)

			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				typeName, pkg := literalType(lit, byAlias, own, needs)
				if typeName == "" || setsField(lit, "Text") {
					return true
				}
				missing = append(missing, fset.Position(lit.Pos()).String()+": "+
					filepath.Base(pkg)+"."+typeName)
				return true
			})
		})

	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("%s is built without its Text normaliser - every constructor that stores "+
			"user text is handed one (M-07)", m)
	}
}

// declaresNormalizer reports whether a struct has a field typed `<alias>.Normalizer`.
func declaresNormalizer(structType *ast.StructType, alias string) bool {
	for _, field := range structType.Fields.List {
		sel, ok := field.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Normalizer" {
			continue
		}
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == alias {
			return true
		}
	}
	return false
}

// literalType names the struct a composite literal builds, when it is one that needs the port:
// `alias.Type{...}` from another package, or a bare `Type{...}` in the package that declares it.
func literalType(
	lit *ast.CompositeLit, byAlias map[string]string, own string, needs map[string]map[string]bool,
) (typeName, pkg string) {
	switch typ := lit.Type.(type) {
	case *ast.SelectorExpr:
		ident, ok := typ.X.(*ast.Ident)
		if !ok {
			return "", ""
		}
		if pkg, known := byAlias[ident.Name]; known && needs[pkg][typ.Sel.Name] {
			return typ.Sel.Name, pkg
		}
	case *ast.Ident:
		if needs[own][typ.Name] {
			return typ.Name, own
		}
	}
	return "", ""
}

// setsField reports whether a keyed composite literal names the field.
func setsField(lit *ast.CompositeLit, name string) bool {
	for _, element := range lit.Elts {
		kv, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == name {
			return true
		}
	}
	return false
}

// importAlias answers the name a file refers to an import by, or "" when it does not import it.
func importAlias(f *ast.File, importPath string) string {
	for _, imp := range f.Imports {
		if strings.Trim(imp.Path.Value, `"`) == importPath {
			return aliasOf(imp, importPath)
		}
	}
	return ""
}

func aliasOf(imp *ast.ImportSpec, importPath string) string {
	if imp.Name != nil {
		return imp.Name.Name
	}
	return importPath[strings.LastIndex(importPath, "/")+1:]
}

// packagePath turns a file path under the repository into its import path.
func packagePath(path string) string {
	return module + "/" + filepath.ToSlash(filepath.Dir(rel(path)))
}
