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

// TestCoreStaysClean checks rule 1 from CLAUDE.md (ADR-0001): the core must point neither
// outwards nor at third-party libraries. core/shared is included - the promise in go.mod covers
// the whole core.
func TestCoreStaysClean(t *testing.T) {
	forbiddenPrefixes := []string{
		module + "/infrastructure",
		module + "/presentation",
		module + "/cmd",
		module + "/core/application", // dependencies point inwards; the domain is innermost
	}

	forEachGoFile(t, []string{"../../core/domain", "../../core/port", "../../core/shared"}, func(path string, f *ast.File, fset *token.FileSet) {
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

// TestCoreStaysTechnologyFree keeps transport and persistence out of the core (ADR-0001). The
// same boundary is configured for depguard; it is checked here as well because a linter can be
// reconfigured in a pull request, whereas a red test is visible in the review.
func TestCoreStaysTechnologyFree(t *testing.T) {
	// The domain must not serialise itself either: DTOs belong to the application layer
	// (project-structure.md §3).
	roots := map[string][]string{
		"../../core/domain":      {"net/http", "database/sql", "encoding/json"},
		"../../core/port":        {"net/http", "database/sql"},
		"../../core/shared":      {"net/http", "database/sql"},
		"../../core/application": {"net/http", "database/sql"},
	}

	for root, forbidden := range roots {
		forEachGoFile(t, []string{root}, func(path string, f *ast.File, fset *token.FileSet) {
			for _, imp := range f.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				for _, bad := range forbidden {
					if importPath == bad {
						t.Errorf("%s: %s belongs behind a port, not in the core (ADR-0001)", rel(path), bad)
					}
				}
			}
		})
	}
}

// TestAdaptersDoNotCallUseCases checks the dependency rule from project-structure.md §2: an
// outbound adapter implements ports, it does not drive the application layer. Repository
// interfaces (core/application/repository) are fine - those are exactly what it implements.
func TestAdaptersDoNotCallUseCases(t *testing.T) {
	forEachGoFile(t, []string{"../../infrastructure"}, func(path string, f *ast.File, fset *token.FileSet) {
		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(importPath, module+"/core/application/service") {
				t.Errorf("%s: adapter calls a use case: %s (project-structure.md §2)", rel(path), importPath)
			}
			if strings.HasPrefix(importPath, module+"/presentation") {
				t.Errorf("%s: adapters do not know each other: %s (project-structure.md §2)", rel(path), importPath)
			}
		}
	})
}

// TestDriverStaysInThePostgresAdapter checks rule 3 from CLAUDE.md: every query goes through the
// transaction wrapper, which is the only place that sets `SET LOCAL app.tenant_id` (ADR-0010).
// Anything holding the driver itself can bypass that wrapper - so nothing else may hold it.
func TestDriverStaysInThePostgresAdapter(t *testing.T) {
	allowed := []string{
		filepath.Clean("../../infrastructure/postgres"),
		filepath.Clean("../../cmd/migrate"),
		// The integration suite connects as the raw application role, without the wrapper -
		// that is how it proves the database enforces the boundary rather than the code.
		// Testing the wrapper through the wrapper would prove nothing.
		filepath.Clean("../../test/integration"),
	}

	forEachGoFile(t, []string{"../../core", "../../infrastructure", "../../presentation", "../../cmd", "../../test"},
		func(path string, f *ast.File, fset *token.FileSet) {
			dir := filepath.Clean(filepath.Dir(path))
			for _, a := range allowed {
				if strings.HasPrefix(dir, a) {
					return
				}
			}
			for _, imp := range f.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				if strings.HasPrefix(importPath, "github.com/jackc/pgx") {
					t.Errorf("%s: the driver belongs to infrastructure/postgres - go through the transaction wrapper (ADR-0010)",
						rel(path))
				}
			}
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
