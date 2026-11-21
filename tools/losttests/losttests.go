// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package losttests reads which tests a change takes away, for the pull request gate and the
// flow report alike, so both read a diff the same way.
//
// Removing a test removes proof, and a deleted or skipped test is the cheapest way to turn a red
// build green (engineering-guidelines.md §3.20). So a test a change takes away is named in the
// description, where the reviewer and the owner read it with its reason. Read from the diff of the
// test files between two commits:
//
//   - a Go `func Test…`/`func Fuzz…`, or a `test(`/`it(` of a *.test.ts, *.test.mjs or *.test.js,
//     whose declaration is removed and whose name is added nowhere in the diff. A name that comes
//     back - in the same file or another - is a moved test. A removed declaration that a new one
//     replaces in the same run of changed lines is a rename.
//   - a test file deleted, unless git reads it as renamed or every test of it comes back.
//   - a skip or todo added to a test the base already had.
//
// What it cannot see: an assertion removed or loosened; a test deleted and another written in
// its place, which reads as a rename; a test removed in one package while one of the same name is
// added in another, which reads as a move; whether the stated reason is true.
package losttests

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path"
	"regexp"
	"strings"
)

// testFiles are the pathspecs of the files read.
var testFiles = []string{"*_test.go", "*.test.ts", "*.test.mjs", "*.test.js"}

var (
	goTestDecl = regexp.MustCompile(`^func (?:\([^)]*\) )?((?:Test|Fuzz)\w*)\(`)
	goFuncDecl = regexp.MustCompile(`^func (?:\([^)]*\) )?(\w+)[\[(]`)
	goSkip     = regexp.MustCompile(`\.Skip(?:f|Now)?\(`)
	jsTestDecl = regexp.MustCompile("^\\s*(?:test|it)(?:\\.(?:skip|todo|only))?\\(\\s*(['\"`])")
	jsSkip     = regexp.MustCompile(`\.skip\(|\bskip:|\.todo\(|\btodo:`)
	jsInserted = regexp.MustCompile(`\$\{[^}]*\}`)
	commentRow = regexp.MustCompile(`^\s*(?://|\*|/\*)`)
)

// minTitlePart is the shortest fixed part of an interpolated test title that still names the test;
// below it the file's name is asked for.
const minTitlePart = 12

// Test is a test the change takes away. Name is what the description has to contain.
type Test struct {
	Path, Name string
	How        string // removed, deleted (a file), skipped
	Identifier bool   // Name is a Go identifier, matched as a whole word
}

// declaration is a test's - or, for a skip in Go, any function's - declaration line.
type declaration struct {
	name       string
	identifier bool
	added      bool
}

type skipLine struct {
	path string
	in   *declaration // nil outside any declaration
}

