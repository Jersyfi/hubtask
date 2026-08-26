// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package privacy holds gates PG-1 to PG-8 (data-protection.md §10, ADR-0018).
//
// They were asserted by four documents and existed in no form at all until E-11 - not a Makefile
// target, not a script, not a test - and one acceptance had already claimed a green from them. So
// the point of this package is not that the checks are clever: it is that they run, that they are
// wired into a gate whose cost suits them, and that `gate-selftest` proves each of them goes red
// against a deliberate violation.
//
// Where each one runs: the cheap ones here, in `make gate-privacy`, which `make verify` runs. The
// two that need a database - PG-2, the deletion test across every storage location, and PG-7, the
// reconciliation of the schema against the data catalogue - carry the `integration` build tag and
// run in the nightly with containers, which is where ADR-0018 puts "the expensive one with the
// highest protective value".
package privacy

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The roots the gates read. `cmd` is in there because a composition root writes audit entries too,
// and a rule that stopped at the layers would be a rule with a hole in the one file that wires
// them together.
var sourceRoots = []string{
	"../../core", "../../infrastructure", "../../presentation", "../../cmd",
}

// forEachGoFile walks the source and hands over every non-test file, parsed.
//
// Test files are left out for the reason the message code gate leaves them out: a fixture is an
// example of a shape rather than something a person's data ever passes through, and demanding the
// same discipline of one would make every table-driven test carry ceremony that proves nothing.
func forEachGoFile(t *testing.T, fn func(path string, file *ast.File, fset *token.FileSet)) {
	t.Helper()
	fset := token.NewFileSet()

	for _, root := range sourceRoots {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case entry.IsDir(), !strings.HasSuffix(path, ".go"), strings.HasSuffix(path, "_test.go"):
				return nil
			}

			parsed, perr := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
			if perr != nil {
				t.Errorf("%s is not parseable: %v", path, perr)
				return nil
			}
			fn(path, parsed, fset)
			return nil
		})
		if err != nil {
			t.Fatalf("%s is not readable: %v", root, err)
		}
	}
}

// at is a source position as a reader of the failure needs it: the path they can open, and the
// line they can jump to.
func at(path string, fset *token.FileSet, node ast.Node) string {
	position := fset.Position(node.Pos())
	return strings.TrimPrefix(filepath.Clean(path), "../../") + ":" +
		strings.TrimPrefix(position.String()[strings.LastIndex(position.String(), ":")+1:], "")
}

// readFile is the whole of a file, for the checks that read text rather than syntax.
func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s is not readable: %v", path, err)
	}
	return string(content)
}
