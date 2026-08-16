// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package architecture enforces the rules no linter knows about.
//
// These tests are a CI gate (make gate-architecture). They are deliberately kept simple: they
// read the source rather than pulling in an analysis library. A rule that lives only in a
// document decays; a rule with a red build does not.
package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const module = "github.com/Jersyfi/hubtask"

// TestCoreStaysClean checks rule 1 from CLAUDE.md (ADR-0001): core/domain and core/port must
// point neither outwards nor at third-party libraries.
func TestCoreStaysClean(t *testing.T) {
	forbiddenPrefixes := []string{
		module + "/infrastructure",
		module + "/presentation",
		module + "/cmd",
	}

	forEachGoFile(t, []string{"../../core/domain", "../../core/port"}, func(path string, f *ast.File, fset *token.FileSet) {
		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			for _, forbidden := range forbiddenPrefixes {
				if strings.HasPrefix(importPath, forbidden) {
					t.Errorf("%s: core imports outwards: %s (ADR-0001)", rel(path), importPath)
				}
			}

			// A third-party dependency is recognisable by its first segment containing a dot
			// (a hostname) while not belonging to our own module.
			first := strings.SplitN(importPath, "/", 2)[0]
			if strings.Contains(first, ".") && !strings.HasPrefix(importPath, module) {
				t.Errorf("%s: third-party dependency in the core: %s (ADR-0001)", rel(path), importPath)
			}
		}
	})
}

// TestNoBareGoroutines checks rule 5 from CLAUDE.md (ADR-0016). A panic in an unguarded
// goroutine terminates the whole process.
func TestNoBareGoroutines(t *testing.T) {
	allowed := filepath.Clean("../../core/shared/concurrency")

	forEachGoFile(t, []string{"../../core", "../../infrastructure", "../../presentation", "../../cmd"},
		func(path string, f *ast.File, fset *token.FileSet) {
			if strings.HasPrefix(filepath.Clean(filepath.Dir(path)), allowed) {
				return
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if g, ok := n.(*ast.GoStmt); ok {
					t.Errorf("%s:%d: bare `go` statement - use concurrency.Go (ADR-0016)",
						rel(path), fset.Position(g.Pos()).Line)
				}
				return true
			})
		})
}

// TestNoDirectTimeSource checks rule 4 from CLAUDE.md: the domain and application layers must
// not reach for the clock or for randomness themselves, otherwise they are not deterministically
// testable (arc42 §8.13).
func TestNoDirectTimeSource(t *testing.T) {
	forbidden := map[string][]string{
		"time":      {"Now"},
		"math/rand": {"Int", "Intn", "Float64", "Shuffle", "New"},
	}

	forEachGoFile(t, []string{"../../core/domain", "../../core/application"},
		func(path string, f *ast.File, fset *token.FileSet) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			ast.Inspect(f, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				for pkg, funcs := range forbidden {
					short := pkg[strings.LastIndex(pkg, "/")+1:]
					if ident.Name != short {
						continue
					}
					for _, fn := range funcs {
						if sel.Sel.Name == fn {
							t.Errorf("%s:%d: %s.%s is forbidden - use the Clock/RandomSource port (arc42 §8.13)",
								rel(path), fset.Position(sel.Pos()).Line, short, fn)
						}
					}
				}
				return true
			})
		})
}

func forEachGoFile(t *testing.T, roots []string, fn func(string, *ast.File, *token.FileSet)) {
	t.Helper()
	fset := token.NewFileSet()

	for _, root := range roots {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if perr != nil {
				t.Errorf("%s is not parseable: %v", path, perr)
				return nil
			}
			fn(path, f, fset)
			return nil
		})
		if err != nil {
			t.Fatalf("directory %s is not readable: %v", root, err)
		}
	}
}

func rel(p string) string {
	c := filepath.Clean(p)
	return strings.TrimPrefix(c, "../../")
}