// Between reads the diff of the test files from base to head, two commits of the repository at
// root. The caller chooses the base: the merge base for a branch, the parent for a merge commit.
func Between(ctx context.Context, root, base, head string) ([]Test, error) {
	// With rename detection: a file git reads as renamed is not deleted.
	args := append([]string{"-c", "core.quotepath=off", "diff", "--name-only", "-M", "--diff-filter=D", base, head, "--"}, testFiles...)
	names, err := git(ctx, root, args...)
	if err != nil {
		return nil, err
	}
	deleted := map[string]bool{}
	for _, name := range strings.Split(strings.TrimSpace(names), "\n") {
		if name != "" {
			deleted[name] = true
		}
	}
	// Each file whole, so the declaration a changed line stands in is in the diff; the prefixes
	// and the driver fixed, so a caller's git configuration does not change what is parsed.
	args = append([]string{"-c", "core.quotepath=off", "diff", "--no-renames", "--no-color", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/", "-U1000000", base, head, "--"}, testFiles...)
	diff, err := git(ctx, root, args...)
	if err != nil {
		return nil, err
	}
	return In(diff, deleted), nil
}

func git(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: fixed command, arguments from the calling tool
	cmd.Dir = root
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[2], err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

// In reads a unified diff that carries every changed test file whole.
func In(diff string, deleted map[string]bool) []Test {
	type run struct {
		path           string
		removed, added []declaration
	}
	var (
		runs        []run
		skips       []skipLine
		order       []string // the files, as the diff lists them
		file        string
		inHunk      bool
		current     *run
		enclosing   *declaration
		skipsGone   int
		runSkips    []skipLine
		addedName   = map[string]bool{}
		removedName = map[string]bool{}
	)
	closeRun := func() {
		if current != nil {
			runs = append(runs, *current)
		}
		// A changed line that had its skip before adds none.
		if len(runSkips) > skipsGone {
			skips = append(skips, runSkips...)
		}
		current, skipsGone, runSkips = nil, 0, nil
	}
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			closeRun()
			file, inHunk, enclosing = "", false, nil
			continue
		case !inHunk:
			if p, ok := strings.CutPrefix(line, "--- a/"); ok {
				file = p
			} else if p, ok := strings.CutPrefix(line, "+++ b/"); ok {
				file = p
			} else if strings.HasPrefix(line, "@@") {
				inHunk = true
				order = append(order, file)
			}
			continue
		case line == "" || line[0] == '\\':
			continue
		}
		mark, text := line[0], line[1:]
		if mark == ' ' {
			closeRun()
		} else if current == nil {
			current = &run{path: file}
		}
		isGo := strings.HasSuffix(file, ".go")
		test, isTest := testDeclared(text, isGo)
		switch mark {
		case '-':
			if isTest {
				current.removed = append(current.removed, test)
				removedName[test.name] = true
			}
			if skipsIn(text, isGo) {
				skipsGone++
			}
		case '+', ' ':
			added := mark == '+'
			if isTest {
				test.added = added
				enclosing = &test
				if added {
					current.added = append(current.added, test)
					addedName[test.name] = true
				}
			} else if m := goFuncDecl.FindStringSubmatch(text); isGo && m != nil {
				enclosing = &declaration{name: m[1], identifier: true, added: added}
			}
			if added && skipsIn(text, isGo) {
				runSkips = append(runSkips, skipLine{path: file, in: enclosing})
			}
		}
	}
	closeRun()

	// A removed name that is added nowhere is gone; within a run, a gone test and a new one pair
	// up, in order, as a rename.
	gone := map[string][]declaration{}
	renamedTo := map[string]bool{}
	for _, r := range runs {
		var fresh []declaration
		for _, d := range r.added {
			if !removedName[d.name] {
				fresh = append(fresh, d)
			}
		}
		for _, d := range r.removed {
			switch {
			case addedName[d.name]:
			case len(fresh) > 0 && !deleted[r.path]:
				renamedTo[fresh[0].name] = true
				fresh = fresh[1:]
			default:
				gone[r.path] = append(gone[r.path], d)
			}
		}
	}

	var out []Test
	declared := map[string]bool{}
	for _, r := range runs {
		declared[r.path] = declared[r.path] || len(r.removed) > 0
	}
	for _, file := range order {
		if deleted[file] {
			if len(gone[file]) > 0 || !declared[file] {
				out = append(out, Test{Path: file, Name: path.Base(file), How: "deleted"})
			}
			continue
		}
		for _, d := range gone[file] {
			out = append(out, named(file, d, "removed"))
		}
	}
	seen := map[string]bool{}
	for _, s := range skips {
		lost := Test{Path: s.path, Name: path.Base(s.path), How: "skipped"}
		if s.in != nil {
			// A test the branch itself adds was never proof at the base.
			if s.in.added && !removedName[s.in.name] && !renamedTo[s.in.name] {
				continue
			}
			lost = named(s.path, *s.in, "skipped")
		}
		if key := lost.Path + "\x00" + lost.Name; !seen[key] {
			seen[key] = true
			out = append(out, lost)
		}
	}
	return out
}

func named(file string, d declaration, how string) Test {
	if d.name == "" {
		return Test{Path: file, Name: path.Base(file), How: how}
	}
	return Test{Path: file, Name: d.name, How: how, Identifier: d.identifier}
}

// testDeclared reads a test's declaration from a line. A title built from inserted values is
// known by its longest fixed part; one too short to tell tests apart leaves the name empty.
func testDeclared(text string, isGo bool) (declaration, bool) {
	if isGo {
		m := goTestDecl.FindStringSubmatch(text)
		if m == nil {
			return declaration{}, false
		}
		return declaration{name: m[1], identifier: true}, true
	}
	at := jsTestDecl.FindStringSubmatchIndex(text)
	if at == nil {
		return declaration{}, false
	}
	quote, rest := text[at[2]], text[at[3]:]
	var title strings.Builder
	for i := 0; i < len(rest) && rest[i] != quote; i++ {
		if rest[i] == '\\' && i+1 < len(rest) {
			i++
		}
		title.WriteByte(rest[i])
	}
	name := ""
	for _, part := range jsInserted.Split(title.String(), -1) {
		if part = strings.TrimSpace(part); len(part) > len(name) {
			name = part
		}
	}
	if quote == '`' && jsInserted.MatchString(title.String()) && len(name) < minTitlePart {
		name = ""
	}
	return declaration{name: name}, true
}

func skipsIn(text string, isGo bool) bool {
	if commentRow.MatchString(text) {
		return false
	}
	if isGo {
		return goSkip.MatchString(text)
	}
	return jsSkip.MatchString(text)
}

// NamedIn reports whether the description names the test.
func (t Test) NamedIn(description string) bool {
	if t.Identifier {
		return regexp.MustCompile(`\b` + regexp.QuoteMeta(t.Name) + `\b`).MatchString(description)
	}
	return strings.Contains(description, t.Name)
}
