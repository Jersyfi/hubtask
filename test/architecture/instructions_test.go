// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The instructions for every coding worker are AGENTS.md (ADR-0081). A CLAUDE.md beside one hides
// it from Claude Code, which reads CLAUDE.md when both exist; an AGENTS.override.md hides it from
// Codex. Either can arrive without anybody meaning it - a tool's init command writes one - so the
// gate refuses the file rather than trusting nobody to commit it.
func TestNoInstructionFileHidesAgentsMD(t *testing.T) {
	hiding := map[string]bool{"CLAUDE.md": true, "CLAUDE.local.md": true, "AGENTS.override.md": true}
	for _, file := range trackedFiles(t) {
		if hiding[path.Base(file)] {
			t.Errorf("%s is committed; it would hide the AGENTS.md beside it from some coding agents - fold its content into AGENTS.md", file)
		}
	}
}

// Rule 14: the traffic between the Go tree and the clients runs one way - the design system
// generates one Go file into the core - so no Go file lives under apps/ or packages/.
func TestNoGoFileUnderAppsOrPackages(t *testing.T) {
	for _, file := range trackedFiles(t) {
		if strings.HasSuffix(file, ".go") && (strings.HasPrefix(file, "apps/") || strings.HasPrefix(file, "packages/")) {
			t.Errorf("%s is a Go file under apps/ or packages/ (rule 14)", file)
		}
	}
}

// Every Go package says what it is responsible for: the package comment is the first thing a
// reader of the package sees, and the one place its purpose is kept. Generated packages are
// exempt - their generator writes them.
func TestEveryGoPackageSaysWhatItIsFor(t *testing.T) {
	documented := map[string]bool{}
	seen := map[string]bool{}
	fset := token.NewFileSet()
	for _, root := range []string{"../../core", "../../infrastructure", "../../presentation", "../../cmd", "../../tools"} {
		err := filepath.WalkDir(root, func(file string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
				return err
			}
			f, perr := parser.ParseFile(fset, file, nil, parser.PackageClauseOnly|parser.ParseComments)
			if perr != nil {
				t.Errorf("%s is not parseable: %v", rel(file), perr)
				return nil
			}
			if isGenerated(f) {
				return nil
			}
			dir := rel(filepath.Dir(file))
			seen[dir] = true
			if f.Doc != nil && strings.TrimSpace(f.Doc.Text()) != "" {
				documented[dir] = true
			}
			return nil
		})
		if err != nil {
			t.Fatalf("directory %s is not readable: %v", root, err)
		}
	}
	var missing []string
	for dir := range seen {
		if !documented[dir] {
			missing = append(missing, dir)
		}
	}
	sort.Strings(missing)
	for _, dir := range missing {
		t.Errorf("package %s has no package comment saying what it is responsible for", dir)
	}
	if len(seen) == 0 {
		t.Fatal("no Go package found - the roots are wrong")
	}
}

func isGenerated(f *ast.File) bool {
	for _, group := range f.Comments {
		if group.Pos() > f.Package {
			break
		}
		for _, c := range group.List {
			if strings.HasPrefix(c.Text, "// Code generated") && strings.HasSuffix(c.Text, "DO NOT EDIT.") {
				return true
			}
		}
	}
	return false
}
